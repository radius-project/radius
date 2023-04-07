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
)

const (
	// DefaultQPS is the default number of queries per second.
	DefaultQPS float32 = 200.0
	// DefaultBurst is the default number of queries client handles concurrently.
	DefaultBurst int = 200
)

// NewClusterConfig loads kubeconfig in cluster or from the file specified by configFilePath.
// If configFilePath is empty string, we will use kube config from home directory.
func NewClusterConfig(configFilePath string) (*rest.Config, error) {
	if configFilePath == "" {
		configFilePath = clientcmd.RecommendedHomeFile
	}

	config, err := rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		// Not in a cluster, fall back to local kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", configFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
	}

	config.QPS = DefaultQPS
	config.Burst = DefaultBurst

	return config, nil
}

// NewClusterConfigWithContext loads kubeconfig in cluster or from the file specified by configFilePath.
// If configFilePath is empty string, we will use kube config from home directory.
func NewClusterConfigWithContext(configFilePath, contextName string, onlyInCluster bool) (*rest.Config, error) {
	if configFilePath == "" {
		configFilePath = clientcmd.RecommendedHomeFile
	}

	var err error
	var config *rest.Config

	if onlyInCluster {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
		}
	} else {
		cfg, err := clientcmd.LoadFromFile(configFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
		}

		config, err = clientcmd.NewNonInteractiveClientConfig(*cfg, contextName, nil, nil).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
		}
	}

	config.QPS = DefaultQPS
	config.Burst = DefaultBurst

	return config, nil
}
