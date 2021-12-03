/*
Copyright 2021.
*/

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/restmapper"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Azure/radius/pkg/hosting"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/kubernetes/apiserver"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	kubernetesmodel "github.com/Azure/radius/pkg/model/kubernetes"
	"github.com/go-logr/logr"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(gatewayv1alpha1.AddToScheme(scheme))

	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))

	utilruntime.Must(bicepv1alpha3.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Get certificate from volume mounted environment variable
	certDir := os.Getenv("TLS_CERT_DIR")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "4fe3568c.radius.dev",
		CertDir:                certDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	unstructuredClient, err := dynamic.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to create dynamic client")
		os.Exit(1)
	}

	// Use discovery client to determine GVR for each resource type
	dc, err := discovery.NewDiscoveryClientForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to create discovery client")
		os.Exit(1)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	options := radcontroller.Options{
		AppModel:      kubernetesmodel.NewKubernetesModel(mgr.GetClient()),
		Client:        mgr.GetClient(),
		Dynamic:       unstructuredClient,
		Scheme:        mgr.GetScheme(),
		Log:           ctrl.Log,
		Recorder:      mgr.GetEventRecorderFor("radius"),
		RestConfig:    ctrl.GetConfigOrDie(),
		RestMapper:    mapper,
		ResourceTypes: radcontroller.DefaultResourceTypes,
		WatchTypes:    radcontroller.DefaultWatchTypes,
		SkipWebhooks:  os.Getenv("SKIP_WEBHOOKS") == "true",
	}

	controller := radcontroller.NewRadiusController(&options)
	err = controller.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create radius controller")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	apiServerOptions := apiserver.APIServerExtensionOptions{
		KubeConfig: ctrl.GetConfigOrDie(),
		Scheme:     scheme,
		TLSCertDir: certDir,
		Port:       7443,
	}
	apiServer := apiserver.NewAPIServerExtension(setupLog, apiServerOptions)

	host := hosting.Host{
		Services: []hosting.Service{
			apiServer,
		},
	}

	// Create a channel to handle the shutdown
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), ctrl.Log))

	setupLog.Info("Starting server...")
	stopped, apiServiceErrors := host.RunAsync(ctx)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	select {
	// Normal shutdown
	case <-exitCh:
		setupLog.Info("Shutdown requested..")
		cancel()
	// A service terminated with a failure. Details of the failure have already been logged.
	case <-apiServiceErrors:
		setupLog.Info("One of the services failed. Shutting down...")
		cancel()
	}

	// Finished shutting down. An error returned here is a failure to terminate
	// gracefully, so just crash if that happens.
	err = <-stopped
	if err != nil {
		return
	}
}
