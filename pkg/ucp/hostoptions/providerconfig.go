// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	"github.com/project-radius/radius/pkg/sdk"
	metricsprovider "github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// UCPConfig includes the resource provider configuration.
type UCPConfig struct {
	StorageProvider dataprovider.StorageProviderOptions    `yaml:"storageProvider"`
	Planes          []rest.Plane                           `yaml:"planes"`
	SecretProvider  provider.SecretProviderOptions         `yaml:"secretProvider"`
	MetricsProvider metricsprovider.MetricsProviderOptions `yaml:"metricsProvider"`
	Logging         ucplog.LoggingOptions                  `yaml:"logging"`
	Identity        Identity                               `yaml:"identity,omitempty"`
	UCP             sdk.UCPOptions                         `yaml:"ucp"`
}

const (
	// AuthUCPCredential is the authentication method via UCP Credential API.
	AuthUCPCredential = "UCPCredential"
	// AuthDefault is the default authentication method, such as environment variables.
	AuthDefault = "default"
)

// Identity includes the identity configuration.
type Identity struct {
	// Auth represents the type of authentication.
	Auth string `yaml:"authentication"`
}
