// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/google/uuid"
)

type ARMDeploymentClient struct {
	ResourceGroup  string
	SubscriptionID string
	Client         resources.DeploymentsClient
}

var _ clients.DeploymentClient = (*ARMDeploymentClient)(nil)

func (dc *ARMDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	template := map[string]interface{}{}
	err := json.Unmarshal([]byte(options.Template), &template)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	name := fmt.Sprintf("rad-deploy-%v", uuid.New().String())
	op, err := dc.Client.CreateOrUpdate(ctx, dc.ResourceGroup, name, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template:   template,
			Parameters: options.Parameters,
			Mode:       resources.DeploymentModeIncremental,
		},
	})
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	err = op.WaitForCompletionRef(ctx, dc.Client.Client)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	deployment, err := op.Result(dc.Client)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	summary, err := dc.createSummary(deployment)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return summary, nil
}

func (dc *ARMDeploymentClient) createSummary(deployment resources.DeploymentExtended) (clients.DeploymentResult, error) {
	if deployment.Properties == nil || deployment.Properties.OutputResources == nil {
		return clients.DeploymentResult{}, nil
	}

	resources := []azresources.ResourceID{}
	for _, resource := range *deployment.Properties.OutputResources {
		if resource.ID == nil {
			continue
		}

		id, err := azresources.Parse(*resource.ID)
		if err != nil {
			return clients.DeploymentResult{}, nil
		}

		resources = append(resources, id)
	}

	outputs := map[string]clients.DeploymentOutput{}
	b, err := json.Marshal(&deployment.Properties.Outputs)
	if err != nil {
		return clients.DeploymentResult{}, nil
	}

	err = json.Unmarshal(b, &outputs)
	if err != nil {
		return clients.DeploymentResult{}, nil
	}

	return clients.DeploymentResult{Resources: resources, Outputs: outputs}, nil
}
