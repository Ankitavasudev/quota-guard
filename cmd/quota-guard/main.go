package main

import (
	"flag"
	"os"

	"github.com/Ankitavasudev/quota-guard/api/v1alpha1"
	"github.com/Ankitavasudev/quota-guard/internal/controller"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	// Register core types explicitly so the scheme works even if the
	// generated code never runs.
	_ = corev1.AddToScheme(scheme)
}

func main() {
	var metricsAddr string
	var zapOpts zap.Options
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metrics endpoint binds to.")
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))

	cfg, err := ctrl.GetConfig()
	if err != nil {
		os.Stderr.WriteString("unable to load kubeconfig: " + err.Error() + "\n")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Metrics:            metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: ":8081",
	})
	if err != nil {
		os.Stderr.WriteString("unable to start manager: " + err.Error() + "\n")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		os.Stderr.WriteString("unable to add health check: " + err.Error() + "\n")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		os.Stderr.WriteString("unable to add ready check: " + err.Error() + "\n")
		os.Exit(1)
	}

	if err := (&controller.ResourceBudgetReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("quota-guard"),
	}).SetupWithManager(mgr); err != nil {
		os.Stderr.WriteString("unable to set up controller: " + err.Error() + "\n")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	if err := mgr.Start(ctx); err != nil {
		os.Stderr.WriteString("manager exited with error: " + err.Error() + "\n")
		os.Exit(1)
	}
}
