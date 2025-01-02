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

package dynamicrp

import (
	"bytes"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	ucpconfig "github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"gopkg.in/yaml.v3"
)

// Config defines the configuration for the DynamicRP server.
//
// For testability, all fields on this struct MUST be parsable from YAML without any further initialization required.
type Config struct {
	// Bicep configures properties for the Bicep recipe driver.
	Bicep hostoptions.BicepOptions `yaml:"bicep"`

	// Database is the configuration for the database.
	Database databaseprovider.Options `yaml:"databaseProvider"`

	// Environment is the configuration for the hosting environment.
	Environment hostoptions.EnvironmentOptions `yaml:"environment"`

	// Kubernetes is the configuration for the Kubernetes client.
	Kubernetes kubernetesclientprovider.Options `yaml:"kubernetes"`

	// Logging is the configuration for the logging system.
	Logging ucplog.LoggingOptions `yaml:"logging"`

	// Metrics is the configuration for the metrics endpoint.
	Metrics metricsservice.Options `yaml:"metricsProvider"`

	// Profiler is the configuration for the profiler endpoint.
	Profiler profilerservice.Options `yaml:"profilerProvider"`

	// Queue is the configuration for the message queue.
	Queue queueprovider.QueueProviderOptions `yaml:"queueProvider"`

	// Secrets is the configuration for the secret storage system.
	Secrets secretprovider.SecretProviderOptions `yaml:"secretProvider"`

	// Server is the configuration for the HTTP server.
	Server hostoptions.ServerOptions `yaml:"server"`

	// Terraform configures properties for the Terraform recipe driver.
	Terraform hostoptions.TerraformOptions `yaml:"terraform"`

	// Tracing is the configuration for the tracing system.
	Tracing traceservice.Options `yaml:"tracerProvider"`

	// UCPConfig is the configuration for the connection to UCP.
	UCP ucpconfig.UCPOptions `yaml:"ucp"`

	// Worker is the configuration for the backend worker server.
	Worker hostoptions.WorkerServerOptions `yaml:"workerServer"`
}

// LoadConfig loads a Config from bytes.
func LoadConfig(bs []byte) (*Config, error) {
	decoder := yaml.NewDecoder(bytes.NewBuffer(bs))
	decoder.KnownFields(true)

	config := Config{}
	err := decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
