// envtest-demo spins up a real local control plane (etcd + kube-apiserver),
// installs the ResourceBudget CRD, runs the quota-guard reconciler against it,
// and prints the status as pods are added and removed.
//
// It is not a unit test — it is a reproducible demo of the operator working
// end to end. Requires the envtest binaries:
//
//	go run sigs.k8s.io/controller-runtime/tools/setup-envtest use 1.30.x --bin-dir ./envtest
//	KUBEBUILDER_ASSETS=./envtest/k8s/1.30.3-windows-amd64 go run ./cmd/envtest-demo

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appsv1alpha1 "github.com/Ankitavasudev/quota-guard/api/v1alpha1"
	"github.com/Ankitavasudev/quota-guard/internal/controller"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var assets = flag.String("assets", os.Getenv("KUBEBUILDER_ASSETS"), "path to envtest binaries (etcd, kube-apiserver)")

func main() {
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))

	if *assets == "" {
		fmt.Fprintln(os.Stderr, "set --assets or KUBEBUILDER_ASSETS to the envtest binaries directory")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start a real local control plane.
	testEnv := &envtest.Environment{
		BinaryAssetsDirectory: *assets,
		CRDDirectoryPaths:     []string{filepath.Join("config", "crd")},
	}
	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "start envtest:", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("\n--- stopping control plane ---")
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "stop envtest:", err)
		}
	}()
	fmt.Println("control plane up:", cfg.Host)

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))

	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintln(os.Stderr, "manager:", err)
		os.Exit(1)
	}
	if err := (&controller.ResourceBudgetReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("quota-guard"),
	}).SetupWithManager(mgr); err != nil {
		fmt.Fprintln(os.Stderr, "controller:", err)
		os.Exit(1)
	}
	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "manager exited:", err)
			os.Exit(1)
		}
	}()

	// 2. Create the namespace and a budget of 1 core / 2Gi.
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}}
	if err := cli.Create(ctx, ns); err != nil {
		fmt.Fprintln(os.Stderr, "create namespace:", err)
		os.Exit(1)
	}
	budget := &appsv1alpha1.ResourceBudget{
		ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: "main"},
		Spec: appsv1alpha1.ResourceBudgetSpec{
			CPU:    "1",
			Memory: "2Gi",
		},
	}
	if err := cli.Create(ctx, budget); err != nil {
		fmt.Fprintln(os.Stderr, "create budget:", err)
		os.Exit(1)
	}
	fmt.Printf("created budget: cpu=%s memory=%s\n\n", budget.Spec.CPU, budget.Spec.Memory)

	newPod := func(name string, cpuM, memMi int64) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: "demo", Name: name},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "app",
					Image: "registry.k8s.io/pause:3.10",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%dm", cpuM)),
							corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memMi)),
						},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	printStatus := func(label string) {
		b := &appsv1alpha1.ResourceBudget{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: "demo", Name: "main"}, b); err != nil {
			fmt.Fprintln(os.Stderr, "get budget:", err)
			return
		}
		s := b.Status
		fmt.Printf("=== %s ===\n", label)
		fmt.Printf("  pods counted:      %d\n", s.Pods)
		fmt.Printf("  observed CPU:      %s\n", s.ObservedCPU)
		fmt.Printf("  observed memory:   %s\n", s.ObservedMemory)
		fmt.Printf("  within budget:     %v\n", s.WithinBudget)
		for _, c := range s.Conditions {
			fmt.Printf("  condition:         %s=%s reason=%s\n", c.Type, c.Status, c.Reason)
		}
		fmt.Println()
	}

	waitForStatus := func(label string, wantWithin bool) {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			b := &appsv1alpha1.ResourceBudget{}
			if err := cli.Get(ctx, client.ObjectKey{Namespace: "demo", Name: "main"}, b); err == nil {
				if b.Status.LastCalculated != nil && b.Status.WithinBudget == wantWithin {
					printStatus(label)
					return
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		printStatus(label + " (TIMEOUT — showing last known status)")
	}

	// 3. Two pods that together blow the budget (1600m > 1000m).
	podA := newPod("api-a", 1000, 1024)
	podB := newPod("api-b", 600, 1024)
	for _, p := range []*corev1.Pod{podA, podB} {
		if err := cli.Create(ctx, p); err != nil {
			fmt.Fprintln(os.Stderr, "create pod:", err)
			os.Exit(1)
		}
	}
	waitForStatus("after api-a (1000m) + api-b (600m) = 1600m > 1-core budget", false)

	// 4. Delete the bigger pod; the budget should flip back to within.
	if err := cli.Delete(ctx, podA); err != nil {
		fmt.Fprintln(os.Stderr, "delete pod:", err)
		os.Exit(1)
	}
	waitForStatus("after deleting api-a (now 600m of 1 core)", true)

	fmt.Println("done — CRD, operator reconcile, status conditions all live on a real control plane.")
}
