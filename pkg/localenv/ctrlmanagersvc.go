// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	localmodel "github.com/Azure/radius/pkg/model/local"
)

type ControllerOptions struct {
	ctrl.Options
	CRDDirectory   string
	KubeConfigPath string
	Start          <-chan struct{}
}

type ControllerManagerService struct {
	log     logr.Logger
	options ControllerOptions
	Manager ctrl.Manager
}

func NewControllerManagerService(log logr.Logger, options ControllerOptions) (*ControllerManagerService, error) {
	return &ControllerManagerService{
		log:     log,
		options: options,
	}, nil
}

func (cms *ControllerManagerService) Name() string {
	return "Local deployment environment controller manager"
}

func (cms *ControllerManagerService) Run(ctx context.Context) error {
	log := cms.log

	log.Info("Controller waiting for API Server...")
	<-cms.options.Start

	config, err := GetRESTConfig(cms.options.KubeConfigPath)
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(config, cms.options.Options)
	if err != nil {
		return fmt.Errorf("unable to create controller manager: %w", err)
	}

	model := radcontroller.NewLocalModel()
	appmodel := localmodel.NewLocalModel()

	unstructuredClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("unable to create dynamic client: %w", err)
	}

	// Use discovery client to determine GVR for each resource type
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("unable to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	radCtrlOptions := radcontroller.Options{
		AppModel:      appmodel,
		Client:        mgr.GetClient(),
		Dynamic:       unstructuredClient,
		Scheme:        mgr.GetScheme(),
		Log:           ctrl.Log,
		Recorder:      mgr.GetEventRecorderFor("radius"),
		RestConfig:    mgr.GetConfig(),
		RestMapper:    mapper,
		ResourceTypes: model.GetReconciledTypes(),
		WatchedTypes:  model.GetWatchedTypes(),
		SkipWebhooks:  true,
	}

	controller := radcontroller.NewRadiusController(&radCtrlOptions)
	err = controller.SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("unable to create radius controller: %w", err)
	}

	// Additional controllers for local development environment
	// CONSIDER making this part of radcontroller.NewRadiusController()
	err = (&radcontroller.ExecutableReconciler{
		Client: mgr.GetClient(),
		Log:    log.WithName("controllers").WithName("Executable"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	// Create the default namespace so we have something to work with.
	ns := unstructured.Unstructured{}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")
	ns.SetName("default")
	err = mgr.GetClient().Patch(ctx, &ns, controller_runtime.Apply, controller_runtime.FieldOwner("radiusd"))
	if err != nil {
		return fmt.Errorf("failed to create default namespace: %w", err)
	}

	cms.Manager = mgr
	return cms.Manager.Start(ctx)
}
