// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
)

// ProviderConfig includes the resource provider configuration.
type ProviderConfig struct {
	Env             EnvironmentOptions                  `yaml:"environment"`
	Identity        IdentityOptions                     `yaml:"identity"`
	StorageProvider dataprovider.StorageProviderOptions `yaml:"storageProvider"`
	QueueProvider   qprovider.QueueProviderOptions      `yaml:"queueProvider"`
	Server          *ServerOptions                      `yaml:"server,omitempty"`
	WorkerServer    *WorkerServerOptions                `yaml:"workerServer,omitempty"`
	MetricsProvider provider.MetricsProviderOptions     `yaml:"metricsProvider"`
	UCP             UCPOptions                          `yaml:"ucp"`

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

// UCPConnectionKind describes how to connect to UCP.
type UCPConnectionKind string

const (
	// UCPConnectionKubernetes describes a connection to UCP through the Kubernetes API server. This is used
	// when the services are deployed to Kubernetes.
	UCPConnectionKubernetes = "kubernetes"

	// UCPConnectionDirect describes a direct connection to UCP. This is used in development scenarios.
	UCPConnectionDirect = "direct"
)

// UCPOptions describes the configuration for communicating and authenticating with UCP.
type UCPOptions struct {
	// Connection describes the type of connection. This is a required field.
	Connection string `yaml:"connection"`

	// Direct describes the configuration options for communicating directly with UCP. This is a required
	// field when using 'Connection == direct'
	Direct *UCPDirectOptions `yaml:"direct,omitempty"`
}

// UCPDirectOptions describes the configuration options for communicating directly with UCP.
type UCPDirectOptions struct {
	// BaseURI specifies the Base URI of the UCP service. This is a required field.
	BaseURI string `yaml:"baseURI,omitempty"`
}

// WorkerServerOptions includes the worker server options.
type WorkerServerOptions struct {
	// Port is the localhost port which provides the system-level info, such as healthprobe and metric port
	Port *int32 `yaml:"port,omitempty"`
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	MaxOperationConcurrency *int `yaml:"maxOperationConcurrency,omitempty"`
	// MaxOperationRetryCount is the maximum retry count to process async request operation.
	MaxOperationRetryCount *int `yaml:"maxOperationRetryCount,omitempty"`
}
