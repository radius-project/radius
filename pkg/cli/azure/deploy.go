// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/google/uuid"
)

const OperationPollInterval time.Duration = time.Second * 5

type ARMDeploymentClient struct {
	ResourceGroup     string
	SubscriptionID    string
	DeploymentsClient resources.DeploymentsClient
	OperationsClient  resources.DeploymentOperationsClient
}

var _ clients.DeploymentClient = (*ARMDeploymentClient)(nil)

func (dc *ARMDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	template := map[string]interface{}{}
	err := json.Unmarshal([]byte(options.Template), &template)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	name := fmt.Sprintf("rad-deploy-%v", uuid.New().String())
	op, err := dc.DeploymentsClient.CreateOrUpdate(ctx, dc.ResourceGroup, name, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template:   template,
			Parameters: options.Parameters,
			Mode:       resources.DeploymentModeIncremental,
		},
	})
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	if options.UpdateChannel != nil {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go dc.monitorDeployment(ctx, name, options.UpdateChannel)
	}

	err = op.WaitForCompletionRef(ctx, dc.DeploymentsClient.Client)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	deployment, err := op.Result(dc.DeploymentsClient)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	summary, err := dc.createSummary(deployment)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return summary, nil
}

func (dc *ARMDeploymentClient) monitorDeployment(ctx context.Context, name string, updateChannel chan<- clients.DeploymentProgressUpdate) error {
	// Now that the deployment has started we can fetch the set of operations and monitor them...
	//
	// Track status so we only broadcast the deltas
	status := map[string]string{}

	// We're the only writer of updates
	defer close(updateChannel)

	// Now loop forever for updates. We're relying on cancellation of the context to terminate.
	for ctx.Err() == nil {
		time.Sleep(OperationPollInterval)

		operations, err := dc.listOperations(ctx, name)
		if err != nil && err == ctx.Err() {
			return nil
		} else if err != nil {
			return err
		}

		for _, operation := range operations {
			if operation.Properties == nil || operation.Properties.TargetResource == nil || operation.Properties.TargetResource.ID == nil {
				continue
			}

			provisioningState := rest.OperationStatus(*operation.Properties.ProvisioningState)
			id, err := azresources.Parse(*operation.Properties.TargetResource.ID)
			if err != nil {
				return err
			}

			current := status[id.ID]
			next := clients.UpdateStart
			if rest.SuccededStatus == provisioningState {
				next = clients.UpdateSucceeded
			} else if rest.IsTeminalStatus(provisioningState) {
				next = clients.UpdateFailed
			}

			if current != next {
				status[id.ID] = next
				updateChannel <- clients.DeploymentProgressUpdate{
					Resource: id,
					Kind:     next,
				}
			}
		}
	}

	return nil
}

func (dc *ARMDeploymentClient) listOperations(ctx context.Context, name string) ([]resources.DeploymentOperation, error) {
	operationList, err := dc.OperationsClient.List(ctx, dc.ResourceGroup, name, nil)
	if err != nil {
		return nil, err
	}

	operations := []resources.DeploymentOperation{}
	for ; operationList.NotDone(); operationList.NextWithContext(ctx) {
		if err != nil {
			return nil, err
		}

		operations = append(operations, operationList.Values()...)
	}

	return operations, nil
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

	return clients.DeploymentResult{Resources: resources}, nil
}
