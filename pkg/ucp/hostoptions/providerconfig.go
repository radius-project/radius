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
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// UCPConfig includes the resource provider configuration.
type UCPConfig struct {
	StorageProvider  dataprovider.StorageProviderOptions      `yaml:"storageProvider"`
	Planes           []rest.Plane                             `yaml:"planes"`
	SecretProvider   provider.SecretProviderOptions           `yaml:"secretProvider"`
	MetricsProvider  metricsprovider.MetricsProviderOptions   `yaml:"metricsProvider"`
	ProfilerProvider profilerprovider.ProfilerProviderOptions `yaml:"profilerProvider"`
	QueueProvider    qprovider.QueueProviderOptions           `yaml:"queueProvider"`
	TracerProvider   trace.Options                            `yaml:"tracerProvider"`
	Logging          ucplog.LoggingOptions                    `yaml:"logging"`
	Identity         Identity                                 `yaml:"identity,omitempty"`
	UCP              config.UCPOptions                        `yaml:"ucp"`
	Location         string                                   `yaml:"location"`
}

const (
	// AuthUCPCredential is the authentication method via UCP Credential API.
	AuthUCPCredential = "UCPCredential"
	// AuthDefault is the default authentication method, such as environment variables.
	AuthDefault = "default"
)

// Identity represents configuration options for authenticating with external systems like Azure and AWS.
type Identity struct {
	// AuthMethod represents the method of authentication for authenticating with external systems like Azure and AWS.
	AuthMethod string `yaml:"authMethod"`
}
