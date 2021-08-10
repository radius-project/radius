// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"flag"
	"os"

	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha1"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	zap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme    = runtime.NewScheme()
	daemonLog = ctrl.Log.WithName("radiusd")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Probably don't need all the Radius CRDs for local development... but for now this will do
	utilruntime.Must(radiusv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":43590", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":43591", "The address the probe endpoint binds to.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	certDir := os.Getenv("TLS_CERT_DIR")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
		CertDir:                certDir,
	})
	if err != nil {
		daemonLog.Error(err, "unable to create controller manager")
		os.Exit(1)
	}

	if err = (&radcontroller.ExecutableReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Executable"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		daemonLog.Error(err, "unable to create controller", "controller", "Executable")
		os.Exit(1)
	}

	daemonLog.Info("starting controller manager...")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		daemonLog.Error(err, "failed to start controller manager")
		os.Exit(2)
	}
}
