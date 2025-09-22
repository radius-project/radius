/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	corerpv20231001 "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/spf13/cobra"
)

const (
	defaultPlaneScope = "/planes/radius/local"
	ucpAPIServiceName = "v1alpha3.api.ucp.dev"
	logWarningPrefix  = "Warning"
	defaultNamespace  = "default"
)

var radiusCRDs = []string{
	"deploymentresources.radapp.io",
	"deploymenttemplates.radapp.io",
	"recipes.radapp.io",
	"queuemessages.ucp.dev",
	"resources.ucp.dev",
}

type environmentCleanup struct {
	ID        string
	Namespace string
}

type cleanupPlan struct {
	HelmReleases        []string
	Environments        []environmentCleanup
	Namespaces          []string
	ProtectedNamespaces []string
	CRDs                []string
	APIServices         []string
	EnvDiscoveryFailed  bool
}

// NewCommand creates an instance of the `rad <fill in the blank>` command and runner.
//

// NewCommand creates a new Cobra command for uninstalling Radius from a Kubernetes cluster, which takes in a factory
// object and returns a Cobra command and a Runner object.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "kubernetes",
		Short: "Uninstall Radius from a Kubernetes cluster",
		Long:  `Uninstall Radius from a Kubernetes cluster.`,
		Example: `# uninstall Radius from the current Kubernetes cluster
rad uninstall kubernetes

# uninstall Radius from a specific Kubernetes cluster based on the Kubeconfig context
rad uninstall kubernetes --kubecontext my-kubecontext`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddKubeContextFlagVar(cmd, &runner.KubeContext)
	cmd.Flags().BoolVar(&runner.Purge, "purge", false, "Delete all data stored by Radius.")
	cmd.Flags().BoolVarP(&runner.AssumeYes, "yes", "y", false, "Automatically confirm uninstall prompts.")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad uninstall kubernetes` command.
type Runner struct {
	Helm        helm.Interface
	Output      output.Interface
	Kubernetes  kubernetes.Interface
	Connections connections.Factory
	Prompter    prompt.Interface

	KubeContext string
	Purge       bool
	AssumeYes   bool
}

// NewRunner creates an instance of the runner for the `rad uninstall kubernetes` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Helm:        factory.GetHelmInterface(),
		Output:      factory.GetOutput(),
		Kubernetes:  factory.GetKubernetesInterface(),
		Connections: factory.GetConnectionFactory(),
		Prompter:    factory.GetPrompter(),
		AssumeYes:   false,
	}
}

// Validate runs validation for the `rad uninstall kubernetes` command.
//

// Validate checks the command and arguments passed to it and returns an error if any of them are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

// Run runs the `rad uninstall kubernetes` command.
//

// Run checks if Radius is installed on the Kubernetes cluster, and if so, uninstalls it, logging a success message
// if successful. It returns an error if an error occurs during the uninstallation.
func (r *Runner) Run(ctx context.Context) error {
	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return err
	}
	if !state.RadiusInstalled {
		r.Output.LogInfo("Radius is not installed on the Kubernetes cluster")
		return nil
	}

	plan, err := r.buildCleanupPlan(ctx, state)
	if err != nil {
		r.Output.LogInfo("%s: %v", logWarningPrefix, err)
	}

	r.describeCleanupPlan(plan)
	confirmed := true
	if !r.AssumeYes {
		if r.Prompter == nil {
			return fmt.Errorf("confirmation prompt unavailable; rerun with --yes to proceed non-interactively")
		}
		confirmed, err = prompt.YesOrNoPrompt("Continue uninstalling Radius?", prompt.ConfirmNo, r.Prompter)
		if err != nil {
			return err
		}
	} else {
		r.Output.LogInfo("Skipping confirmation because --yes flag was provided.")
	}
	if !confirmed {
		r.Output.LogInfo("Uninstall cancelled.")
		return nil
	}

	if r.Purge {
		if plan.EnvDiscoveryFailed {
			r.Output.LogInfo("%s: skipping Radius environment deletion because the Radius management APIs could not be reached", logWarningPrefix)
		} else if err := r.deleteEnvironments(ctx, plan.Environments); err != nil {
			// Environment removal via the management APIs is best-effort; namespace cleanup guarantees resources are gone.
			r.Output.LogInfo("%s: failed to delete environments via Radius APIs: %v", logWarningPrefix, err)
		}
	}

	err = r.Helm.UninstallRadius(ctx, helm.NewDefaultClusterOptions(), r.KubeContext)
	if err != nil {
		return err
	}

	if r.Purge {
		if len(plan.APIServices) > 0 {
			for _, svc := range plan.APIServices {
				r.Output.LogInfo("Removing APIService %s", svc)
			}
		}
		if len(plan.CRDs) > 0 {
			r.Output.LogInfo("Removing Radius custom resource definitions")
		}
		for _, ns := range plan.Namespaces {
			r.Output.LogInfo("Deleting namespace %s", ns)
		}
		for _, ns := range plan.ProtectedNamespaces {
			r.Output.LogInfo("Skipping deletion of namespace %s because Kubernetes does not allow deletion", ns)
		}
		r.Output.LogInfo("Waiting for resource deletions to finish...")

		cleanupPlan := kubernetes.CleanupPlan{
			Namespaces:  plan.Namespaces,
			APIServices: plan.APIServices,
			CRDs:        plan.CRDs,
		}
		if err := r.Kubernetes.PerformRadiusCleanup(ctx, r.KubeContext, cleanupPlan); err != nil {
			return err
		}
		r.Output.LogInfo("Radius was fully uninstalled. All data has been removed.")
		return nil
	}
	r.Output.LogInfo("Radius was uninstalled successfully. Any existing data will be retained for future installations. Local configuration is also retained. Use the `rad workspace` command if updates are needed to your configuration.")
	return nil
}

func (r *Runner) buildCleanupPlan(ctx context.Context, state helm.InstallState) (cleanupPlan, error) {
	plan := cleanupPlan{}
	plan.HelmReleases = append(plan.HelmReleases, "radius")
	if state.ContourInstalled {
		plan.HelmReleases = append(plan.HelmReleases, "contour")
	}

	if !r.Purge {
		return plan, nil
	}

	plan.CRDs = append(plan.CRDs, radiusCRDs...)
	plan.APIServices = append(plan.APIServices, ucpAPIServiceName)
	namespaceSet := map[string]struct{}{
		helm.RadiusSystemNamespace: {},
	}
	protectedNamespaceSet := map[string]struct{}{}
	plan.Namespaces = append(plan.Namespaces, helm.RadiusSystemNamespace)

	environments, err := r.fetchEnvironmentCleanupInfos(ctx)
	if err != nil {
		plan.EnvDiscoveryFailed = true
		r.Output.LogInfo("%s: unable to enumerate Radius environments via Radius APIs: %v", logWarningPrefix, err)
		return plan, nil
	}
	plan.Environments = environments
	for _, env := range environments {
		if env.Namespace == "" {
			continue
		}
		if _, exists := namespaceSet[env.Namespace]; exists {
			continue
		}
		if env.Namespace == defaultNamespace {
			if _, exists := protectedNamespaceSet[env.Namespace]; exists {
				continue
			}
			protectedNamespaceSet[env.Namespace] = struct{}{}
			plan.ProtectedNamespaces = append(plan.ProtectedNamespaces, env.Namespace)
			continue
		}
		namespaceSet[env.Namespace] = struct{}{}
		plan.Namespaces = append(plan.Namespaces, env.Namespace)
	}

	return plan, nil
}

func (r *Runner) describeCleanupPlan(plan cleanupPlan) {
	r.Output.LogInfo("About to uninstall Radius. This will remove:")
	if len(plan.HelmReleases) > 0 {
		r.Output.LogInfo("- Helm releases: %s", strings.Join(plan.HelmReleases, ", "))
	}

	if r.Purge {
		if len(plan.Environments) > 0 {
			r.Output.LogInfo("- Radius environments:")
			for _, env := range plan.Environments {
				r.Output.LogInfo("  • %s (namespace %s)", env.ID, env.Namespace)
			}
		} else if plan.EnvDiscoveryFailed {
			r.Output.LogInfo("- Radius environments: unable to determine (Radius management APIs unreachable)")
		} else {
			r.Output.LogInfo("- Radius environments: none")
		}

		if len(plan.Namespaces) > 0 {
			r.Output.LogInfo("- Kubernetes namespaces: %s", strings.Join(plan.Namespaces, ", "))
		}
		if len(plan.ProtectedNamespaces) > 0 {
			r.Output.LogInfo("- Kubernetes namespaces (skipped): %s", strings.Join(plan.ProtectedNamespaces, ", "))
		}
		if len(plan.APIServices) > 0 {
			r.Output.LogInfo("- Kubernetes API services: %s", strings.Join(plan.APIServices, ", "))
		}
		if len(plan.CRDs) > 0 {
			r.Output.LogInfo("- Kubernetes custom resource definitions: %s", strings.Join(plan.CRDs, ", "))
		}
	}
}

func (r *Runner) fetchEnvironmentCleanupInfos(ctx context.Context) ([]environmentCleanup, error) {
	workspace, err := r.workspaceForContext()
	if err != nil {
		return nil, err
	}

	client, err := r.Connections.CreateApplicationsManagementClient(ctx, *workspace)
	if err != nil {
		return nil, fmt.Errorf("creating applications client: %w", err)
	}

	environmentResources, err := client.ListEnvironmentsAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}

	infos := make([]environmentCleanup, 0, len(environmentResources))
	for _, env := range environmentResources {
		namespace := getEnvironmentNamespace(env)
		if namespace == "" {
			continue
		}

		id := firstNonEmpty(derefString(env.ID), derefString(env.Name))
		infos = append(infos, environmentCleanup{ID: id, Namespace: namespace})
	}

	return infos, nil
}

func (r *Runner) deleteEnvironments(ctx context.Context, environments []environmentCleanup) error {
	if len(environments) == 0 {
		return nil
	}

	workspace, err := r.workspaceForContext()
	if err != nil {
		return err
	}

	client, err := r.Connections.CreateApplicationsManagementClient(ctx, *workspace)
	if err != nil {
		return fmt.Errorf("creating applications client: %w", err)
	}

	var errs []error
	for _, env := range environments {
		if env.ID == "" {
			errs = append(errs, fmt.Errorf("environment targeting namespace %s is missing identifier", env.Namespace))
			continue
		}
		r.Output.LogInfo("Deleting environment %s", env.ID)
		if _, err := client.DeleteEnvironment(ctx, env.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete environment %s: %w", env.ID, err))
		}
	}

	return errors.Join(errs...)
}

func (r *Runner) workspaceForContext() (*workspaces.Workspace, error) {
	workspace := workspaces.MakeFallbackWorkspace()
	if workspace.Connection == nil {
		workspace.Connection = map[string]any{}
	}
	workspace.Connection["context"] = r.KubeContext
	workspace.Scope = defaultPlaneScope
	return workspace, nil
}

func getEnvironmentNamespace(env corerpv20231001.EnvironmentResource) string {
	if env.Properties == nil || env.Properties.Compute == nil {
		return ""
	}

	switch compute := env.Properties.Compute.(type) {
	case *corerpv20231001.KubernetesCompute:
		if compute.Namespace != nil {
			return *compute.Namespace
		}
	}

	return ""
}

func derefString(str *string) string {
	if str == nil {
		return ""
	}
	return *str
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
