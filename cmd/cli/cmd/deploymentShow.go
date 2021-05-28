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
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
)

// deploymentShowCmd command to show details of a deployment
var deploymentShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show Radius deployment details",
	Long:  "Show details of the specified Radius deployment deployed in the default environment",
	RunE:  showDeployment,
}

func init() {
	deploymentCmd.AddCommand(deploymentShowCmd)
}

func showDeployment(cmd *cobra.Command, args []string) error {
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	depName, err := rad.RequireDeployment(cmd, args)
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)

	response, err := dc.Get(cmd.Context(), env.ResourceGroup, applicationName, depName, nil)
	if err != nil {
		var httpresp azcore.HTTPResponse
		if ok := errors.As(err, &httpresp); ok && httpresp.RawResponse().StatusCode == http.StatusNotFound {
			errorMessage := fmt.Sprintf("Deployment '%s' for application '%s' and resource group '%s' was not found.", depName, applicationName, env.ResourceGroup)
			return radclient.NewRadiusError("ResourceNotFound", errorMessage)
		}

		return utils.UnwrapErrorFromRawResponse(err)
	}

	deploymentResource := *response.DeploymentResource
	deploymentDetails, err := json.MarshalIndent(deploymentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deploymentDetails))

	return err
}
