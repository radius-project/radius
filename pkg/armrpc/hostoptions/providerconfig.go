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
	sprovider "github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
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
	Bicep            BicepOptions                             `yaml:"bicep,omitempty"`
	Terraform        TerraformOptions                         `yaml:"terraform,omitempty"`

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

// BicepOptions includes options required for bicep execution.
type BicepOptions struct {
	// DeleteRetryCount is the number of times to retry the request.
	DeleteRetryCount string `yaml:"deleteRetryCount,omitempty"`
	// DeleteRetryDelaySeconds is the delay between retries in seconds.
	DeleteRetryDelaySeconds string `yaml:"deleteRetryDelaySeconds,omitempty"`
}

// TerraformOptions includes options required for terraform execution.
type TerraformOptions struct {
	// Path is the path to the directory mounted to the container where terraform can be installed and executed.
	Path string `yaml:"path,omitempty"`
}
