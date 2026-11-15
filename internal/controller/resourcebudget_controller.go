// Package controller contains the ResourceBudget reconciler.
//
// The reconciler compares the aggregate CPU/memory requests of all pods in a
// namespace against the ResourceBudget custom resource and reports the result
// in the CR's status plus a Kubernetes event and Prometheus metrics.
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	appsv1alpha1 "github.com/Ankitavasudev/quota-guard/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	requeueAfter = 30 * time.Second
)

var (
	cpuRequestGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "quota_guard_cpu_request_millicores",
		Help: "Sum of pod CPU requests in the namespace.",
	}, []string{"namespace"})
	memoryRequestGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "quota_guard_memory_request_bytes",
		Help: "Sum of pod memory requests in the namespace.",
	}, []string{"namespace"})
	storageRequestGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "quota_guard_storage_request_bytes",
		Help: "Sum of pod storage requests in the namespace.",
	}, []string{"namespace"})
	ephemeralRequestGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "quota_guard_ephemeral_storage_request_bytes",
		Help: "Sum of pod ephemeral-storage requests in the namespace.",
	}, []string{"namespace"})
	overBudgetGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "quota_guard_over_budget",
		Help: "1 when the namespace exceeds its ResourceBudget, 0 otherwise.",
	}, []string{"namespace"})
)

func init() {
	prometheus.MustRegister(cpuRequestGauge, memoryRequestGauge, storageRequestGauge, ephemeralRequestGauge, overBudgetGauge)
}

// ResourceBudgetReconciler reconciles a ResourceBudget object.
type ResourceBudgetReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager registers the controller with the manager.
func (r *ResourceBudgetReconciler) SetupWithManager(mgr manager.Manager) error {
	// Re-reconcile every budget in a namespace whenever a pod there changes.
	enqueue := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		budgets := &appsv1alpha1.ResourceBudgetList{}
		if err := r.Client.List(ctx, budgets, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(budgets.Items))
		for _, b := range budgets.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: b.Namespace, Name: b.Name}})
		}
		return reqs
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.ResourceBudget{}).
		Watches(&corev1.Pod{}, enqueue, builder.WithPredicates(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return true },
			GenericFunc: func(e event.GenericEvent) bool { return true },
		})).
		Complete(r)
}

// Reconcile compares pod requests against the budget and updates the CR status.
func (r *ResourceBudgetReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	budget := &appsv1alpha1.ResourceBudget{}
	if err := r.Client.Get(ctx, req.NamespacedName, budget); err != nil {
		if errors.IsNotFound(err) {
			// Budget deleted: nothing to reconcile.
			deleteGauges(req.Namespace)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	var cpuBudget resource.Quantity
	var memBudget resource.Quantity
	var storageBudget resource.Quantity
	var ephemeralBudget resource.Quantity
	if q, err := resource.ParseQuantity(budget.Spec.CPU); err == nil {
		cpuBudget = q
	}
	if q, err := resource.ParseQuantity(budget.Spec.Memory); err == nil {
		memBudget = q
	}
	if budget.Spec.Storage != "" {
		if q, err := resource.ParseQuantity(budget.Spec.Storage); err == nil {
			storageBudget = q
		}
	}
	if budget.Spec.EphemeralStorage != "" {
		if q, err := resource.ParseQuantity(budget.Spec.EphemeralStorage); err == nil {
			ephemeralBudget = q
		}
	}

	pods := &corev1.PodList{}
	if err := r.Client.List(ctx, pods, client.InNamespace(req.Namespace)); err != nil {
		return reconcile.Result{}, err
	}

	var cpuUsed, memUsed int64
	var storageUsed, ephemeralUsed int64
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, container := range pod.Spec.Containers {
			cpuUsed += container.Resources.Requests.Cpu().MilliValue()
			memUsed += container.Resources.Requests.Memory().Value()
			if q, ok := container.Resources.Requests[corev1.ResourceStorage]; ok {
				storageUsed += q.Value()
			}
			if q, ok := container.Resources.Requests[corev1.ResourceEphemeralStorage]; ok {
				ephemeralUsed += q.Value()
			}
		}
	}


	overBudget := memUsed > memBudget.Value() || (cpuBudget.MilliValue() > 0 && cpuUsed > cpuBudget.MilliValue())
	if storageBudget.Value() > 0 && storageUsed > storageBudget.Value() {
		overBudget = true
	}
	if ephemeralBudget.Value() > 0 && ephemeralUsed > ephemeralBudget.Value() {
		overBudget = true
	}

	status := budget.Status
	status.ObservedCPU = resource.NewMilliQuantity(cpuUsed, resource.DecimalSI).String()
	status.ObservedMemory = resource.NewQuantity(memUsed, resource.BinarySI).String()
	if budget.Spec.Storage != "" {
		status.ObservedStorage = resource.NewQuantity(storageUsed, resource.BinarySI).String()
	}
	if budget.Spec.EphemeralStorage != "" {
		status.ObservedEphemeralStorage = resource.NewQuantity(ephemeralUsed, resource.BinarySI).String()
	}
	status.Pods = len(pods.Items)
	status.WithinBudget = !overBudget
	now := metav1.Now()
	status.LastCalculated = &now

	msg := fmt.Sprintf("pod requests (%dm CPU, %s memory) vs budget (%s CPU, %s memory)", cpuUsed, resource.NewQuantity(memUsed, resource.BinarySI).String(), budget.Spec.CPU, budget.Spec.Memory)
	if budget.Spec.Storage != "" {
		msg += fmt.Sprintf("; storage %s/%s", resource.NewQuantity(storageUsed, resource.BinarySI).String(), budget.Spec.Storage)
	}
	if budget.Spec.EphemeralStorage != "" {
		msg += fmt.Sprintf("; ephemeral %s/%s", resource.NewQuantity(ephemeralUsed, resource.BinarySI).String(), budget.Spec.EphemeralStorage)
	}

	condition := metav1.Condition{
		Type:               "WithinBudget",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: budget.Generation,
		LastTransitionTime: now,
		Reason:             "WithinBudget",
		Message:            "within budget: " + msg,
	}
	if overBudget {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "OverBudget"
		condition.Message = "over budget: " + msg
	}
	apimeta.SetStatusCondition(&status.Conditions, condition)
	budget.Status = status

	if err := r.Client.Status().Update(ctx, budget); err != nil {
		logger.Error(err, "unable to update budget status")
		return reconcile.Result{}, err
	}

	cpuRequestGauge.WithLabelValues(req.Namespace).Set(float64(cpuUsed))
	memoryRequestGauge.WithLabelValues(req.Namespace).Set(float64(memUsed))
	storageRequestGauge.WithLabelValues(req.Namespace).Set(float64(storageUsed))
	ephemeralRequestGauge.WithLabelValues(req.Namespace).Set(float64(ephemeralUsed))
	if overBudget {
		overBudgetGauge.WithLabelValues(req.Namespace).Set(1)
		r.Recorder.Event(budget, corev1.EventTypeWarning, "OverBudget", condition.Message)
	} else {
		overBudgetGauge.WithLabelValues(req.Namespace).Set(0)
		r.Recorder.Event(budget, corev1.EventTypeNormal, "WithinBudget", condition.Message)
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
}

func deleteGauges(namespace string) {
	cpuRequestGauge.DeleteLabelValues(namespace)
	memoryRequestGauge.DeleteLabelValues(namespace)
	storageRequestGauge.DeleteLabelValues(namespace)
	ephemeralRequestGauge.DeleteLabelValues(namespace)
	overBudgetGauge.DeleteLabelValues(namespace)
}
