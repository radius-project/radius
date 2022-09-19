// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// recipeListCmd command returns properties of an environment
var recipeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environment recipes",
	Long:  "List recipes linked to the Radius environment",
	RunE:  listRecipes,
}

func init() {
	recipeCmd.AddCommand(recipeListCmd)
}

func listRecipes(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())

	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	environmentName, err := cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	envResource, err := client.GetEnvDetails(cmd.Context(), environmentName)
	if err != nil {
		return err
	}

	var envRecipes []EnvironmentRecipe
	for recipeName, recipeProperties := range envResource.Properties.Recipes {
		recipe := EnvironmentRecipe{
			Name:          recipeName,
			ConnectorType: *recipeProperties.ConnectorType,
			TemplatePath:  *recipeProperties.TemplatePath,
		}
		envRecipes = append(envRecipes, recipe)
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, envRecipes, cmd.OutOrStdout(), objectformats.GetEnvironmentRecipesTableFormat())
	if err != nil {
		return err
	}

	return nil
}

type EnvironmentRecipe struct {
	Name          string `json:"name,omitempty"`
	ConnectorType string `json:"connectorType,omitempty"`
	TemplatePath  string `json:"templatePath,omitempty"`
}
