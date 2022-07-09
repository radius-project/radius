// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	"github.com/project-radius/radius/pkg/client/azuread"
	kubeenv "github.com/project-radius/radius/pkg/client/kubernetes"
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
)

// ProviderConfig includes the resource provider configuration.
type ProviderConfig struct {
	Environment     EnvironmentOptions                  `yaml:"environment"`
	AzureAD         *azuread.Options                    `yaml:"azureAd,omitempty"`
	Kubernetes      *kubeenv.Options                    `yaml:"kubernetes,omitempty"`
	StorageProvider dataprovider.StorageProviderOptions `yaml:"storageProvider"`
	QueueProvider   qprovider.QueueProviderOptions      `yaml:"queueProvider"`
	Server          *ServerOptions                      `yaml:"server,omitempty"`
	WorkerServer    *WorkerServerOptions                `yaml:"workerServer,omitempty"`
	MetricsProvider provider.MetricsProviderOptions     `yaml:"metricsProvider"`

	// FeatureFlags includes the list of feature flags.
	FeatureFlags []string `yaml:"featureFlags"`
}

// ServerOptions includes http server bootstrap options.
type ServerOptions struct {
	Host     string               `yaml:"host"`
	Port     int                  `yaml:"port"`
	PathBase string               `yaml:"pathBase,omitempty"`
	AuthType AuthentificationType `yaml:"authType,omitempty"`
	// ArmMetadataEndpoints provides the client certification to authenticate between ARM and RP.
	// https://armwiki.azurewebsites.net/authorization/AuthenticateBetweenARMandRP.html
	ArmMetadataEndpoint string `yaml:"armMetadataEndpoint,omitempty"`
	// EnableAuth when set the arm client authetication will be performed
	EnableArmAuth bool `yaml:"enableArmAuth,omitempty"`
}

// WorkerServerOptions includes the worker server options.
type WorkerServerOptions struct {
	// Port is the localhost port which provides the system-level info, such as healthprobe and metric port
	Port *int32 `yaml:"port,omitempty"`
}
