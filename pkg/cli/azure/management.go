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
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/Azure/radius/pkg/cli/clients"
)

type ARMManagementClient struct {
	Connection      *armcore.Connection
	ResourceGroup   string
	SubscriptionID  string
	EnvironmentName string
}

var _ clients.ManagementClient = (*ARMManagementClient)(nil)

func (dm *ARMManagementClient) ListApplications(ctx context.Context) (*radclient.ApplicationList, error) {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.ListByResourceGroup(ctx, dm.ResourceGroup, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Applications not found in environment '%s'", dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}

	return response.ApplicationList, nil
}

func (dm *ARMManagementClient) ShowApplication(ctx context.Context, applicationName string) (*radclient.ApplicationResource, error) {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.Get(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}

	return response.ApplicationResource, err
}

func (dm *ARMManagementClient) DeleteApplication(ctx context.Context, applicationName string) error {
	// Delete application
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)

	_, err := ac.Delete(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application '%s' not found in environment '%s'", applicationName, dm.EnvironmentName)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return err
	}

	return err
}

func (dm *ARMManagementClient) ListComponents(ctx context.Context, applicationName string) (*radclient.ComponentList, error) {
	componentClient := radclient.NewComponentClient(dm.Connection, dm.SubscriptionID)

	response, err := componentClient.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Components not found in application '%s' and environment '%s'", applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return response.ComponentList, err
}

func (dm *ARMManagementClient) ShowComponent(ctx context.Context, applicationName string, componentName string) (*radclient.ComponentResource, error) {
	componentClient := radclient.NewComponentClient(dm.Connection, dm.SubscriptionID)

	response, err := componentClient.Get(ctx, dm.ResourceGroup, applicationName, componentName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Component '%s' not found in application '%s' and environment '%s'", componentName, applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
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
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Deployment '%s' not found in application '%s' environment '%s'", deploymentName, applicationName, dm.EnvironmentName)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return err
	}

	return err
}

func (dm *ARMManagementClient) ListDeployments(ctx context.Context, applicationName string) (*radclient.DeploymentList, error) {

	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)

	response, err := dc.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Deployments not found in application '%s' and environment '%s'", applicationName, dm.EnvironmentName)
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
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Deployment '%s' not found in application '%s' environment '%s'", deploymentName, applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return nil, err
	}

	return response.DeploymentResource, err
}

func (dm *ARMManagementClient) ListComponentsV3(ctx context.Context, applicationName string) (*radclientv3.RadiusResourceList, error) {
	radiusResourceClient := radclientv3.NewRadiusResourceClient(dm.Connection, dm.SubscriptionID)

	response, err := radiusResourceClient.List(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Resources not found in application '%s' and environment '%s'", applicationName, dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return response.RadiusResourceList, err
}

func isNotFound(err error) bool {
	var httpresp azcore.HTTPResponse
	ok := errors.As(err, &httpresp)
	return ok && httpresp.RawResponse().StatusCode == http.StatusNotFound
}
