// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/mitchellh/go-homedir"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/Azure/radius/pkg/hosting"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/localenv"
	"github.com/Azure/radius/pkg/model/local"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type startupOpts struct {
	Clean           bool
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
	CannotCreateWorkingDirectory  RadiusdExitCode = 6
	ForcedShutdown                RadiusdExitCode = 99
)

func main() {
	workingDir := getWorkingDir()
	exeDir := getExeDir()

	opts := getStartupOpts()
	log, err := configureLogger()

	abort := func(err error, msg string, code RadiusdExitCode) {
		log.Error(err, msg)
		os.Exit(int(code))
	}

	err = os.MkdirAll(workingDir, os.FileMode(0755))
	if err != nil {
		abort(err, "unable to create working directory", CannotCreateWorkingDirectory)
	}

	apiServerStarted := make(chan struct{})
	apiServerReady := make(chan struct{})

	scheme := apiruntime.NewScheme()
	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	utilruntime.Must(bicepv1alpha3.AddToScheme(scheme))

	certDir := os.Getenv("TLS_CERT_DIR")
	controllerOptions := localenv.ControllerOptions{
		Options: ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     opts.MetricsAddr,
			HealthProbeBindAddress: opts.HealthProbeAddr,
			LeaderElection:         false,
			CertDir:                certDir,
			Logger:                 log,
		},
		CRDDirectory:   getCRDDir(),
		KubeConfigPath: getKubeConfigPath(),
		Start:          apiServerReady,
	}

	cms, err := localenv.NewControllerManagerService(log, controllerOptions)
	if err != nil {
		abort(err, "unable to create controller manager service", CannotCreateControllerManager)
	}

	kcpOptions := localenv.KcpOptions{
		Clean:            opts.Clean,
		WorkingDirectory: workingDir,
		KubeConfigPath:   getKubeConfigPath(),
		Started:          apiServerStarted,
	}

	kcpRunner, err := localenv.NewKcpRunner(log, exeDir, kcpOptions)
	if err != nil {
		abort(err, "unable to create KCP runner service", CannotCreateKcpRunner)
	}

	apiServerOptions := localenv.APIServerExtensionOptions{
		AppModel:       local.NewLocalModel(),
		KubeConfigPath: getKubeConfigPath(),
		Scheme:         scheme,
		Start:          apiServerReady,
	}
	apiServer := localenv.NewAPIServerExtension(log, apiServerOptions)

	host := hosting.Host{
		Services: []hosting.Service{
			kcpRunner,
			cms,
			apiServer,
		},
	}
	// Create a channel to handle the shutdown
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), log))

	err = kcpRunner.EnsureKcpExecutable(ctx)
	if err != nil {
		abort(err, "unable to ensure KCP executable", CannotDownloadKcpExecutable)
	}

	log.Info("Starting server...")
	stopped, serviceErrors := host.RunAsync(ctx)

	go func() {
		<-apiServerStarted
		log.Info("Applying CRDs...")
		localenv.ApplyCRDs(ctx, getKubeConfigPath(), getCRDDir())
		log.Info("CRDs Ready")
		close(apiServerReady)
	}()

	select {
	// Normal shutdown
	case <-exitCh:
		log.Info("Shutdown requested..")
		cancel()

	// A service terminated with a failure. Details of the failure have already been logged.
	case <-serviceErrors:
		log.Info("One of the services failed. Shutting down...")
		cancel()
	}

	// Finished shutting down. An error returned here is a failure to terminate
	// gracefully, so just crash if that happens.
	err = <-stopped
	if err != nil {
		abort(err, "Graceful shutdown failed. Aborting...", ForcedShutdown)
	}
}

func getStartupOpts() *startupOpts {
	opts := startupOpts{}

	flag.BoolVar(&opts.Clean, "clean", false, "Clean server state")
	flag.StringVar(&opts.MetricsAddr, "metrics-bind-address", ":43590", "The address the metric endpoint binds to.")
	flag.StringVar(&opts.HealthProbeAddr, "health-probe-bind-address", ":43591", "The address the probe endpoint binds to.")
	flag.Parse()
	return &startupOpts{}
}

func configureLogger() (logr.Logger, error) {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	config.EncoderConfig.CallerKey = zapcore.OmitKey
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// Use a really simple date/time format
	config.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("Jan 2 15:04:05"))
	}

	zlogger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize zap logger: %w", err)
	}

	logger := zapr.NewLogger(zlogger).WithName("radiusd")
	return logger, nil
}

func getWorkingDir() string {
	home, err := homedir.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not determine user's home directory: %v", err)
		os.Exit(int(KcpPathNotFound))
	}
	exeDir := path.Join(home, ".rad", "server")
	return exeDir
}

func getExeDir() string {
	home, err := homedir.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not determine user's home directory: %v", err)
		os.Exit(int(KcpPathNotFound))
	}
	exeDir := path.Join(home, ".rad", "bin")
	return exeDir
}

func getKubeConfigPath() string {
	home, err := homedir.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not determine user's home directory: %v", err)
		os.Exit(int(KcpPathNotFound))
	}
	exeDir := path.Join(home, ".rad", "server", ".kcp", "data", "admin.kubeconfig")
	return exeDir
}

func getCRDDir() string {
	home, err := homedir.Dir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not determine user's home directory: %v", err)
		os.Exit(int(KcpPathNotFound))
	}
	exeDir := path.Join(home, ".rad", "crd")
	return exeDir
}
