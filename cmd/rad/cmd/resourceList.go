// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// resourceListCmd command to list resources in an application
var resourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application resources",
	Long:  "List all the resources in the specified application",
	RunE:  listResources,
}

func init() {
	resourceCmd.AddCommand(resourceListCmd)
}

func listResources(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}
	isUCPEnabled := false
	if env.GetKind() == environments.KindKubernetes {
		isUCPEnabled = env.(*environments.KubernetesEnvironment).GetEnableUCP()
	}
	if isUCPEnabled {
		err := listResourcesUCP(cmd, args, env)
		if err != nil {
			return err
		}
	} else {
		err := listResourcesLegacy(cmd, args, env)
		if err != nil {
			return err
		}
	}
	return nil
}

func listResourcesLegacy(cmd *cobra.Command, args []string, env environments.Environment) error {
	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateLegacyManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	resourceList, err := client.ListAllResourcesByApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	return printOutput(cmd, resourceList.Value, true)
}

func listResourcesUCP(cmd *cobra.Command, args []string, env environments.Environment) error {
	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}
	resourceList, err := client.ListAllResourcesByApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	return printOutput(cmd, resourceList, false)
}

func printOutput(cmd *cobra.Command, obj interface{}, isLegacy bool) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	var formatterOptions output.FormatterOptions
	if !isLegacy {
		formatterOptions = objectformats.GetResourceTableFormat()
	} else {
		formatterOptions = objectformats.GetResourceTableFormatOld()
	}
	err = output.Write(format, obj, cmd.OutOrStdout(), formatterOptions)
	if err != nil {
		return err
	}
	return nil
}
