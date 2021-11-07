// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// This package has TEMPORARY code that we use for fill the role of the ARM deployment engine
// in environments where it can't run right now (K8s, local testing). We don't intend to
// maintain this long-term and we don't intend to achieve parity.
package localrp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/azure/azresources"
	azclients "github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/cli/clients"
)

// We use a shorted poll interval for local testing to make it faster.
// It wouldn't have an effect trying to use a shorter poll interval in Azure, because
// it's server-controlled.
const PollInterval = 5 * time.Second

type LocalRPDeploymentClient struct {
	Providers      ProviderMap
	SubscriptionID string
	ResourceGroup  string
}

type ProviderMap = map[string]DeploymentProvider

type DeploymentProvider struct {
	Authorizer autorest.Authorizer
	BaseURL    string
	Connection *arm.Connection
}

var _ clients.DeploymentClient = (*LocalRPDeploymentClient)(nil)

func (dc *LocalRPDeploymentClient) GetExistingDeployment(ctx context.Context, options clients.DeploymentOptions) (*clients.DeploymentResult, error) {
	return nil, nil
}

func (dc *LocalRPDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	template, err := armtemplate.Parse(options.Template)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	resourceGroup, err := dc.GetResourceGroupData(ctx)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{
		SubscriptionID:         dc.SubscriptionID,
		ResourceGroup:          resourceGroup,
		Parameters:             options.Parameters,
		EvaluatePropertiesNode: false,
	})
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	deployed := map[string]map[string]interface{}{}
	evaluator := &armtemplate.DeploymentEvaluator{
		Template: template,
		Options: armtemplate.TemplateOptions{
			SubscriptionID:         dc.SubscriptionID,
			ResourceGroup:          resourceGroup,
			Parameters:             options.Parameters,
			EvaluatePropertiesNode: true,
		},
		CustomActionCallback: func(id string, apiVersion string, action string, body interface{}) (interface{}, error) {
			provider, err := dc.findProvider(id)
			if err != nil {
				return nil, err
			}

			return dc.customAction(ctx, provider, id, apiVersion, action, body)
		},
		Deployed:  deployed,
		Variables: map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		if err != nil {
			return clients.DeploymentResult{}, err
		}

		evaluator.Variables[name] = value
	}

	// NOTE: this is currently test-only code so we're fairly noisy about what we output here.
	ids := []azresources.ResourceID{}
	fmt.Printf("Starting deployment...\n")
	for _, resource := range resources {
		provider, err := dc.findProvider(resource.ID)
		if err != nil {
			return clients.DeploymentResult{}, err
		}

		body, err := evaluator.VisitMap(resource.Body)
		if err != nil {
			return clients.DeploymentResult{}, err
		}

		resource.Body = body

		fmt.Printf("Deploying %s %s...\n", resource.Type, resource.Name)
		response, result, err := dc.deployResource(ctx, provider, resource)
		if err != nil {
			return clients.DeploymentResult{}, fmt.Errorf("failed to PUT resource %s %s: %w", resource.Type, resource.Name, err)
		}

		fmt.Printf("succeed with status code %d\n", response.StatusCode)
		evaluator.Deployed[resource.ID] = result

		parsed, err := azresources.Parse(resource.ID)
		if err != nil {
			// We don't expect this to fail, but just in case...
			return clients.DeploymentResult{}, err
		}

		ids = append(ids, parsed)
	}

	if options.UpdateChannel != nil {
		close(options.UpdateChannel)
	}

	return clients.DeploymentResult{Resources: ids}, err
}

func (dc *LocalRPDeploymentClient) deployResource(ctx context.Context, provider DeploymentProvider, resource armtemplate.Resource) (*http.Response, map[string]interface{}, error) {
	client := azclients.NewGenericResourceClient(dc.SubscriptionID, provider.Authorizer)
	client.BaseURI = strings.TrimSuffix(provider.BaseURL, "/")
	client.PollingDelay = PollInterval

	converted := resources.GenericResource{}
	err := resource.Convert(&converted)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert resource %q: %w", resource.ID, err)
	}

	future, err := client.CreateOrUpdateByID(ctx, strings.TrimPrefix(resource.ID, "/"), resource.APIVersion, converted)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to PUT resource %q: %w", resource.ID, err)
	}

	err = future.WaitForCompletionRef(ctx, client.Client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to PUT resource %q: %w", resource.ID, err)
	}

	generic, err := future.Result(client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to PUT resource %q: %w", resource.ID, err)
	}

	b, err := json.Marshal(&generic)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response %q: %w", resource.ID, err)
	}

	result := map[string]interface{}{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal response %q: %w", resource.ID, err)
	}

	return future.Response(), result, nil
}

func (dc *LocalRPDeploymentClient) customAction(ctx context.Context, provider DeploymentProvider, id string, apiVersion string, action string, body interface{}) (map[string]interface{}, error) {
	client := azclients.NewCustomActionClient(dc.SubscriptionID, provider.Authorizer)
	client.BaseURI = provider.BaseURL

	response, err := client.InvokeCustomAction(ctx, id, apiVersion, action, body)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke custom action %q: %w", action, err)
	}

	return response.Body, nil
}

func (dc *LocalRPDeploymentClient) findProvider(id string) (DeploymentProvider, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return DeploymentProvider{}, err
	}

	if azresources.IsRadiusResource(parsed) || azresources.IsRadiusCustomAction(parsed) {
		provider, ok := dc.Providers["radius"]
		if !ok {
			return DeploymentProvider{}, fmt.Errorf("could not find %s provider", "radius")
		}

		return provider, nil
	} else if azresources.IsKubernetesResource(parsed) {
		provider, ok := dc.Providers["kubernetes"]
		if !ok {
			return DeploymentProvider{}, fmt.Errorf("could not find %s provider", "kubernetes")
		}

		return provider, nil
	} else {
		provider, ok := dc.Providers["azure"]
		if !ok {
			return DeploymentProvider{}, fmt.Errorf("could not find %s provider", "azure")
		}

		return provider, nil
	}
}

func (dc *LocalRPDeploymentClient) GetResourceGroupData(ctx context.Context) (armtemplate.ResourceGroup, error) {
	provider, ok := dc.Providers["azure"]
	if !ok {
		// no Azure provider, just provide the name for building resource ids
		return armtemplate.ResourceGroup{
			Name: dc.ResourceGroup,
		}, nil
	}

	rgc := azclients.NewGroupsClient(dc.SubscriptionID, provider.Authorizer)
	group, err := rgc.Get(ctx, dc.ResourceGroup)
	if err != nil {
		return armtemplate.ResourceGroup{}, fmt.Errorf("error finding resource group %q: %w", dc.ResourceGroup, err)
	}

	// TODO: for some reason this doesn't roundtrip through JSON well :-/

	return armtemplate.ResourceGroup{
		Name: dc.ResourceGroup,
		Properties: map[string]interface{}{
			"id":       *group.ID,
			"location": *group.Location,
			"name":     *group.Name,
		},
	}, nil
}
