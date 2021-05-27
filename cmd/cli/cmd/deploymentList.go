// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
)

// deploymentListCmd command to list deployments in an application
var deploymentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application deployments",
	Long:  "List all the deployments in the specified application",
	RunE:  listDeployments,
}

func init() {
	deploymentCmd.AddCommand(deploymentListCmd)
}

func listDeployments(cmd *cobra.Command, args []string) error {
	env, err := requireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := requireApplication(cmd, env)
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)

	response, err := dc.ListByApplication(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		var httpresp azcore.HTTPResponse
		if ok := errors.As(err, &httpresp); ok && httpresp.RawResponse().StatusCode == http.StatusNotFound {
			errorMessage := fmt.Sprintf("Application '%s' was not found in the resource group '%s'.", applicationName, env.ResourceGroup)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return utils.UnwrapErrorFromRawResponse(err)
	}

	deploymentsList := *response.DeploymentList
	deployments, err := json.MarshalIndent(deploymentsList, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deployments))

	return err
}
