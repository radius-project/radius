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
	"strings"

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

// V3 API
func (dm *ARMManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) (*radclientv3.RadiusResourceList, error) {
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

func (dm *ARMManagementClient) ListApplicationsV3(ctx context.Context) (*radclientv3.ApplicationList, error) {
	ac := radclientv3.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.List(ctx, dm.ResourceGroup, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Applications not found in environment '%s'", dm.EnvironmentName)
			return nil, radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return nil, err
	}
	return response.ApplicationList, nil
}

func (dm *ARMManagementClient) ShowApplicationV3(ctx context.Context, applicationName string) (*radclientv3.ApplicationResource, error) {
	ac := radclientv3.NewApplicationClient(dm.Connection, dm.SubscriptionID)
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

func (dm *ARMManagementClient) DeleteApplicationV3(ctx context.Context, appName string) error {
	con, sub, rg := dm.Connection, dm.SubscriptionID, dm.ResourceGroup
	radiusResourceClient := radclientv3.NewRadiusResourceClient(con, sub)
	resp, err := radiusResourceClient.List(ctx, dm.ResourceGroup, appName, nil)
	if err != nil {
		if isNotFound(err) {
			errorMessage := fmt.Sprintf("Application  %q not found in environment %q", appName, dm.EnvironmentName)
			return radclientv3.NewRadiusError("ResourceNotFound", errorMessage)
		}
		return err
	}
	for _, resource := range resp.RadiusResourceList.Value {
		types := strings.Split(*resource.Type, "/")
		resourceType := types[len(types)-1]
		poller, err := radclientv3.NewRadiusResourceClient(con, sub).BeginDelete(
			ctx, rg, appName, resourceType, *resource.Name, nil)
		if err != nil {
			return err
		}

		_, err = poller.PollUntilDone(ctx, radclientv3.PollInterval)
		if err != nil {
			if isNotFound(err) {
				errorMessage := fmt.Sprintf("Resource %s/%s not found in application '%s' environment '%s'",
					resourceType, *resource.Name, appName, dm.EnvironmentName)
				return radclient.NewRadiusError("ResourceNotFound", errorMessage)
			}
			return err
		}
	}
	poller, err := radclientv3.NewApplicationClient(con, sub).BeginDelete(ctx, rg, appName, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, radclientv3.PollInterval)
	if isNotFound(err) {
		errorMessage := fmt.Sprintf("Application  %q not found in environment %q", appName, dm.EnvironmentName)
		return radclientv3.NewRadiusError("ResourceNotFound", errorMessage)
	}
	return err
}

func (dm *ARMManagementClient) ShowResource(ctx context.Context, appName string, resourceType string, name string) (interface{}, error) {
	client := radclientv3.NewRadiusResourceClient(dm.Connection, dm.SubscriptionID)
	result, err := client.Get(ctx, dm.ResourceGroup, appName, resourceType, name, nil)
	if err != nil {
		return nil, err
	}
	return result.RadiusResource, nil
}

func isNotFound(err error) bool {
	var httpresp azcore.HTTPResponse
	ok := errors.As(err, &httpresp)
	return ok && httpresp.RawResponse().StatusCode == http.StatusNotFound
}
