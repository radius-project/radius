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

package ucp

import (
	"bytes"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	ucpconfig "github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"gopkg.in/yaml.v3"
)

// Config defines the configuration for the UCP server.
//
// For testability, all fields on this struct MUST be parsable from YAML without any further initialization required.
type Config struct {
	// Database is the configuration for the database used for resource data.
	Database databaseprovider.Options `yaml:"databaseProvider"`

	// Environment is the configuration for the hosting environment.
	Environment hostoptions.EnvironmentOptions `yaml:"environment"`

	// Identity is the configuration for authenticating with external systems like Azure and AWS.
	Identity IdentityConfig `yaml:"identity"`

	// Initialization is the configuration for initializing the UCP server.
	Initialization InitializationConfig `yaml:"initialization"`

	// Logging is the configuration for the logging system.
	Logging ucplog.LoggingOptions `yaml:"logging"`

	// Metrics is the configuration for the metrics endpoint.
	Metrics metricsservice.Options `yaml:"metricsProvider"`

	// Profiler is the configuration for the profiler endpoint.
	Profiler profilerservice.Options `yaml:"profilerProvider"`

	// Routing is the configuration for UCP routing.
	Routing RoutingConfig `yaml:"routing"`

	// Queue is the configuration for the message queue.
	Queue queueprovider.QueueProviderOptions `yaml:"queueProvider"`

	// Secrets is the configuration for the secret storage system.
	Secrets secretprovider.SecretProviderOptions `yaml:"secretProvider"`

	// Server is the configuration for the HTTP server.
	Server hostoptions.ServerOptions `yaml:"server"`

	// Tracing is the configuration for the tracing system.
	Tracing traceservice.Options `yaml:"tracerProvider"`

	// UCPConfig is the configuration for the connection to UCP.
	UCP ucpconfig.UCPOptions `yaml:"ucp"`

	// Worker is the configuration for the backend worker server.
	Worker hostoptions.WorkerServerOptions `yaml:"workerServer"`
}

const (
	// AuthUCPCredential is the authentication method via UCP Credential API.
	AuthUCPCredential = "UCPCredential"

	// AuthDefault is the default authentication method, such as environment variables.
	AuthDefault = "default"
)

// Identity represents configuration options for authenticating with external systems like Azure and AWS.
type IdentityConfig struct {
	// AuthMethod represents the method of authentication for authenticating with external systems like Azure and AWS.
	AuthMethod string `yaml:"authMethod"`
}

// RoutingConfig provides configuration for UCP routing.
type RoutingConfig struct {
	// DefaultDownstreamEndpoint is the default destination when a resource provider does not provide a downstream endpoint.
	// In practice, this points to the URL of dynamic-rp.
	DefaultDownstreamEndpoint string `yaml:"defaultDownstreamEndpoint"`
}

// InitializeConfig defines the configuration for initializing the UCP server.
//
// This includes resources that are added to UCP's data on startup.
//
// TODO: this will be generalized as part of the UDT work. Right now it only
// handles planes, and we need to support other kinds of resources.
type InitializationConfig struct {
	// Planes is a list of planes to create at startup.
	Planes []Plane `yaml:"planes,omitempty"`
}

// Plane is a configuration entry for a plane resource. This is used to create a plane resource at startup.
type Plane struct {
	// ID is the resource ID of the plane.
	ID string `json:"id" yaml:"id"`

	// Type is the resource type of the plane.
	Type string `json:"type" yaml:"type"`

	// Name is the resource name of the plane.
	Name string `json:"name" yaml:"name"`

	// Properties is the properties of the plane.
	Properties PlaneProperties `json:"properties" yaml:"properties"`
}

type PlaneProperties struct {
	// ResourceProviders is a map of resource provider namespaces to their respective addresses.
	//
	// This is part of legacy (non-UDT) support for planes and will be removed.
	ResourceProviders map[string]string `json:"resourceProviders" yaml:"resourceProviders"`

	// Kind is the legacy UCP plane type.
	Kind string `json:"kind" yaml:"kind"`

	// URL is the downsteam URL for the plane.
	URL string `json:"url" yaml:"url"`
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
