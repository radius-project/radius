// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

import (
	"context"
	"fmt"

	cli_k8s "github.com/project-radius/radius/pkg/cli/kubernetes"
	k8s "github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/resources"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DoesComponentNameExist(ctx context.Context, appID string, resourceName string) (bool, error) {
	parsedAppID, err := resources.Parse(appID)
	if err != nil {
		return false, err
	}

	found := false
	componentName := k8s.MakeResourceName(parsedAppID.Name(), resourceName)

	// Prepare the label
	label, err := labels.Parse(fmt.Sprintf("%s=%s", k8s.LabelDaprComponentName, componentName))
	if err != nil {
		return found, err
	}

	// Get the K8S Client
	k8sClient, err := GetK8SClient()
	if err != nil {
		return found, err
	}

	// Check the cluster for the given label
	var services v1.ServiceList
	err = k8sClient.List(ctx, &services, &client.ListOptions{LabelSelector: label})
	if err != nil {
		return found, err
	}
	if len(services.Items) > 0 {
		found = true
	}

	return found, nil
}

func GetK8SClient() (client.Client, error) {
	k8sconfig, err := cli_k8s.ReadKubeConfig()
	if err != nil {
		return nil, err
	}

	client, err := cli_k8s.CreateRuntimeClient(k8sconfig.CurrentContext, cli_k8s.Scheme)
	if err != nil {
		return nil, err
	}

	return client, nil
}
