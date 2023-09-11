/*
Copyright 2023 The Radius Authors.

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

package hostoptions

import (
	metricsprovider "github.com/radius-project/radius/pkg/metrics/provider"
	profilerprovider "github.com/radius-project/radius/pkg/profiler/provider"
	"github.com/radius-project/radius/pkg/trace"
	"github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	qprovider "github.com/radius-project/radius/pkg/ucp/queue/provider"
	"github.com/radius-project/radius/pkg/ucp/rest"
	"github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
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
