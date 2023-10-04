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
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
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

	metricsAddr := "0" // Disable metrics
	if s.Options.Config.MetricsProvider.Prometheus.Enabled {
		metricsAddr = fmt.Sprintf(":%d", s.Options.Config.MetricsProvider.Prometheus.Port)
	}

	port := s.Options.Config.Server.Port
	mgr, err := ctrl.NewManager(s.Options.K8sConfig, ctrl.Options{
		Logger:                 logger,
		CertDir:                s.TLSCertDir,
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   port,
		HealthProbeBindAddress: fmt.Sprintf(":%d", port),
		LeaderElection:         false,
		LeaderElectionID:       "c85b2113.radapp.io",
	})
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	logger.Info("Registering controllers.")

	if s.TLSCertDir == "" {
		logger.Info("Webhooks will be skipped. TLS certificates not present.")
	} else {
		logger.Info("Registering webhooks.")
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
