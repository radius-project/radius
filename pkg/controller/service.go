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

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/hosting"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/controller/reconciler"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(radappiov1alpha3.AddToScheme(scheme))
}

var _ hosting.Service = (*Service)(nil)

// Service implements the Kubernetes controller functionality.
type Service struct {
	// Options is the options for the controller.
	Options hostoptions.HostOptions

	// TLSConfigDir is the directory containing the TLS configuration.
	TLSCertDir string
}

// Name returns the name of the service.
func (*Service) Name() string {
	return "controller"
}

// Run implements the main functionality of the service.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	metricsAddr := "0" // Setting the address to 0 will disable the metrics server.
	if s.Options.Config.MetricsProvider.Enabled {
		metricsAddr = fmt.Sprintf(":%d", s.Options.Config.MetricsProvider.Prometheus.Port)
	}

	port := s.Options.Config.Server.Port
	healthProbePort := *s.Options.Config.WorkerServer.Port
	mgr, err := ctrl.NewManager(s.Options.K8sConfig, ctrl.Options{
		Logger: logger,
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: fmt.Sprintf(":%d", healthProbePort),
		LeaderElection:         false,
		LeaderElectionID:       "c85b2113.radapp.io",
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    port,
			CertDir: s.TLSCertDir,
		})})
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	logger.Info("Registering controllers.")
	err = (&reconciler.RecipeReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("recipe-controller"),
		Radius:        reconciler.NewRadiusClient(s.Options.UCPConnection),
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup %s controller: %w", "Recipe", err)
	}
	err = (&reconciler.DeploymentReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("radius-deployment-controller"),
		Radius:        reconciler.NewRadiusClient(s.Options.UCPConnection),
	}).SetupWithManager(mgr)
	if err != nil {
		return fmt.Errorf("failed to setup %s controller: %w", "Deployment", err)
	}

	resourceDeploymentsClient, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          s.Options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(s.Options.UCPConnection),
	})
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

	if s.TLSCertDir == "" {
		logger.Info("Webhooks will be skipped. TLS certificates not present.")
	} else {
		logger.Info("Registering validating webhook.")
		if err = (&reconciler.RecipeWebhook{}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("failed to create recipe-webhook: %w", err)
		}
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
