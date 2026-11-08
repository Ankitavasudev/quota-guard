package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1alpha1 "github.com/Ankitavasudev/quota-guard/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("clientgo scheme: %v", err)
	}
	if err := appsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("quota-guard scheme: %v", err)
	}
	return s
}

func newBudget(namespace, name, cpu, memory string) *appsv1alpha1.ResourceBudget {
	return &appsv1alpha1.ResourceBudget{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       appsv1alpha1.ResourceBudgetSpec{CPU: cpu, Memory: memory},
	}
}

func podWithRequests(namespace, name string, cpuM, memMi int64) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%dm", cpuM)),
							corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memMi)),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func reconcileOnce(t *testing.T, b *appsv1alpha1.ResourceBudget, pods []corev1.Pod) appsv1alpha1.ResourceBudgetStatus {
	t.Helper()
	s := testScheme(t)

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(b).WithStatusSubresource(b)
	for i := range pods {
		builder = builder.WithObjects(&pods[i])
	}
	cli := builder.Build()

	r := &ResourceBudgetReconciler{Client: cli, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: b.Namespace, Name: b.Name},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &appsv1alpha1.ResourceBudget{}
	nn := types.NamespacedName{Namespace: b.Namespace, Name: b.Name}
	if err := cli.Get(context.Background(), nn, got); err != nil {
		t.Fatalf("get budget: %v", err)
	}
	return got.Status
}

func TestWithinBudgetWhenPodsFit(t *testing.T) {
	b := newBudget("web", "main", "4", "8Gi")
	pods := []corev1.Pod{
		podWithRequests("web", "a", 2000, 2048),
		podWithRequests("web", "b", 1000, 1024),
	}
	status := reconcileOnce(t, b, pods)
	if !status.WithinBudget {
		t.Fatalf("expected WithinBudget=true, got %+v", status)
	}
	if cond := apimeta.FindStatusCondition(status.Conditions, "WithinBudget"); cond == nil || cond.Status != "True" {
		t.Fatalf("expected WithinBudget condition True, got %+v", cond)
	}
	if status.Pods != 2 {
		t.Fatalf("expected 2 pods counted, got %d", status.Pods)
	}
}

func TestOverBudgetWhenPodsExceed(t *testing.T) {
	b := newBudget("web", "main", "1", "1Gi")
	pods := []corev1.Pod{
		podWithRequests("web", "a", 1500, 2048),
	}
	status := reconcileOnce(t, b, pods)
	if status.WithinBudget {
		t.Fatalf("expected WithinBudget=false, got %+v", status)
	}
	if cond := apimeta.FindStatusCondition(status.Conditions, "WithinBudget"); cond == nil || cond.Status != "False" {
		t.Fatalf("expected WithinBudget condition False, got %+v", cond)
	}
	if cond := apimeta.FindStatusCondition(status.Conditions, "WithinBudget"); cond != nil && cond.Reason != "OverBudget" {
		t.Fatalf("expected reason OverBudget, got %q", cond.Reason)
	}
}

func TestPodsInOtherNamespaceDoNotCount(t *testing.T) {
	b := newBudget("web", "main", "1", "1Gi")
	pods := []corev1.Pod{
		podWithRequests("other", "x", 8000, 8000),
	}
	status := reconcileOnce(t, b, pods)
	if !status.WithinBudget {
		t.Fatalf("expected within budget when pods live elsewhere, got %+v", status)
	}
}

func TestSucceededPodsDoNotCount(t *testing.T) {
	b := newBudget("web", "main", "100m", "64Mi")
	done := podWithRequests("web", "done", 10_000, 10_000)
	done.Status.Phase = corev1.PodSucceeded
	status := reconcileOnce(t, b, []corev1.Pod{done})
	if !status.WithinBudget {
		t.Fatalf("expected succeeded pod not to count, got %+v", status)
	}
}

func TestLastCalculatedIsSet(t *testing.T) {
	b := newBudget("web", "main", "4", "8Gi")
	status := reconcileOnce(t, b, nil)
	if status.LastCalculated == nil {
		t.Fatal("expected LastCalculated to be set")
	}
	if time.Since(status.LastCalculated.Time) > time.Minute {
		t.Fatal("LastCalculated looks stale")
	}
}

func TestDeleteMissingBudgetIsIgnored(t *testing.T) {
	s := testScheme(t)
	cli := fake.NewClientBuilder().WithScheme(s).Build()
	r := &ResourceBudgetReconciler{Client: cli, Scheme: s, Recorder: record.NewFakeRecorder(10)}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "web", Name: "ghost"},
	})
	if err != nil {
		t.Fatalf("missing budget should be a no-op, got %v", err)
	}
}

func TestBudgetNamespaceIsUsedInStatus(t *testing.T) {
	b := newBudget("payments", "main", "2", "2Gi")
	pods := []corev1.Pod{podWithRequests("payments", "api", 100, 128)}
	status := reconcileOnce(t, b, pods)
	if status.Pods != 1 {
		t.Fatalf("expected 1 pod, got %d", status.Pods)
	}
}

var _ = client.IgnoreNotFound
