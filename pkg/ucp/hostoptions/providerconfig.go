// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
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
}
