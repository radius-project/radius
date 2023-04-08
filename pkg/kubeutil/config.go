// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubeutil

import (
	"errors"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	// ServerBurst is the default number of queries per second for server.
	ServerQPS float32 = 200.0
	// ServerBurst is the default number of queries k8sclient handles concurrently for server.
	ServerBurst int = 200

	// CliQPS is the default number of queries per second for CLI.
	CliQPS float32 = 50.0
	// CliBurst is the default number of queries k8sclient handles concurrently for CLI.
	CliBurst int = 100
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

// LoadDefaultConfig returns kube config from home directory.
func LoadDefaultConfig() (*api.Config, error) {
	// empty config file path falls back to the default .kube/config in home directory.
	return LoadKubeConfig("")
}

// LoadKubeConfig loads kubenetes config from specified config file.
// If configFilePath is empty, it will use the default config from home directory.
func LoadKubeConfig(configFilePath string) (*api.Config, error) {
	if configFilePath == "" {
		configFilePath = clientcmd.RecommendedHomeFile
	}

	return clientcmd.LoadFromFile(configFilePath)
}

// NewClusterConfig loads kubeconfig in cluster or from the file.
func NewClusterConfig(options *ConfigOptions) (*rest.Config, error) {
	options = buildConfigOptions(options)

	var config *rest.Config
	var err error

	config, err = rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		config, err = NewClusterConfigFromLocal(options)
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

// NewClusterConfigFromLocal loads config from local home directory.
func NewClusterConfigFromLocal(options *ConfigOptions) (*rest.Config, error) {
	options = buildConfigOptions(options)

	cfg, err := LoadKubeConfig(options.ConfigFilePath)
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
