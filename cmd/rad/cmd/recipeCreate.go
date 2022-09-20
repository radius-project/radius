// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	coreRpApps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

var recipeCreateCmd = &cobra.Command{
	Use:     "create",
	Short:   "Add a connector recipe to the environment.",
	Long:    `Add a connector recipe to the environment.`,
	Example: `rad recipe create --templatePath template_path --connectorType Applications.Connector/mongoDatabases --recipeName cosmosdb -e env_name -w workspace`,
	RunE:    recipeCreate,
}

func init() {
	recipeCmd.AddCommand(recipeCreateCmd)
	recipeCreateCmd.Flags().String("templatePath", "", "specify the path to the template provided by the recipe.")
	recipeCreateCmd.Flags().String("connectorType", "", "specify the type of the connector this recipe can be consumed by")
	recipeCreateCmd.Flags().String("recipeName", "", "specify the name of the recipe")
}

func recipeCreate(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	environmentName, err := cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}

	templatePath, err := requireTemplatePath(cmd)
	if err != nil {
		return err
	}
	connectorType, err := requireConnectorType(cmd)
	if err != nil {
		return err
	}
	recipeName, err := requireRecipeName(cmd)
	if err != nil {
		return err
	}

	scopeId, err := resources.Parse(workspace.Scope)
	if err != nil {
		return err
	}
	resourceGroupName := scopeId.FindScope(resources.ResourceGroupsSegment)

	var contextName string
	_, _, contextName, err = createKubernetesClients("")
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
	if envResource.Properties.Recipes != nil {
		envResource.Properties.Recipes[recipeName] = &coreRpApps.EnvironmentRecipeProperties{
			ConnectorType: &connectorType,
			TemplatePath:  &templatePath,
		}
	} else {
		envResource.Properties.Recipes = map[string]*coreRpApps.EnvironmentRecipeProperties{
			recipeName: {
				ConnectorType: &connectorType,
				TemplatePath:  &templatePath,
			},
		}
	}
	baseURL, transporter, err := kubernetes.CreateAPIServerTransporter(contextName, "")
	if err != nil {
		return fmt.Errorf("failed to create environment client: %w", err)
	}

	rootScope := fmt.Sprintf("planes/radius/local/resourceGroups/%s", resourceGroupName)

	envClient, err := coreRpApps.NewEnvironmentsClient(rootScope, &aztoken.AnonymousCredential{}, connections.GetClientOptions(baseURL, transporter))
	if err != nil {
		return fmt.Errorf("failed to create environment client: %w", err)
	}

	_, err = envClient.CreateOrUpdate(cmd.Context(), environmentName, envResource, nil)
	if err != nil {
		return fmt.Errorf("failed to update Applications.Core/environments resource with recipe: %w", err)
	}
	output.LogInfo("Successfully linked recipe %q to environment %q ", recipeName, environmentName)
	return nil
}

func requireTemplatePath(cmd *cobra.Command) (string, error) {
	templatePath, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return templatePath, err
	}
	return templatePath, nil

}

func requireConnectorType(cmd *cobra.Command) (string, error) {
	connectorType, err := cmd.Flags().GetString("connectorType")
	if err != nil {
		return connectorType, err
	}
	return connectorType, nil
}

func requireRecipeName(cmd *cobra.Command) (string, error) {
	recipeName, err := cmd.Flags().GetString("recipeName")
	if err != nil {
		return recipeName, err
	}
	return recipeName, nil
}
