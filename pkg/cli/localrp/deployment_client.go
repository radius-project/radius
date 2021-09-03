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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/cli/clients"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
)

const PollInterval = 5 * time.Second

type LocalRPDeploymentClient struct {
	Connection     *armcore.Connection
	SubscriptionID string
	ResourceGroup  string
}

var _ clients.DeploymentClient = (*LocalRPDeploymentClient)(nil)

func (dc *LocalRPDeploymentClient) Deploy(ctx context.Context, content string) error {
	template, err := armtemplate.Parse(content)
	if err != nil {
		return err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{
		SubscriptionID: dc.SubscriptionID,
		ResourceGroup:  dc.ResourceGroup,
	})
	if err != nil {
		return err
	}

	// NOTE: this is currently test-only code so we're fairly noisy about what we output here.
	fmt.Printf("Starting deployment...\n")
	for _, resource := range resources {
		fmt.Printf("Deploying %s %s...\n", resource.Type, resource.Name)
		response, err := dc.deployResource(ctx, dc.Connection, resource)
		if err != nil {
			return fmt.Errorf("failed to PUT resource %s %s: %w", resource.Type, resource.Name, err)
		}

		fmt.Printf("succeed with status code %d\n", response.StatusCode)
	}

	return nil
}

func (dc *LocalRPDeploymentClient) deployResource(ctx context.Context, connection *armcore.Connection, resource armtemplate.Resource) (*http.Response, error) {
	if resource.Type == radresources.ApplicationResourceType.Type() {
		return dc.deployApplication(ctx, connection, resource)
	} else if resource.Type == radresources.ComponentResourceType.Type() {
		return dc.deployComponent(ctx, connection, resource)
	} else if resource.Type == radresources.DeploymentResourceType.Type() {
		return dc.deployDeployment(ctx, connection, resource)
	}

	return nil, fmt.Errorf("unsupported resource type '%s'. radtest only supports radius types", resource.Type)
}

func (dc *LocalRPDeploymentClient) deployApplication(ctx context.Context, connection *armcore.Connection, resource armtemplate.Resource) (*http.Response, error) {
	client := radclient.NewApplicationClient(connection, dc.SubscriptionID)

	names := strings.Split(resource.Name, "/")
	if len(names) != 2 {
		return nil, fmt.Errorf("expected name in format of 'radius/<application>'. was '%s'", resource.Name)
	}

	parameters := radclient.ApplicationCreateParameters{}
	err := resource.Convert(&parameters)
	if err != nil {
		return nil, err
	}

	response, err := client.CreateOrUpdate(ctx, dc.ResourceGroup, names[1], parameters, nil)
	if err != nil {
		return nil, err
	}

	return response.RawResponse, nil
}

func (dc *LocalRPDeploymentClient) deployComponent(ctx context.Context, connection *armcore.Connection, resource armtemplate.Resource) (*http.Response, error) {
	client := radclient.NewComponentClient(connection, dc.SubscriptionID)

	names := strings.Split(resource.Name, "/")
	if len(names) != 3 {
		return nil, fmt.Errorf("expected name in format of 'radius/<application>/<component>'. was '%s'", resource.Name)
	}

	parameters := radclient.ComponentCreateParameters{}
	err := resource.Convert(&parameters)
	if err != nil {
		return nil, err
	}

	response, err := client.CreateOrUpdate(ctx, dc.ResourceGroup, names[1], names[2], parameters, nil)
	if err != nil {
		return nil, err
	}

	return response.RawResponse, nil
}

func (dc *LocalRPDeploymentClient) deployDeployment(ctx context.Context, connection *armcore.Connection, resource armtemplate.Resource) (*http.Response, error) {
	client := radclient.NewDeploymentClient(connection, dc.SubscriptionID)

	names := strings.Split(resource.Name, "/")
	if len(names) != 3 {
		return nil, fmt.Errorf("expected name in format of 'radius/<application>/<deployment>'. was '%s'", resource.Name)
	}

	parameters := radclient.DeploymentCreateParameters{}
	err := resource.Convert(&parameters)
	if err != nil {
		return nil, err
	}

	poller, err := client.BeginCreateOrUpdate(ctx, dc.ResourceGroup, names[1], names[2], parameters, nil)
	if err != nil {
		return nil, err
	}

	response, err := poller.PollUntilDone(ctx, PollInterval)
	if err != nil {
		return nil, err
	}

	return response.RawResponse, nil
}
