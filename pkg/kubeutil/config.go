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
	DefaultQPS   float32 = 200.0
	DefaultBurst int     = 200
)

// NewClusterConfig gets the Kubernetes config
func NewClusterConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		// Not in a cluster, fall back to local kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to connect with kubeconfig: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to connect with in-cluster config: %w", err)
	}

	config.QPS = DefaultQPS
	config.Burst = DefaultBurst

	return config, nil
}

// NewClusterConfigWithContext creates cluster config with context.
func NewClusterConfigWithContext(onlyInCluster bool, contextName string) (*rest.Config, error) {
	var err error
	var config *rest.Config

	if onlyInCluster {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
		}
	} else {
		cfg, err := clientcmd.LoadFromFile(clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
		}

		config, err = clientcmd.NewNonInteractiveClientConfig(*cfg, contextName, nil, nil).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
		}
	}

	config.QPS = DefaultQPS
	config.Burst = DefaultBurst

	return config, nil
}
