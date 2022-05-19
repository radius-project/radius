// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/azure/azresources"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/radrp/rest"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

// OperationPollInterval is the interval used for polling of deployment operations for progress.
const OperationPollInterval time.Duration = time.Second * 5

type ResouceDeploymentClient struct {
	SubscriptionID   string
	ResourceGroup    string
	Client           azclients.ResourceDeploymentClient
	OperationsClient azclients.ResourceDeploymentOperationsClient
	Tags             map[string]*string
	EnableUCP        bool
}

var _ clients.DeploymentClient = (*ResouceDeploymentClient)(nil)

func (dc *ResouceDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	// Used for graceful shutdown of the polling listener.
	wg := sync.WaitGroup{}
	defer func() {
		wg.Wait()
		if options.ProgressChan != nil {
			close(options.ProgressChan)
		}
	}()

	name := fmt.Sprintf("rad-deploy-%v", uuid.New().String())
	future, err := dc.startDeployment(ctx, name, options)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	if options.ProgressChan != nil {
		// To monitor the progress we have to do polling. We cancel that once
		// the operation completes.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		wg.Add(1)
		go func() {
			_ = dc.monitorProgress(ctx, name, options.ProgressChan)
			wg.Done()
		}()
	}

	summary, err := dc.waitForCompletion(ctx, *future)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return summary, nil
}

func (dc *ResouceDeploymentClient) startDeployment(ctx context.Context, name string, options clients.DeploymentOptions) (*resources.DeploymentsCreateOrUpdateFuture, error) {
	template := map[string]interface{}{}
	err := json.Unmarshal([]byte(options.Template), &template)
	if err != nil {
		return nil, err
	}

	var resourceId string
	if dc.EnableUCP {
		scopes := []ucpresources.ScopeSegment{
			{Type: "planes", Name: "deployments/local"},
			{Type: "resourcegroups", Name: dc.ResourceGroup},
		}
		types := []ucpresources.TypeSegment{
			{Type: "Microsoft.Resources/deployments", Name: name},
		}

		resourceId = ucpresources.MakeRelativeID(scopes, types...)
	} else {
		scopes := []ucpresources.ScopeSegment{
			{Type: "subscriptions", Name: dc.SubscriptionID},
			{Type: "resourcegroups", Name: dc.ResourceGroup},
		}
		types := []ucpresources.TypeSegment{
			{Type: "Microsoft.Resources/deployments", Name: name},
		}
		resourceId = ucpresources.MakeRelativeID(scopes, types...)
	}

	// /apis/api.ucp.dev/v1alpha3/planes/deployments/local/resourceGroups/justin-azure-rg/providers/Microsoft.Resources/deployments/rad-deploy-d6b4f46b-bf81-4b1d-a6df-c77432d1c334
	future, err := dc.Client.CreateOrUpdate(ctx, resourceId, resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template:   template,
			Parameters: options.Parameters,
			Mode:       resources.DeploymentModeIncremental,
		},
		Tags: dc.Tags,
	})

	if err != nil {
		return nil, err
	}

	return &future, nil
}

func (dc *ResouceDeploymentClient) createSummary(deployment resources.DeploymentExtended) (clients.DeploymentResult, error) {
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
			return clients.DeploymentResult{}, err
		}

		resources = append(resources, id)
	}

	outputs := map[string]clients.DeploymentOutput{}
	b, err := json.Marshal(&deployment.Properties.Outputs)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	err = json.Unmarshal(b, &outputs)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return clients.DeploymentResult{Resources: resources, Outputs: outputs}, nil
}

func (dc *ResouceDeploymentClient) waitForCompletion(ctx context.Context, future resources.DeploymentsCreateOrUpdateFuture) (clients.DeploymentResult, error) {
	var err error
	var deployment resources.DeploymentExtended

	err = future.WaitForCompletionRef(ctx, dc.Client.Client)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	deployment, err = future.Result(dc.Client.DeploymentsClient)

	if err != nil {
		return clients.DeploymentResult{}, err
	}

	summary, err := dc.createSummary(deployment)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return summary, nil
}

func (dc *ResouceDeploymentClient) monitorProgress(ctx context.Context, name string, progressChan chan<- clients.ResourceProgress) error {
	// A note about this: since we're doing polling we might not see all of the operations
	// complete before the overall deployment completes. That's fine, this will be handled
	// by the presentation layer. In this code we just cancel when we're told to.
	//
	// However we do need to 'drain' on cancellation. That is, we wait will communicate
	// back to the caller when we're fully-shut-down. This prevents writing to a closed channel.

	// Also nothing listens to errors if we report them here. It's just a convenient way to degrade gracefully
	// in the event of an issue.

	// We need to track the status so we can report the deltas
	status := map[string]clients.ResourceStatus{}

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
			next := clients.StatusStarted
			if rest.SuccededStatus == provisioningState {
				next = clients.StatusCompleted
			} else if rest.IsTeminalStatus(provisioningState) {
				next = clients.StatusFailed
			}

			if current != next && progressChan != nil {
				status[id.ID] = next
				progressChan <- clients.ResourceProgress{
					Resource: id,
					Status:   next,
				}
			}
		}
	}

	return nil
}

func (dc *ResouceDeploymentClient) listOperations(ctx context.Context, name string) ([]resources.DeploymentOperation, error) {
	var resourceId string

	// No providers section, hence all segments are part of scopes
	if dc.EnableUCP {
		scopes := []ucpresources.ScopeSegment{
			{Type: "planes", Name: "deployments"},
			{Type: "local", Name: ""},
			{Type: "resourcegroups", Name: dc.ResourceGroup},
			{Type: "deployments", Name: name},
			{Type: "operations"},
		}
		resourceId = ucpresources.MakeRelativeID(scopes)
	} else {
		scopes := []ucpresources.ScopeSegment{
			{Type: "subscriptions", Name: dc.SubscriptionID},
			{Type: "resourcegroups", Name: dc.ResourceGroup},
			{Type: "deployments", Name: name},
			{Type: "operations"},
		}
		resourceId = ucpresources.MakeRelativeID(scopes)
	}
	//"/subscriptions/default/resourcegroups/default/providers/deployments/rad-deploy-6de8cdf7-e64e-4aa2-8e2d-a2baa8f74275/operations"

	operationList, err := dc.OperationsClient.List(ctx, resourceId, nil)
	if err != nil {
		return nil, err
	}

	operations := []resources.DeploymentOperation{}
	for ; operationList.NotDone(); err = operationList.NextWithContext(ctx) {
		if err != nil {
			return nil, err
		}

		operations = append(operations, operationList.Values()...)
	}

	return operations, nil
}
