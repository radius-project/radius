// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

// appStatusCmd command to show properties of an application
var appStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Radius application status details",
	Long:  "Show Radius application details, such as public endpoints and resource count.",
	RunE:  showApplicationStatus,
}

func init() {
	applicationCmd.AddCommand(appStatusCmd)
}

func showApplicationStatus(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, ConfigFromContext(cmd.Context()), DirectoryConfigFromContext(cmd.Context()))
	if err != nil {
		return err
	}

	// TODO: support fallback workspace
	if !workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	application, err := cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	applicationsClient, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	resourceList, err := applicationsClient.ListAllResourcesByApplication(cmd.Context(), application)
	if err != nil {
		return err
	}

	applicationStatus := clients.ApplicationStatus{
		Name:          application,
		ResourceCount: len(resourceList),
	}

	diagnosticsClient, err := connections.DefaultFactory.CreateDiagnosticsClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	for _, resource := range resourceList {
		resourceID, err := resources.ParseResource(*resource.ID)
		if err != nil {
			return err
		}

		publicEndpoint, err := diagnosticsClient.GetPublicEndpoint(cmd.Context(), clients.EndpointOptions{
			ResourceID: resourceID,
		})
		if err != nil {
			return err
		}

		if publicEndpoint != nil {
			applicationStatus.Gateways = append(applicationStatus.Gateways, clients.GatewayStatus{
				Name:     *resource.Name,
				Endpoint: *publicEndpoint,
			})
		}
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, applicationStatus, cmd.OutOrStdout(), objectformats.GetApplicationStatusTableFormat())
	if err != nil {
		return err
	}

	if format == output.FormatTable && len(applicationStatus.Gateways) > 0 {
		// Print newline for readability
		fmt.Println()

		err = output.Write(format, applicationStatus.Gateways, cmd.OutOrStdout(), objectformats.GetApplicationGatewaysTableFormat())
		if err != nil {
			return err
		}
	}

	return nil
}
