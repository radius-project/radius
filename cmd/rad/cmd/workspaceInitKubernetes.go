// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

var workspaceInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initialize local workspace on kubernetes",
	Long:  `Initialize local workspace on kubernetes`,
	RunE:  initWorkspaceKubernetes,
}

func init() {
	workspaceInitCmd.AddCommand(workspaceInitKubernetesCmd)
	workspaceInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Collect values for required command arguments through command line interface prompts")
	workspaceInitKubernetesCmd.Flags().BoolP("force", "f", false, "Overwrite existing workspace if present")
	workspaceInitKubernetesCmd.Flags().String("kubecontext", "", "the Kubernetes context to use, will use the default if unset")

	setup.RegistePersistantAzureProviderArgs(workspaceInitKubernetesCmd)
}

func initWorkspaceKubernetes(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	kubecontext, err := cmd.Flags().GetString("kubecontext")
	if err != nil {
		return err
	}

	// Configure Azure provider for cloud resources if specified
	azureProvider, err := setup.ParseAzureProviderArgs(cmd, interactive)
	if err != nil {
		return err
	}

	name, err := cli.ReadWorkspaceNameArgs(cmd, args)
	if err != nil {
		return err
	}

	if name != "" {
		// Name was specified at the command line - validate uniqueness.
		existing, err := cli.HasWorkspace(config, name)
		if err != nil {
			return err
		}

		if existing && !force {
			return fmt.Errorf("the workspace %q already exists. Specify '--force' to overwrite", name)
		}
	} else if interactive {

		name, err = prompt.Text(
			"Enter the name to use for the workspace:",
			prompt.MatchAll(prompt.ResourceName, setup.ValidateWorkspaceUniqueness(config, force)))
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("the workspace name is required")
	}

	// We validate the context and make sure we actually store a named context (not the empty string)
	kubeconfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return err
	}

	if kubecontext == "" && kubeconfig.CurrentContext == "" {
		return errors.New("the kubeconfig has no current context")
	} else if kubecontext == "" {
		kubecontext = kubeconfig.CurrentContext
	} else {
		_, ok := kubeconfig.Contexts[kubecontext]
		if !ok {
			return fmt.Errorf("the kubeconfig does not contain a context called %q", kubecontext)
		}
	}

	step := output.BeginStep("Creating Workspace...")

	// TODO: we TEMPORARILY create a resource group as part of creating the workspace.
	//
	// We'll flesh this out more when we add explicit commands for managing resource groups.
	id, err := setup.CreateWorkspaceResourceGroup(cmd.Context(), &workspaces.KubernetesConnection{Context: kubecontext}, name)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		workspace := workspaces.Workspace{
			Connection: map[string]interface{}{
				"kind":    "kubernetes",
				"context": kubecontext,
			},
			Scope: id,
		}

		if azureProvider != nil {
			workspace.ProviderConfig.Azure = &workspaces.AzureProvider{
				SubscriptionID: azureProvider.SubscriptionID,
				ResourceGroup:  azureProvider.ResourceGroup,
			}
		}

		name := strings.ToLower(name)
		section.Default = name
		section.Items[name] = workspace

		return nil
	})
	if err != nil {
		return err
	}

	output.LogInfo("Set %q as current workspace", name)
	output.CompleteStep(step)

	return nil
}
