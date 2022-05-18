// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/telemetry/metrics"
)

// ProviderConfig includes the resource provider configuration.
type ProviderConfig struct {
	CloudEnv        CloudEnvironmentOptions             `yaml:"cloudEnvironment"`
	Identity        IdentityOptions                     `yaml:"identity"`
	StorageProvider dataprovider.StorageProviderOptions `yaml:"storageProvider"`
	Server          *ServerOptions                      `yaml:"server,omitempty"`
	WorkerServer    *WorkerServerOptions                `yaml:"workerServer,omitempty"`
	MetricsProvider metrics.MetricsOptions              `yaml:"metricsProvider"`

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
	// SystemHTTPServerPort is the localhost port which provides the system-level info, such as healthprobe and metric port
	SystemHTTPServerPort *int32 `yaml:"systemHttpServerPort,omitempty"`
}
