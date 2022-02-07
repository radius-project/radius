// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
)

// We use a shorted poll interval for local testing to make it faster.
// It wouldn't have an effect trying to use a shorter poll interval in Azure, because
// it's server-controlled.
const PollInterval = 5 * time.Second

const AzureProviderImport = "az"
const DeploymentProviderImport = "deployment"
const RadiusProviderImport = "radius"

var _ Provider = (*AzureProvider)(nil)

type AzureProvider struct {
	Authorizer     autorest.Authorizer
	BaseURL        string
	SubscriptionID string
	ResourceGroup  string
	RoundTripper   http.RoundTripper
}

var _ autorest.Sender = (*sender)(nil)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

func (p *AzureProvider) createClient() resources.Client {
	client := azclients.NewGenericResourceClient(p.SubscriptionID, p.Authorizer)
	client.BaseURI = strings.TrimSuffix(p.BaseURL, "/")
	client.PollingDelay = PollInterval
	if p.RoundTripper != nil {
		client.Sender = &sender{RoundTripper: p.RoundTripper}
	}

	return client
}

func (p *AzureProvider) GetDeployedResource(ctx context.Context, id string, version string) (map[string]interface{}, error) {
	client := p.createClient()

	generic, err := client.GetByID(ctx, id, version)
	if err != nil {
		return nil, fmt.Errorf("failed to GET resource %s: %w", id, err)
	}

	b, err := json.Marshal(&generic)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response %q: %w", id, err)
	}

	result := map[string]interface{}{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response %q: %w", id, err)
	}

	return result, nil
}

func (p *AzureProvider) DeployResource(ctx context.Context, id string, version string, body map[string]interface{}) (map[string]interface{}, error) {
	client := p.createClient()

	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resource to JSON: %w", err)
	}

	converted := resources.GenericResource{}
	err = json.Unmarshal(b, &converted)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resource JSON to %T: %w", converted, err)
	}

	future, err := client.CreateOrUpdateByID(ctx, strings.TrimPrefix(id, "/"), version, converted)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT resource %q: %w", id, err)
	}

	err = future.WaitForCompletionRef(ctx, client.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT resource %q: %w", id, err)
	}

	generic, err := future.Result(client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT resource %q: %w", id, err)
	}

	b, err = json.Marshal(&generic)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response %q: %w", id, err)
	}

	result := map[string]interface{}{}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response %q: %w", id, err)
	}

	return result, nil
}

func (p *AzureProvider) InvokeCustomAction(ctx context.Context, id string, version string, action string, body interface{}) (map[string]interface{}, error) {
	client := azclients.NewCustomActionClient(p.SubscriptionID, p.Authorizer)
	client.BaseURI = strings.TrimSuffix(p.BaseURL, "/")
	client.PollingDelay = PollInterval
	client.RetryAttempts = 10
	client.RetryDuration = 3 * time.Second

	if p.RoundTripper != nil {
		client.Sender = &sender{RoundTripper: p.RoundTripper}
	}

	response, err := client.InvokeCustomAction(ctx, id, version, action, body)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke custom action %q: %w", action, err)
	}

	return response.Body, nil
}

func (p *AzureProvider) GetResourceGroup(ctx context.Context) (map[string]interface{}, error) {
	client := azclients.NewGroupsClient(p.SubscriptionID, p.Authorizer)
	client.BaseURI = strings.TrimSuffix(p.BaseURL, "/")
	client.PollingDelay = PollInterval
	client.RetryAttempts = 10
	client.RetryDuration = 3 * time.Second

	if p.RoundTripper != nil {
		client.Sender = &sender{RoundTripper: p.RoundTripper}
	}

	// Resource group has custom marshaling AND the original response body is closed by the SDK function
	// so we have to use the guts of the SDK function for getting a resource group :-|
	request, err := client.GetPreparer(ctx, p.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group: %w", err)
	}

	response, err := client.GetSender(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group: %w", err)
	}

	result := map[string]interface{}{}
	err = autorest.Respond(
		response,
		azure.WithErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (p *AzureProvider) GetEnvironment(ctx context.Context) (map[string]interface{}, error) {
	// We can query the metadata for all clouds using https://management.azure.com/metadata/endpoints/?api-version=2020-06-01
	// We just want the AzureCloud result for now.
	sender := p.RoundTripper
	if p.RoundTripper == nil {
		sender = http.DefaultTransport
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://management.azure.com/metadata/endpoints/?api-version=2020-06-01", nil)
	if err != nil {
		return nil, err
	}

	response, err := sender.RoundTrip(request)
	if err != nil {
		return nil, err
	}

	// Yeah it returns a top-level JSON array :-/
	environments := []map[string]interface{}{}
	err = autorest.Respond(
		response,
		azure.WithErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&environments),
		autorest.ByClosing())
	if err != nil {
		return nil, err
	}

	for _, env := range environments {
		if env["name"] == "AzureCloud" {
			return env, nil
		}
	}

	return nil, fmt.Errorf("could not find AzureCloud environment details")
}
