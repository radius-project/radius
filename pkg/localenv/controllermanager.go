// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"os"

	"github.com/go-logr/logr"

	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	radcontroller "github.com/Azure/radius/pkg/kubernetes/controllers/radius"
)

var (
	scheme *apiruntime.Scheme
)

type ControllerManagerService struct {
	Manager ctrl.Manager
}

func NewControllerManagerService(log logr.Logger, metricsAddr string, healthProbeAddr string) (*ControllerManagerService, error) {
	if scheme == nil {
		scheme = apiruntime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		// Probably don't need all the Radius CRDs for local development... but for now this will do
		utilruntime.Must(radiusv1alpha3.AddToScheme(scheme))
	}

	certDir := os.Getenv("TLS_CERT_DIR")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: healthProbeAddr,
		LeaderElection:         false,
		CertDir:                certDir,
		Logger:                 log,
	})
	if err != nil {
		return nil, err
	}

	err = (&radcontroller.ExecutableReconciler{
		Client: mgr.GetClient(),
		Log:    log.WithName("controllers").WithName("Executable"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	if err != nil {
		return nil, err
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
