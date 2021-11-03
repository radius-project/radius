// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"

	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	localmodel "github.com/Azure/radius/pkg/model/local"
)

var (
	scheme *apiruntime.Scheme
)

type ControllerManagerService struct {
	Manager ctrl.Manager
}

func NewControllerManagerService(log logr.Logger, options ctrl.Options) (*ControllerManagerService, error) {
	if scheme == nil {
		scheme = apiruntime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		// Probably don't need all the Radius CRDs for local development... but for now this will do
		utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return nil, fmt.Errorf("unable to create controller manager: %w", err)
	}

	model := radcontroller.NewLocalModel()
	appmodel := localmodel.NewLocalModel(mgr.GetClient())

	unstructuredClient, err := dynamic.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		return nil, fmt.Errorf("unable to create dynamic client: %w", err)
	}

	// Use discovery client to determine GVR for each resource type
	dc, err := discovery.NewDiscoveryClientForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		return nil, fmt.Errorf("unable to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	radCtrlOptions := radcontroller.Options{
		AppModel:      appmodel,
		Client:        mgr.GetClient(),
		Dynamic:       unstructuredClient,
		Scheme:        mgr.GetScheme(),
		Log:           ctrl.Log,
		Recorder:      mgr.GetEventRecorderFor("radius"),
		RestConfig:    ctrl.GetConfigOrDie(),
		RestMapper:    mapper,
		ResourceTypes: model.GetReconciledTypes(),
		WatchedTypes:  model.GetWatchedTypes(),
		SkipWebhooks:  os.Getenv("SKIP_WEBHOOKS") == "true",
	}

	controller := radcontroller.NewRadiusController(&radCtrlOptions)
	err = controller.SetupWithManager(mgr)
	if err != nil {
		return nil, fmt.Errorf("unable to create radius controller: %w", err)
	}

	// Additional controllers for local development environment
	// CONSIDER making this part of radcontroller.NewRadiusController()
	err = (&radcontroller.ExecutableReconciler{
		Client: mgr.GetClient(),
		Log:    log.WithName("controllers").WithName("Executable"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return nil, err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	return &ControllerManagerService{
		Manager: mgr,
	}, nil
}

func (cms *ControllerManagerService) Name() string {
	return "Local deployment environment controller manager"
}

func (cms *ControllerManagerService) Run(ctx context.Context) error {
	return cms.Manager.Start(ctx)
}
