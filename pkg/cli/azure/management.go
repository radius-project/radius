// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/clients"
)

type ARMManagementClient struct {
	Connection     *armcore.Connection
	ResourceGroup  string
	SubscriptionID string
}

var _ clients.ManagementClient = (*ARMManagementClient)(nil)

func (dm *ARMManagementClient) ListApplications(ctx context.Context) (*radclient.ApplicationList, error) {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.ListByResourceGroup(ctx, dm.ResourceGroup, nil)
	if err != nil {
		return nil, err
	}

	return response.ApplicationList, nil
}

func (dm *ARMManagementClient) ShowApplication(ctx context.Context, applicationName string) (*radclient.ApplicationResource, error) {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.Get(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return nil, err
	}

	return response.ApplicationResource, err
}

func (dm *ARMManagementClient) DeleteApplication(ctx context.Context, applicationName string) error {
	// Delete application
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)

	_, err := ac.Delete(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return err
	}

	return err
}

func (dm *ARMManagementClient) ListComponents(ctx context.Context, applicationName string) (*radclient.ComponentList, error) {
	componentClient := radclient.NewComponentClient(dm.Connection, dm.SubscriptionID)

	response, err := componentClient.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return nil, err
	}
	return response.ComponentList, err
}

func (dm *ARMManagementClient) ShowComponent(ctx context.Context, applicationName string, componentName string) (*radclient.ComponentResource, error) {
	componentClient := radclient.NewComponentClient(dm.Connection, dm.SubscriptionID)

	response, err := componentClient.Get(ctx, dm.ResourceGroup, applicationName, componentName, nil)
	if err != nil {
		return nil, err
	}

	return response.ComponentResource, err
}

func (dm *ARMManagementClient) DeleteDeployment(ctx context.Context, applicationName string, deploymentName string) error {
	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)
	poller, err := dc.BeginDelete(ctx, dm.ResourceGroup, applicationName, deploymentName, nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, radclient.PollInterval)
	if err != nil {
		return err
	}
	return err
}

func (dm *ARMManagementClient) ListDeployments(ctx context.Context, applicationName string) (*radclient.DeploymentList, error) {

	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)

	response, err := dc.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		var httpresp azcore.HTTPResponse
		if ok := errors.As(err, &httpresp); ok && httpresp.RawResponse().StatusCode == http.StatusNotFound {
			errorMessage := fmt.Sprintf("application '%s' was not found in the resource group '%s'", applicationName, dm.ResourceGroup)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return nil, err
	}

	return response.DeploymentList, err
}

func (dm *ARMManagementClient) ShowDeployment(ctx context.Context, deploymentName string, applicationName string) (*radclient.DeploymentResource, error) {
	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)

	response, err := dc.Get(ctx, dm.ResourceGroup, applicationName, deploymentName, nil)
	if err != nil {
		var httpresp azcore.HTTPResponse
		if ok := errors.As(err, &httpresp); ok && httpresp.RawResponse().StatusCode == http.StatusNotFound {
			errorMessage := fmt.Sprintf("deployment '%s' for application '%s' and resource group '%s' was not found", deploymentName, applicationName, dm.ResourceGroup)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return nil, err
	}

	return response.DeploymentResource, err
}
