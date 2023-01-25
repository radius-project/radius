// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"errors"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetConfig gets the Kubernetes config
func GetConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		// Fall-back to kubeconfig
	} else if err != nil {
		return nil, fmt.Errorf("failed to connect with in-cluster config: %w", err)
	} else {
		return config, nil
	}

	// Not in a cluster, fall back to local kubeconfig
	config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to connect with kubeconfig: %w", err)
	}

	return config, nil
}
