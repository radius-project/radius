/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"net/http"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/components/hosting"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radappiov1alpha3.AddToScheme(scheme))
	utilruntime.Must(sourcev1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
}

var _ hosting.Service = (*Service)(nil)

// Service implements the Kubernetes controller functionality.
type Service struct {
	// Options is the options for the controller.
	Options hostoptions.HostOptions
}

// Name returns the name of the service.
func (*Service) Name() string {
	return "controller"
}

// Run implements the main functionality of the service.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	metrics := s.Options.Config.MetricsProvider
	proof, err := mtls.ProcessStartupProof(s.Options.Config.Authorization,
		metrics.Enabled && metrics.Prometheus != nil && metrics.Prometheus.Port > 0 &&
			metrics.Prometheus.Port <= 65535)
	if err != nil {
		return err
	}
	var extraHandlers map[string]http.Handler
	if proof != nil {
		extraHandlers = map[string]http.Handler{mtls.StartupProofPath: proof}
	}
	authorization := s.Options.Config.Authorization
	if authorization.EffectiveMode() != authz.ModeOff && authorization.EffectiveMTLSProvider() == "external" &&
		metrics.Enabled && metrics.Prometheus != nil && metrics.Prometheus.Port > 0 && metrics.Prometheus.Port <= 65535 {
		if s.Options.UCPConnection == nil {
			return fmt.Errorf("external identity evidence requires the configured UCP connection")
		}
		handler, stop, err := mtls.NewExternalIdentityProbe(ctx, authorization,
			s.Options.UCPConnection.Client(), s.Options.UCPConnection.Endpoint(), s.Options.K8sConfig)
		if err != nil {
			return err
		}
		defer stop()
		extraHandlers = map[string]http.Handler{mtls.InternalIdentityPath: handler}
	}

	metricsAddr := "0" // Setting the address to 0 will disable the metrics server.
	if s.Options.Config.MetricsProvider.Enabled {
		metricsAddr = fmt.Sprintf(":%d", s.Options.Config.MetricsProvider.Prometheus.Port)
	}

	healthProbePort := *s.Options.Config.WorkerServer.Port
	mgr, err := ctrl.NewManager(s.Options.K8sConfig, ctrl.Options{
		Logger: logger,
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress:   metricsAddr,
			ExtraHandlers: extraHandlers,
		},
		HealthProbeBindAddress: fmt.Sprintf(":%d", healthProbePort),
		LeaderElection:         false,
		LeaderElectionID:       "c85b2113.radapp.io",
	})
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	logger.Info("Registering controllers.")
	resourceDeploymentsClient, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          s.Options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(s.Options.UCPConnection),
	})
	if err != nil {
		return fmt.Errorf("failed to create resource deployments client: %w", err)
	}
	//nolint:staticcheck // SA1019: GetEventRecorderFor is deprecated but migration to new events API requires significant refactoring
	err = (&reconciler.DeploymentTemplateReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymenttemplate-controller"),
		Radius:                    reconciler.NewRadiusClient(s.Options.UCPConnection),
		ResourceDeploymentsClient: resourceDeploymentsClient,
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup %s controller: %w", "DeploymentTemplate", err)
	}
	//nolint:staticcheck // SA1019: GetEventRecorderFor is deprecated but migration to new events API requires significant refactoring
	err = (&reconciler.DeploymentResourceReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymentresource-controller"),
		Radius:                    reconciler.NewRadiusClient(s.Options.UCPConnection),
		ResourceDeploymentsClient: resourceDeploymentsClient,
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup %s controller: %w", "DeploymentResource", err)
	}

	err = (&reconciler.FluxController{
		Client:         mgr.GetClient(),
		ArchiveFetcher: reconciler.NewArchiveFetcher(),
		FileSystem:     filesystem.NewOSFS(),
		Bicep: &bicep.Impl{
			FileSystem: filesystem.NewOSFS(),
		},
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup %s controller: %w", "FluxController", err)
	}

	logger.Info("Registering health checks.")
	err = mgr.AddHealthzCheck("healthz", healthz.Ping)
	if err != nil {
		return fmt.Errorf("failed to start health check: %w", err)
	}

	err = mgr.AddReadyzCheck("readyz", healthz.Ping)
	if err != nil {
		return fmt.Errorf("failed to start ready check: %w", err)
	}

	logger.Info("Starting controller manager.")
	err = mgr.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start controller manager: %w", err)
	}

	logger.Info("Running.")
	return nil
}
