// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/google/uuid"
	azclients "github.com/project-radius/radius/pkg/azure/clients"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	NestedModuleType string = "Microsoft.Resources/deployments"

	// OperationPollInterval is the interval used for polling of deployment operations for progress.
	OperationPollInterval time.Duration = time.Second * 5
)

type ResourceDeploymentClient struct {
	RadiusResourceGroup string
	Client              azclients.ResourceDeploymentClient
	OperationsClient    azclients.ResourceDeploymentOperationsClient
	Tags                map[string]*string
	AzProvider          *workspaces.AzureProvider
	AWSProvider         *workspaces.AWSProvider
}

var _ clients.DeploymentClient = (*ResourceDeploymentClient)(nil)

func (dc *ResourceDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
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
			_ = dc.monitorProgress(ctx, name, options.ProgressChan, &wg)
			wg.Done()
		}()
	}

	summary, err := dc.waitForCompletion(ctx, *future)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return summary, nil
}

func (dc *ResourceDeploymentClient) startDeployment(ctx context.Context, name string, options clients.DeploymentOptions) (*resources.DeploymentsCreateOrUpdateFuture, error) {
	var resourceId string
	scopes := []ucpresources.ScopeSegment{
		{Type: "deployments", Name: "local"},
		{Type: "resourcegroups", Name: dc.RadiusResourceGroup},
	}
	types := []ucpresources.TypeSegment{
		{Type: "Microsoft.Resources/deployments", Name: name},
	}

	resourceId = ucpresources.MakeUCPID(scopes, types...)

	providerConfig := dc.GetProviderConfigs()

	future, err := dc.Client.CreateOrUpdate(ctx, resourceId, azclients.Deployment{
		Properties: &azclients.DeploymentProperties{
			Template:       options.Template,
			Parameters:     options.Parameters,
			ProviderConfig: providerConfig,
			Mode:           resources.DeploymentModeIncremental,
		},
		Tags: dc.Tags,
	})

	if err != nil {
		return nil, err
	}
	return &future, nil
}

func (dc *ResourceDeploymentClient) GetProviderConfigs() azclients.ProviderConfig {
	var providerConfigs azclients.ProviderConfig
	if dc.AzProvider != nil {
		if dc.AzProvider.SubscriptionID != "" && dc.AzProvider.ResourceGroup != "" {
			scope := "/subscriptions/" + dc.AzProvider.SubscriptionID + "/resourceGroups/" + dc.AzProvider.ResourceGroup
			providerConfigs.Az = &azclients.Az{
				Type: "AzureResourceManager",
				Value: azclients.Value{
					Scope: scope,
				},
			}
		}
	}

	if dc.AWSProvider != nil {
		scope := "/planes/aws/aws/accounts/" + dc.AWSProvider.AccountId + "/regions/" + dc.AWSProvider.Region
		providerConfigs.AWS = &azclients.AWS{
			Type: "AWS",
			Value: azclients.Value{
				Scope: scope,
			},
		}
	}

	if dc.RadiusResourceGroup != "" {
		scope := "/planes/radius/local/resourceGroups/" + dc.RadiusResourceGroup
		providerConfigs.Radius = &azclients.Radius{
			Type: "Radius",
			Value: azclients.Value{
				Scope: scope,
			},
		}

		scope = "/planes/deployments/local/resourceGroups/" + dc.RadiusResourceGroup
		providerConfigs.Deployments = &azclients.Deployments{
			Type: "Microsoft.Resources",
			Value: azclients.Value{
				Scope: scope,
			},
		}
	}

	return providerConfigs
}

func (dc *ResourceDeploymentClient) createSummary(deployment resources.DeploymentExtended) (clients.DeploymentResult, error) {
	if deployment.Properties == nil || deployment.Properties.OutputResources == nil {
		return clients.DeploymentResult{}, nil
	}

	resources := []ucpresources.ID{}
	for _, resource := range *deployment.Properties.OutputResources {
		if resource.ID == nil {
			continue
		}

		// We might see scopes here as well as resources, so using the general Parse function.
		id, err := ucpresources.Parse(*resource.ID)
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

func (dc *ResourceDeploymentClient) waitForCompletion(ctx context.Context, future resources.DeploymentsCreateOrUpdateFuture) (clients.DeploymentResult, error) {
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

func (dc *ResourceDeploymentClient) monitorProgress(ctx context.Context, name string, progressChan chan<- clients.ResourceProgress, wg *sync.WaitGroup) error {
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

			provisioningState := v1.ProvisioningState(*operation.Properties.ProvisioningState)

			// We might see scopes here as well as resources, so using the general Parse function.
			id, err := ucpresources.Parse(*operation.Properties.TargetResource.ID)
			if err != nil {
				return err
			}

			if strings.EqualFold(id.Type(), NestedModuleType) {
				// Recursively monitor progress for nested deployments in a new goroutine
				wg.Add(1)
				go func() {
					// Bicep modules are themselves a resource, and so they only will show up after the deployment starts.
					// When that happens we need to monitor them recursively so we can display the resources inside of them.
					_ = dc.monitorProgress(ctx, id.Name(), progressChan, wg)
					wg.Done()
				}()
			}

			current := status[id.String()]

			next := clients.StatusStarted
			if v1.ProvisioningStateSucceeded == provisioningState {
				next = clients.StatusCompleted
			} else if provisioningState.IsTerminal() {
				next = clients.StatusFailed
			}

			if current != next && progressChan != nil {
				status[id.String()] = next
				progressChan <- clients.ResourceProgress{
					Resource: id,
					Status:   next,
				}
			}
		}
	}

	return nil
}

func (dc *ResourceDeploymentClient) listOperations(ctx context.Context, name string) ([]resources.DeploymentOperation, error) {
	var resourceId string

	// No providers section, hence all segments are part of scopes
	scopes := []ucpresources.ScopeSegment{
		{Type: "deployments", Name: "local"},
		{Type: "resourcegroups", Name: dc.RadiusResourceGroup},
	}
	types := ucpresources.TypeSegment{
		Type: "Microsoft.Resources/deployments",
		Name: name,
	}
	resourceId = ucpresources.MakeUCPID(scopes, types)

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
