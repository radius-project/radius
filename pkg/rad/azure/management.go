// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad/clients"
	"github.com/Azure/radius/pkg/radclient"
)

type ARMManagementClient struct {
	AzCred         *azidentity.ChainedTokenCredential
	Connection     *armcore.Connection
	ResourceGroup  string
	SubscriptionID string
}

var _ clients.ManagementClient = (*ARMManagementClient)(nil)

func (dm *ARMManagementClient) ListApplications(ctx context.Context) error {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.ListByResourceGroup(ctx, dm.ResourceGroup, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	applicationsList := *response.ApplicationList
	applications, err := json.MarshalIndent(applicationsList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applications))

	return nil
}

func (dm *ARMManagementClient) ShowApplication(ctx context.Context, applicationName string) error {
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)
	response, err := ac.Get(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	applicationResource := *response.ApplicationResource
	applicationDetails, err := json.MarshalIndent(applicationResource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applicationDetails))

	return nil
}

func (dm *ARMManagementClient) DeleteApplication(ctx context.Context, applicationName string) error {

	// Delete deployments: An application can have multiple deployments in it that should be deleted before the application can be deleted.
	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)

	// Retrieve all the deployments in the application
	response, err := dc.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	// Delete the deployments
	deploymentResources := *response.DeploymentList
	for _, deploymentResource := range *deploymentResources.Value {
		// This is needed until server side implementation is fixed https://github.com/Azure/radius/issues/159
		deploymentName := *deploymentResource.Name

		poller, err := dc.BeginDelete(ctx, dm.ResourceGroup, applicationName, deploymentName, nil)
		if err != nil {
			return utils.UnwrapErrorFromRawResponse(err)
		}

		_, err = poller.PollUntilDone(ctx, radclient.PollInterval)
		if err != nil {
			return utils.UnwrapErrorFromRawResponse(err)
		}

		fmt.Printf("Deleted deployment '%s'\n", deploymentName)
	}

	// Delete application
	ac := radclient.NewApplicationClient(dm.Connection, dm.SubscriptionID)

	_, err = ac.Delete(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}
	fmt.Printf("Application '%s' has been deleted\n", applicationName)

	// TODO
	// return dm.updateApplicationConfig(applicationName, ac)
	return nil
}

func (dm *ARMManagementClient) ListComponents(ctx context.Context, applicationName string) error {
	componentClient := radclient.NewComponentClient(dm.Connection, dm.SubscriptionID)

	response, err := componentClient.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	componentsList := *response.ComponentList
	components, err := json.MarshalIndent(componentsList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal component response as JSON %w", err)
	}
	fmt.Println(string(components))

	return err
}

func (dm *ARMManagementClient) ShowComponent(ctx context.Context, applicationName string, componentName string) error {
	componentClient := radclient.NewComponentClient(dm.Connection, dm.SubscriptionID)

	response, err := componentClient.Get(ctx, dm.ResourceGroup, applicationName, componentName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	componentResource := *response.ComponentResource
	componentDetails, err := json.MarshalIndent(componentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal component response as JSON %w", err)
	}
	fmt.Println(string(componentDetails))

	return err
}

func (dm *ARMManagementClient) DeleteDeployment(ctx context.Context, deploymentName string, applicationName string) error {
	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)
	poller, err := dc.BeginDelete(ctx, dm.ResourceGroup, applicationName, deploymentName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	_, err = poller.PollUntilDone(ctx, radclient.PollInterval)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	fmt.Printf("Deployment '%s' deleted.\n", deploymentName)
	return err
}

func (dm *ARMManagementClient) ListDeployments(ctx context.Context, applicationName string) error {

	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)

	response, err := dc.ListByApplication(ctx, dm.ResourceGroup, applicationName, nil)
	if err != nil {
		var httpresp azcore.HTTPResponse
		if ok := errors.As(err, &httpresp); ok && httpresp.RawResponse().StatusCode == http.StatusNotFound {
			errorMessage := fmt.Sprintf("application '%s' was not found in the resource group '%s'", applicationName, dm.ResourceGroup)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return utils.UnwrapErrorFromRawResponse(err)
	}

	deploymentsList := *response.DeploymentList
	deployments, err := json.MarshalIndent(deploymentsList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deployments))

	return err
}

func (dm *ARMManagementClient) ShowDeployment(ctx context.Context, deploymentName string, applicationName string) error {
	dc := radclient.NewDeploymentClient(dm.Connection, dm.SubscriptionID)

	response, err := dc.Get(ctx, dm.ResourceGroup, applicationName, deploymentName, nil)
	if err != nil {
		var httpresp azcore.HTTPResponse
		if ok := errors.As(err, &httpresp); ok && httpresp.RawResponse().StatusCode == http.StatusNotFound {
			errorMessage := fmt.Sprintf("deployment '%s' for application '%s' and resource group '%s' was not found", deploymentName, applicationName, dm.ResourceGroup)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return utils.UnwrapErrorFromRawResponse(err)
	}

	deploymentResource := *response.DeploymentResource
	deploymentDetails, err := json.MarshalIndent(deploymentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deploymentDetails))
	return err
}
