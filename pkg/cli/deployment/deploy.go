/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	sdkclients "github.com/project-radius/radius/pkg/sdk/clients"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	NestedModuleType string = "Microsoft.Resources/deployments"

	// OperationPollInterval is the interval used for polling of deployment operations for progress.
	OperationPollInterval time.Duration = time.Second * 5

	// deploymentPollInterval is the polling frequency of the deployment client.
	// This is set to a relatively low number because we're using the UCP deployment engine
	// inside the cluster. This is a good balance to feel responsible for quick operations
	// like deploying Kubernetes resources without generating a wasteful amount of traffic.
	// The default would be 30 seconds.
	deploymentPollInterval = time.Second * 5
)

type ResourceDeploymentClient struct {
	RadiusResourceGroup string
	Client              *sdkclients.ResourceDeploymentsClient
	OperationsClient    *sdkclients.ResourceDeploymentOperationsClient
	Tags                map[string]*string
}

var _ clients.DeploymentClient = (*ResourceDeploymentClient)(nil)

// # Function Explanation
//
// Deploy starts a deployment, monitors its progress, and returns the deployment summary when it is complete, or an error if one occurs.
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
	poller, err := dc.startDeployment(ctx, name, options)
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

	summary, err := dc.waitForCompletion(ctx, poller)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return summary, nil
}

func (dc *ResourceDeploymentClient) startDeployment(ctx context.Context, name string, options clients.DeploymentOptions) (*runtime.Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	var resourceId string
	scopes := []ucpresources.ScopeSegment{
		{
			Type: "radius",
			Name: "local",
		},
		{
			Type: "resourcegroups",
			Name: dc.RadiusResourceGroup,
		},
	}
	types := []ucpresources.TypeSegment{
		{
			Type: "Microsoft.Resources/deployments",
			Name: name,
		},
	}

	resourceId = ucpresources.MakeUCPID(scopes, types...)
	providerConfig := dc.GetProviderConfigs(options)

	poller, err := dc.Client.CreateOrUpdate(ctx,
		sdkclients.Deployment{
			Properties: &sdkclients.DeploymentProperties{
				Template:       options.Template,
				Parameters:     options.Parameters,
				ProviderConfig: providerConfig,
				Mode:           armresources.DeploymentModeIncremental,
			},
		},
		resourceId,
		sdkclients.DeploymentsClientAPIVersion)
	if err != nil {
		return nil, err
	}

	return poller, nil
}

// # Function Explanation
//
// GetProviderConfigs() creates a default provider config and then updates it with any provider scopes passed in the DeploymentOptions.
func (dc *ResourceDeploymentClient) GetProviderConfigs(options clients.DeploymentOptions) sdkclients.ProviderConfig {
	providerConfig := sdkclients.NewDefaultProviderConfig(dc.RadiusResourceGroup)
	// if there are no providers, then return default provider config
	if options.Providers == nil {
		return providerConfig
	}

	if options.Providers.Azure != nil && options.Providers.Azure.Scope != "" {
		providerConfig.Az = &sdkclients.Az{
			Type: sdkclients.ProviderTypeAzure,
			Value: sdkclients.Value{
				Scope: options.Providers.Azure.Scope,
			},
		}
	}

	if options.Providers.AWS != nil && options.Providers.AWS.Scope != "" {
		providerConfig.AWS = &sdkclients.AWS{
			Type: sdkclients.ProviderTypeAWS,
			Value: sdkclients.Value{
				Scope: options.Providers.AWS.Scope,
			},
		}
	}

	return providerConfig
}

func (dc *ResourceDeploymentClient) createSummary(deployment *armresources.DeploymentExtended) (clients.DeploymentResult, error) {
	if deployment.Properties == nil || deployment.Properties.OutputResources == nil {
		return clients.DeploymentResult{}, nil
	}

	resources := []ucpresources.ID{}
	for _, resource := range deployment.Properties.OutputResources {
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

func (dc *ResourceDeploymentClient) waitForCompletion(ctx context.Context, poller *runtime.Poller[sdkclients.ClientCreateOrUpdateResponse]) (clients.DeploymentResult, error) {
	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: deploymentPollInterval})
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	summary, err := dc.createSummary(&resp.DeploymentExtended)
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

func (dc *ResourceDeploymentClient) listOperations(ctx context.Context, name string) ([]*armresources.DeploymentOperation, error) {
	var resourceId string

	// No providers section, hence all segments are part of scopes
	scopes := []ucpresources.ScopeSegment{
		{Type: "radius", Name: "local"},
		{Type: "resourcegroups", Name: dc.RadiusResourceGroup},
	}
	types := ucpresources.TypeSegment{
		Type: "Microsoft.Resources/deployments",
		Name: name,
	}

	resourceId = ucpresources.MakeUCPID(scopes, types)

	ops, err := dc.OperationsClient.List(ctx, dc.RadiusResourceGroup, name, resourceId, sdkclients.DeploymentOperationsClientAPIVersion, nil)
	if err != nil {
		return nil, err
	}

	return ops.Value, nil
}
