// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/Azure/radius/pkg/hosting"
	"github.com/Azure/radius/pkg/localenv"
	"github.com/Azure/radius/pkg/radlogger"
)

type startupOpts struct {
	MetricsAddr     string
	HealthProbeAddr string
}

type RadiusdExitCode int

const (
	KcpPathNotFound               RadiusdExitCode = 1
	CannotCreateLogger            RadiusdExitCode = 2
	CannotCreateControllerManager RadiusdExitCode = 3
	CannotCreateKcpRunner         RadiusdExitCode = 4
	CannotDownloadKcpExecutable   RadiusdExitCode = 5
	ForcedShutdown                RadiusdExitCode = 99
)

func main() {
	exeDir := getExeDir()

	opts := getStartupOpts()
	log, flushLogs, err := radlogger.NewLogger("radiusd")
	if err != nil {
		println(err.Error())
		os.Exit(int(CannotCreateLogger))
	}
	defer flushLogs()

	abort := func(err error, msg string, code RadiusdExitCode) {
		log.Error(err, msg)
		flushLogs()
		os.Exit(int(code))
	}

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
		abort(err, "unable to create controller manager service", CannotCreateControllerManager)
	}

	kcpRunner, err := localenv.NewKcpRunner(exeDir, nil)
	if err != nil {
		abort(err, "unable to create KCP runner service", CannotCreateKcpRunner)
	}

	host := hosting.Host{
		Services: []hosting.Service{
			kcpRunner,
			cms,
		},
	}
	// Create a channel to handle the shutdown
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, os.Kill, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), log))

	err = kcpRunner.EnsureKcpExecutable(ctx)
	if err != nil {
		abort(err, "unable to ensure KCP executable", CannotDownloadKcpExecutable)
	}

	stopped, serviceErrors := host.RunAsync(ctx)

	for {
		select {
		// Normal shutdown
		case <-exitCh:
			log.Info("Shutdown requested..")
			cancel()

		// A service terminated with a failure. Details of the failure have already been logged.
		case <-serviceErrors:
			log.Info("One of the services failed. Shutting down...")
			cancel()

		// Finished shutting down. An error returned here is a failure to terminate
		// gracefully, so just crash if that happens.
		case err := <-stopped:
			if err != nil {
				abort(err, "Graceful shutdown failed. Aborting...", ForcedShutdown)
			}
		}
	}
}

func getStartupOpts() *startupOpts {
	opts := startupOpts{}

	flag.StringVar(&opts.MetricsAddr, "metrics-bind-address", ":43590", "The address the metric endpoint binds to.")
	flag.StringVar(&opts.HealthProbeAddr, "health-probe-bind-address", ":43591", "The address the probe endpoint binds to.")
	flag.Parse()
	return &startupOpts{}
}

func getExeDir() string {
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(int(KcpPathNotFound))
	}
	exeDir := path.Dir(exePath)
	return exeDir
}
