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

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/Azure/go-autorest/autorest"
	azclients "github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/cli/clients"
)

// We use a shorted poll interval for local testing to make it faster.
// It wouldn't have an effect trying to use a shorter poll interval in Azure, because
// it's server-controlled.
const PollInterval = 5 * time.Second

type LocalRPDeploymentClient struct {
	Authorizer     autorest.Authorizer
	BaseURL        string
	Connection     *armcore.Connection
	SubscriptionID string
	ResourceGroup  string
}

var _ clients.DeploymentClient = (*LocalRPDeploymentClient)(nil)

func (dc *LocalRPDeploymentClient) Deploy(ctx context.Context, content string, parameters clients.DeploymentParameters) error {
	template, err := armtemplate.Parse(content)
	if err != nil {
		return err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{
		SubscriptionID:         dc.SubscriptionID,
		ResourceGroup:          dc.ResourceGroup,
		Parameters:             parameters,
		EvaluatePropertiesNode: false,
	})
	if err != nil {
		return err
	}

	deployed := map[string]map[string]interface{}{}
	evaluator := &armtemplate.DeploymentEvaluator{
		Template: template,
		Options: armtemplate.TemplateOptions{
			SubscriptionID:         dc.SubscriptionID,
			ResourceGroup:          dc.ResourceGroup,
			Parameters:             parameters,
			EvaluatePropertiesNode: true,
		},
		CustomActionCallback: func(id, apiVersion string, action string, body interface{}) (interface{}, error) {
			return dc.customAction(ctx, id, apiVersion, action, body)
		},
		Deployed:  deployed,
		Variables: map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		if err != nil {
			return err
		}

		evaluator.Variables[name] = value
	}

	// NOTE: this is currently test-only code so we're fairly noisy about what we output here.
	fmt.Printf("Starting deployment...\n")
	for _, resource := range resources {
		body, err := evaluator.VisitMap(resource.Body)
		if err != nil {
			return err
		}

		resource.Body = body

		fmt.Printf("Deploying %s %s...\n", resource.Type, resource.Name)
		response, result, err := dc.deployResource(ctx, dc.Connection, resource)
		if err != nil {
			return fmt.Errorf("failed to PUT resource %s %s: %w", resource.Type, resource.Name, err)
		}

		fmt.Printf("succeed with status code %d\n", response.StatusCode)
		evaluator.Deployed[resource.ID] = result
	}

	return nil
}

func (dc *LocalRPDeploymentClient) deployResource(ctx context.Context, connection *armcore.Connection, resource armtemplate.Resource) (*http.Response, map[string]interface{}, error) {
	client := azclients.NewGenericResourceClient(dc.SubscriptionID, dc.Authorizer)
	client.BaseURI = strings.TrimSuffix(dc.BaseURL, "/")
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

func (dc *LocalRPDeploymentClient) customAction(ctx context.Context, id string, apiVersion string, action string, body interface{}) (map[string]interface{}, error) {
	client := azclients.NewCustomActionClient(dc.SubscriptionID, dc.Authorizer)
	client.BaseURI = dc.BaseURL

	response, err := client.InvokeCustomAction(ctx, id, apiVersion, action, body)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke custom action %q: %w", action, err)
	}

	return response.Body, nil
}
