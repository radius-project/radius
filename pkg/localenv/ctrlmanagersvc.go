// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/go-logr/logr"

	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
	localmodel "github.com/Azure/radius/pkg/model/local"
)

var (
	scheme *apiruntime.Scheme
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

	if scheme == nil {
		scheme = apiruntime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		// Probably don't need all the Radius CRDs for local development... but for now this will do
		utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
		utilruntime.Must(bicepv1alpha3.AddToScheme(scheme))
	}
	cms.options.Options.Scheme = scheme

	log.Info("Waiting for API Server...")
	<-cms.options.Start
	log.Info("API Server Ready")

	log.Info("Applying CRDs...")
	applyCRDsWithRetries(context.TODO(), cms.options.KubeConfigPath, cms.options.CRDDirectory)
	log.Info("CRDs Ready")

	rawconfig, err := clientcmd.LoadFromFile(cms.options.KubeConfigPath)
	if err != nil {
		return fmt.Errorf("unable to get Kubernetes client config: %w", err)
	}

	context := rawconfig.Contexts[rawconfig.CurrentContext]
	if context == nil {
		return fmt.Errorf("kubernetes context '%s' could not be found", rawconfig.CurrentContext)
	}

	clientconfig := clientcmd.NewNonInteractiveClientConfig(*rawconfig, rawconfig.CurrentContext, nil, nil)
	merged, err := clientconfig.ClientConfig()
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(merged, cms.options.Options)
	if err != nil {
		return fmt.Errorf("unable to create controller manager: %w", err)
	}

	model := radcontroller.NewLocalModel()
	appmodel := localmodel.NewLocalModel(mgr.GetClient())

	unstructuredClient, err := dynamic.NewForConfig(merged)
	if err != nil {
		return fmt.Errorf("unable to create dynamic client: %w", err)
	}

	// Use discovery client to determine GVR for each resource type
	dc, err := discovery.NewDiscoveryClientForConfig(merged)
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
		RestConfig:    ctrl.GetConfigOrDie(),
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

	cms.Manager = mgr
	return cms.Manager.Start(ctx)
}

func applyCRDsWithRetries(ctx context.Context, kubeConfigPath string, crdDirectory string) error {
	var err error
	for i := 0; i < 20; i++ {
		err = applyCRDs(ctx, kubeConfigPath, crdDirectory)
		if err == nil {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return err
}

func applyCRDs(ctx context.Context, kubeConfigPath string, crdDirectory string) error {
	executable := "kubectl"

	args := []string{
		"apply",
		"-f", crdDirectory,
		"--kubeconfig", kubeConfigPath,
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	args = []string{
		"wait",
		"--for", "condition=established",
		"-f", crdDirectory,
		"--kubeconfig", kubeConfigPath,
	}
	cmd = exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
