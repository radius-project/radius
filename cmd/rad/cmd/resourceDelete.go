// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// resourceDeleteCmd is the command to delete a resource
var resourceDeleteCmd = &cobra.Command{
	Use:     "delete [type] [resource]",
	Short:   "Delete a RAD resource",
	Long:    "Deletes a RAD resource with the given name",
	Example: `rad resource delete --application icecream-store containers orders`,
	RunE:    deleteResource,
}

func init() {
	resourceCmd.AddCommand(resourceDeleteCmd)
}

func deleteResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	resourceType, resourceName, err := cli.RequireResourceTypeAndName(args)
	if err != nil {
		return err
	}

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(cmd.Context(), &respFromCtx)

	_, err = client.DeleteResource(ctxWithResp, resourceType, resourceName)
	if err != nil {
		return err
	}

	if respFromCtx.StatusCode == 204 {
		output.LogInfo("Resource '%s' of type '%s' does not exist or has already been deleted", resourceName, resourceType)
	} else {
		output.LogInfo("Resource deleted")
	}

	return nil
}
