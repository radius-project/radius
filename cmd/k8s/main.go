/*
Copyright 2021.
*/

package main

import (
	"context"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/restmapper"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	bicepcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/bicep"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	"github.com/Azure/radius/pkg/model/resourcesv1alpha3"
	//+kubebuilder:scaffold:imports
)

const (
	CacheKeyController = "metadata.controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))

	utilruntime.Must(bicepv1alpha3.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
	_ = scheme.AddConversionFunc(&radiusv1alpha3.Resource{}, &resourcesv1alpha3.GenericResource{}, converters.ConvertComponentToInternal)
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

	if err = (&radcontroller.ApplicationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Application"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Application")
		os.Exit(1)
	}

	// Index deployments by the owner (any resource besides application)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.Deployment{}, CacheKeyController, extractOwnerKey)
	if err != nil {
		setupLog.Error(err, "unable to create ownership of deployments", "controller")
		os.Exit(1)
	}

	// Index services by the owner (any resource besides application)
	err = mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Service{}, CacheKeyController, extractOwnerKey)
	if err != nil {
		setupLog.Error(err, "unable to create ownership of services", "controller")
		os.Exit(1)
	}

	resourceTypes := []struct {
		client.Object
		client.ObjectList
	}{
		{&radiusv1alpha3.ContainerComponent{}, &radiusv1alpha3.ContainerComponentList{}},
		{&radiusv1alpha3.DaprIOInvokeRoute{}, &radiusv1alpha3.DaprIOInvokeRouteList{}},
		{&radiusv1alpha3.DaprIOPubSubComponent{}, &radiusv1alpha3.DaprIOPubSubComponentList{}},
		{&radiusv1alpha3.DaprIOStateStoreComponent{}, &radiusv1alpha3.DaprIOStateStoreComponentList{}},
		{&radiusv1alpha3.GrpcRoute{}, &radiusv1alpha3.GrpcRouteList{}},
		{&radiusv1alpha3.HttpRoute{}, &radiusv1alpha3.HttpRouteList{}},
		{&radiusv1alpha3.MongoDBComponent{}, &radiusv1alpha3.MongoDBComponentList{}},
		{&radiusv1alpha3.RabbitMQComponent{}, &radiusv1alpha3.RabbitMQComponentList{}},
		{&radiusv1alpha3.RedisComponent{}, &radiusv1alpha3.RedisComponentList{}},
	}

	unstructuredClient, err := dynamic.NewForConfig(ctrl.GetConfigOrDie())

	// Use discovery client to determine GVR for each resource type
	dc, err := discovery.NewDiscoveryClientForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "unable to create discovery client")
		os.Exit(1)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	for _, resourceType := range resourceTypes {
		gvks, _, err := scheme.ObjectKinds(resourceType.Object)
		if err != nil {
			setupLog.Error(err, "unable to get GVK for component type", "componentType", resourceType.Object)
			os.Exit(1)
		}

		for _, gvk := range gvks {
			if gvk.GroupVersion() != radiusv1alpha3.GroupVersion {
				continue
			}

			// Get GVR for corresponding component.
			gvr, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)

			if err != nil {
				setupLog.Error(err, "unable to get GVR for component", "component", gvk.Kind)
				os.Exit(1)
			}

			if err = (&radcontroller.ResourceReconciler{
				Client:  mgr.GetClient(),
				Log:     ctrl.Log.WithName("controllers").WithName(resourceType.GetName()),
				Scheme:  mgr.GetScheme(),
				Dynamic: unstructuredClient,
				GVR:     gvr.Resource,
			}).SetupWithManager(mgr, resourceType.Object, resourceType.ObjectList); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "Component")
				os.Exit(1)
			}
		}
	}

	if err = (&bicepcontroller.DeploymentTemplateReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("DeploymentTemplate"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DeploymentTemplate")
		os.Exit(1)
	}

	// TODO webhook per resource type, currently doesn't work.
	if os.Getenv("SKIP_WEBHOOKS") != "true" {
		if err = (&radiusv1alpha3.Application{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Application")
			os.Exit(1)
		}
		if err = (&radiusv1alpha3.Resource{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Component")
			os.Exit(1)
		}
		if err = (&bicepv1alpha3.DeploymentTemplate{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "DeploymentTemplate")
			os.Exit(1)
		}
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func extractOwnerKey(obj client.Object) []string {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		return nil
	}

	// Assume all other types besides Application are owned by us with radius.
	if owner.APIVersion != radiusv1alpha3.GroupVersion.String() || owner.Kind == "Application" {
		return nil
	}

	return []string{owner.Name}
}
