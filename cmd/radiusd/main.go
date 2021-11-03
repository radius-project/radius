// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"flag"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	zap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Azure/radius/pkg/localenv"
	"github.com/Azure/radius/pkg/radlogger"
)

type startupOpts struct {
	MetricsAddr     string
	HealthProbeAddr string
}

func main() {
	opts := getStartupOpts()
	zap.New()
	log, flushLogs, err := radlogger.NewLogger("radiusd")
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
	defer flushLogs()

	certDir := os.Getenv("TLS_CERT_DIR")
	controllerOptions := ctrl.Options{
		MetricsBindAddress:     opts.MetricsAddr,
		HealthProbeBindAddress: opts.HealthProbeAddr,
		LeaderElection:         false,
		CertDir:                certDir,
		Logger:                 log,
	}

	cms, err := localenv.NewControllerManagerService(log, controllerOptions)
	if err != nil {
		log.Error(err, "unable to create controller manager service")
		os.Exit(2)
	}

	ctx := ctrl.SetupSignalHandler()

	log.Info("starting controller manager...")
	if err := cms.Run(ctx); err != nil {
		log.Error(err, "failed to start controller manager")
		os.Exit(3)
	}
}

func getStartupOpts() *startupOpts {
	opts := startupOpts{}

	flag.StringVar(&opts.MetricsAddr, "metrics-bind-address", ":43590", "The address the metric endpoint binds to.")
	flag.StringVar(&opts.HealthProbeAddr, "health-probe-bind-address", ":43591", "The address the probe endpoint binds to.")
	flag.Parse()
	return &startupOpts{}
}
