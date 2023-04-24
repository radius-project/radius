// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	metricsprovider "github.com/project-radius/radius/pkg/metrics/provider"
	profilerprovider "github.com/project-radius/radius/pkg/profiler/provider"
	"github.com/project-radius/radius/pkg/trace"
	"github.com/project-radius/radius/pkg/ucp/config"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
	sprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// ProviderConfig includes the resource provider configuration.
type ProviderConfig struct {
	Env              EnvironmentOptions                       `yaml:"environment"`
	Identity         IdentityOptions                          `yaml:"identity"`
	StorageProvider  dataprovider.StorageProviderOptions      `yaml:"storageProvider"`
	SecretProvider   sprovider.SecretProviderOptions          `yaml:"secretProvider"`
	QueueProvider    qprovider.QueueProviderOptions           `yaml:"queueProvider"`
	Server           *ServerOptions                           `yaml:"server,omitempty"`
	WorkerServer     *WorkerServerOptions                     `yaml:"workerServer,omitempty"`
	MetricsProvider  metricsprovider.MetricsProviderOptions   `yaml:"metricsProvider"`
	TracerProvider   trace.Options                            `yaml:"tracerProvider"`
	ProfilerProvider profilerprovider.ProfilerProviderOptions `yaml:"profilerProvider"`
	UCP              config.UCPOptions                        `yaml:"ucp"`
	Logging          ucplog.LoggingOptions                    `yaml:"logging"`

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
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	MaxOperationConcurrency *int `yaml:"maxOperationConcurrency,omitempty"`
	// MaxOperationRetryCount is the maximum retry count to process async request operation.
	MaxOperationRetryCount *int `yaml:"maxOperationRetryCount,omitempty"`
}
