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

package kubeutil

import (
	"errors"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	// DefaultServerBurst is the default number of queries per second for server.
	DefaultServerQPS float32 = 200.0
	// DefaultServerBurst is the default number of queries k8sclient handles concurrently for server.
	DefaultServerBurst int = 200

	// DefaultCLIQPS is the default number of queries per second for CLI.
	DefaultCLIQPS float32 = 50.0
	// DefaultCLIBurst is the default number of queries k8sclient handles concurrently for CLI.
	DefaultCLIBurst int = 100
)

// ConfigOptions is custom options to configure kubernetes client config.
type ConfigOptions struct {
	ConfigFilePath string
	QPS            float32
	Burst          int
	ContextName    string
}

func buildConfigOptions(options *ConfigOptions) *ConfigOptions {
	if options == nil {
		options = &ConfigOptions{}
	}

	if options.ConfigFilePath == "" {
		options.ConfigFilePath = clientcmd.RecommendedHomeFile
	}

	return options
}

// LoadConfigFile loads the Kubernetes config file from the given path, or will load the default
// config from the home directory if no path is provided.
func LoadConfigFile(configFilePath string) (*api.Config, error) {
	if configFilePath == "" {
		configFilePath = clientcmd.RecommendedHomeFile
	}

	return clientcmd.LoadFromFile(configFilePath)
}

// NewClientConfig builds a Kubernetes client config from either in-cluster or local configuration, and sets the QPS and
// Burst values if provided. It returns an error if the config cannot be built.
func NewClientConfig(options *ConfigOptions) (*rest.Config, error) {
	options = buildConfigOptions(options)

	var config *rest.Config
	var err error

	config, err = rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		config, err = NewClientConfigFromLocal(options)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
	}

	if options.QPS > 0.0 {
		config.QPS = options.QPS
	}

	if options.Burst > 0 {
		config.Burst = options.Burst
	}

	return config, nil
}

// NewClientConfigFromLocal builds a Kubernetes client config from a ConfigOptions instance, loading the config file from the
// ConfigFilePath and setting the QPS and Burst values if specified. It returns an error if the config file cannot be loaded
// or the client config cannot be initialized.
func NewClientConfigFromLocal(options *ConfigOptions) (*rest.Config, error) {
	options = buildConfigOptions(options)

	cfg, err := LoadConfigFile(options.ConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
	}

	contextName := options.ContextName
	if contextName == "" {
		contextName = cfg.CurrentContext
	}

	merged, err := clientcmd.NewNonInteractiveClientConfig(*cfg, contextName, nil, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
	}

	if options.QPS > 0.0 {
		merged.QPS = options.QPS
	}

	if options.Burst > 0 {
		merged.Burst = options.Burst
	}

	return merged, nil
}
