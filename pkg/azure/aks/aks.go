// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aks

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetAKSMonitoringCredentials(ctx context.Context, subscriptionID string, resourceGroup string, clusterName string) (*rest.Config, error) {
	armauth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return nil, err
	}

	// Currently we go to AKS every time to ask for credentials, we don't
	// cache them locally. This could be done in the future, but skipping it for now
	// since it's non-obvious that we'd store credentials in your ~/.rad directory
	mcc := clients.NewManagedClustersClient(subscriptionID, armauth)

	results, err := mcc.ListClusterMonitoringUserCredentials(ctx, resourceGroup, clusterName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list AKS cluster credentials: %w", err)
	}

	if results.Kubeconfigs == nil || len(*results.Kubeconfigs) == 0 {
		return nil, errors.New("failed to list AKS cluster credentials: response did not contain credentials")
	}

	kc := (*results.Kubeconfigs)[0]
	c, err := clientcmd.NewClientConfigFromBytes(*kc.Value)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig was invalid: %w", err)
	}

	restconfig, err := c.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig did not contain client credentials: %w", err)
	}

	return restconfig, nil
}
