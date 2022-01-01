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
	"time"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/cli/armtemplate"
	"github.com/project-radius/radius/pkg/cli/armtemplate/providers"
	"github.com/project-radius/radius/pkg/cli/clients"
)

// We use a shorted poll interval for local testing to make it faster.
// It wouldn't have an effect trying to use a shorter poll interval in Azure, because
// it's server-controlled.
const PollInterval = 5 * time.Second

type keyStruct struct {
}

var key = &keyStruct{}

type LocalRPDeploymentClient struct {
	SubscriptionID string
	ResourceGroup  string
	Providers      map[string]providers.Provider
}

var _ clients.DeploymentClient = (*LocalRPDeploymentClient)(nil)

func (dc *LocalRPDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	if options.ProgressChan != nil {
		defer close(options.ProgressChan)
		ctx = context.WithValue(ctx, key, options.ProgressChan)
	}

	template, err := armtemplate.Parse(options.Template)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	resources, outputsRaw, err := dc.deployTemplate(ctx, template, options.Parameters)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	outputs := map[string]clients.DeploymentOutput{}
	for name, output := range outputsRaw {
		outputs[name] = clients.DeploymentOutput{
			Type:  template.Outputs[name]["type"].(string),
			Value: output["value"],
		}
	}

	return clients.DeploymentResult{Resources: resources, Outputs: outputs}, err
}

func (dc *LocalRPDeploymentClient) DeployNested(ctx context.Context, id string, apiVersion string, body map[string]interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resource := armtemplate.DeploymentResource{}
	err = json.Unmarshal(b, &resource)
	if err != nil {
		return nil, err
	}

	_, outputsRaw, err := dc.deployTemplate(ctx, resource.Properties.Template, resource.Properties.Parameters)
	if err != nil {
		return nil, err
	}

	output := map[string]interface{}{}
	for k, v := range body {
		output[k] = v
	}

	// Need to use map[string]interface{} instead of more specific interface types
	outputs := map[string]interface{}{}
	for k, v := range outputsRaw {
		outputs[k] = v
	}

	output["properties"].(map[string]interface{})["outputs"] = outputs

	return output, nil
}

func (dc *LocalRPDeploymentClient) deployTemplate(ctx context.Context, template armtemplate.DeploymentTemplate, parameters map[string]map[string]interface{}) ([]azresources.ResourceID, map[string]map[string]interface{}, error) {
	obj := ctx.Value(key)

	var progressChan chan<- clients.ResourceProgress
	if obj != nil {
		progressChan = obj.(chan<- clients.ResourceProgress)
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{
		SubscriptionID:         dc.SubscriptionID,
		ResourceGroup:          dc.ResourceGroup,
		Parameters:             parameters,
		EvaluatePropertiesNode: false,
	})
	if err != nil {
		return nil, nil, err
	}

	deployed := map[string]map[string]interface{}{}
	evaluator := &armtemplate.DeploymentEvaluator{
		Providers: dc.Providers,
		Template:  template,
		Options: armtemplate.TemplateOptions{
			SubscriptionID:         dc.SubscriptionID,
			ResourceGroup:          dc.ResourceGroup,
			Parameters:             parameters,
			EvaluatePropertiesNode: true,
		},
		CustomActionCallback: func(id string, apiVersion string, action string, body interface{}) (interface{}, error) {
			return dc.customAction(ctx, id, apiVersion, action, body)
		},
		Deployed:  deployed,
		Variables: map[string]interface{}{},
		Outputs:   map[string]map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		if err != nil {
			return nil, nil, err
		}

		evaluator.Variables[name] = value
	}

	ids := []azresources.ResourceID{}
	for _, resource := range resources {
		body, err := evaluator.VisitResourceBody(resource)
		if err != nil {
			return nil, nil, err
		}

		resource.Body = body

		parsed, err := azresources.Parse(resource.ID)
		if err != nil {
			// We don't expect this to fail, but just in case...
			return nil, nil, err
		}

		if progressChan != nil {
			progressChan <- clients.ResourceProgress{
				Resource: parsed,
				Status:   clients.StatusStarted,
			}
		}

		result, err := dc.deployResource(ctx, resource)
		if err != nil {
			if progressChan != nil {
				progressChan <- clients.ResourceProgress{
					Resource: parsed,
					Status:   clients.StatusFailed,
				}
			}

			return nil, nil, fmt.Errorf("failed to PUT resource %s %s: %w", resource.Type, resource.Name, err)
		}

		evaluator.Deployed[resource.ID] = result

		if progressChan != nil {
			progressChan <- clients.ResourceProgress{
				Resource: parsed,
				Status:   clients.StatusCompleted,
			}
		}

		ids = append(ids, parsed)
	}

	for name, output := range template.Outputs {
		value, err := evaluator.VisitMap(output)
		if err != nil {
			return nil, nil, err
		}

		evaluator.Outputs[name] = value
	}

	return ids, evaluator.Outputs, nil
}

func (dc *LocalRPDeploymentClient) deployResource(ctx context.Context, resource armtemplate.Resource) (map[string]interface{}, error) {
	// TODO(tcnghia/rynowak): Right now we don't use symbolic references so we have to
	// hack this based on the ARM resource type. Long-term we will be able to use symbolic
	// references to find the provider by its ID.
	provider, err := armtemplate.GetProvider(dc.Providers, "", "", resource.Type)
	if err != nil {
		return nil, err
	}

	return provider.DeployResource(ctx, resource.ID, resource.APIVersion, resource.Body)
}

func (dc *LocalRPDeploymentClient) customAction(ctx context.Context, id string, apiVersion string, action string, body interface{}) (map[string]interface{}, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return nil, err
	}

	// TODO(tcnghia/rynowak): Right now we don't use symbolic references so we have to
	// hack this based on the ARM resource type. Long-term we will be able to use symbolic
	// references to find the provider by its ID.
	provider, err := armtemplate.GetProvider(dc.Providers, "", "", parsed.Type())
	if err != nil {
		return nil, err
	}

	return provider.InvokeCustomAction(ctx, id, apiVersion, action, body)
}
