// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package list

import (
	"context"
	"errors"
	"fmt"

	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	coreRpApps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "create",
		Short:   "Add a connector recipe to the environment.",
		Long:    `Add a connector recipe to the environment.`,
		Example: `rad recipe create --name cosmosdb -e env_name -w workspace --templatePath template_path --connectorType Applications.Connector/mongoDatabases`,
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().String("templatePath", "", "specify the path to the template provided by the recipe.")
	cmd.Flags().String("connectorType", "", "specify the type of the connector this recipe can be consumed by")
	cmd.Flags().String("name", "", "specify the name of the recipe")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	Format            string
	TemplatePath      string
	ConnectorType     string
	RecipeName        string
	ResourceGroupName string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	environmentName, err := cli.RequireEnvironmentName(cmd, args, *workspace)
	if err != nil {
		return err
	}
	r.Workspace.Environment = environmentName

	templatePath, err := requireTemplatePath(cmd)
	if err != nil {
		return err
	}
	r.TemplatePath = templatePath

	connectorType, err := requireConnectorType(cmd)
	if err != nil {
		return err
	}
	r.ConnectorType = connectorType

	recipeName, err := requireRecipeName(cmd)
	if err != nil {
		return err
	}
	r.RecipeName = recipeName

	scopeId, err := resources.ParseScope(workspace.Scope)
	if err != nil {
		return err
	}
	r.ResourceGroupName = scopeId.FindScope(resources.ResourceGroupsSegment)

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	var contextName string
	_, _, contextName, err := createKubernetesClients("")
	if err != nil {
		return err
	}
	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}
	envResource, err := client.GetEnvDetails(ctx, r.Workspace.Environment)
	if err != nil {
		return err
	}
	if envResource.Properties.Recipes != nil {
		envResource.Properties.Recipes[r.RecipeName] = &coreRpApps.EnvironmentRecipeProperties{
			ConnectorType: &r.ConnectorType,
			TemplatePath:  &r.TemplatePath,
		}
	} else {
		envResource.Properties.Recipes = map[string]*coreRpApps.EnvironmentRecipeProperties{
			r.RecipeName: {
				ConnectorType: &r.ConnectorType,
				TemplatePath:  &r.TemplatePath,
			},
		}
	}
	baseURL, transporter, err := kubernetes.CreateAPIServerTransporter(contextName, "")
	if err != nil {
		return fmt.Errorf("failed to create environment client: %w", err)
	}

	rootScope := fmt.Sprintf("planes/radius/local/resourceGroups/%s", r.ResourceGroupName)

	envClient, err := coreRpApps.NewEnvironmentsClient(rootScope, &aztoken.AnonymousCredential{}, connections.GetClientOptions(baseURL, transporter))
	if err != nil {
		return fmt.Errorf("failed to create environment client: %w", err)
	}

	_, err = envClient.CreateOrUpdate(ctx, r.Workspace.Environment, envResource, nil)
	if err != nil {
		return fmt.Errorf("failed to update Applications.Core/environments resource with recipe: %w", err)
	}
	r.Output.LogInfo("Successfully linked recipe %q to environment %q ", r.RecipeName, r.Workspace.Environment)
	return nil
}

func requireTemplatePath(cmd *cobra.Command) (string, error) {
	templatePath, err := cmd.Flags().GetString("templatePath")
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
	recipeName, err := cmd.Flags().GetString("name")
	if err != nil {
		return recipeName, err
	}
	return recipeName, nil
}

func createKubernetesClients(contextName string) (client_go.Interface, runtime_client.Client, string, error) {
	k8sConfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return nil, nil, "", err
	}

	if contextName == "" && k8sConfig.CurrentContext == "" {
		return nil, nil, "", errors.New("no kubernetes context is set")
	} else if contextName == "" {
		contextName = k8sConfig.CurrentContext
	}

	context := k8sConfig.Contexts[contextName]
	if context == nil {
		return nil, nil, "", fmt.Errorf("kubernetes context '%s' could not be found", contextName)
	}

	client, _, err := kubernetes.CreateTypedClient(contextName)
	if err != nil {
		return nil, nil, "", err
	}

	runtimeClient, err := kubernetes.CreateRuntimeClient(contextName, kubernetes.Scheme)
	if err != nil {
		return nil, nil, "", err
	}

	return client, runtimeClient, contextName, nil
}
