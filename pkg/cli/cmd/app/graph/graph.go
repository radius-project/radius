// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	cligraph "github.com/radius-project/radius/pkg/cli/graph"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	gitstore "github.com/radius-project/radius/pkg/graph/persistence/git"
	"github.com/spf13/cobra"
)

const (
	bicepExtension          = ".bicep"
	defaultModeledGraphFile = "app-graph.json"
	modeledGraphKeyName     = "app-graph"

	// envGitHubActions is set to "true" by GitHub Actions for every step
	// running inside a runner. When present, rad operates in repo-radius
	// mode and persists graph artifacts to the radius-graph orphan branch.
	envGitHubActions = "GITHUB_ACTIONS"

	// envGitHubHeadRef is the source branch of a pull request (e.g.
	// "feature/foo"). Set only for pull_request events.
	envGitHubHeadRef = "GITHUB_HEAD_REF"

	// envGitHubRefName is the short ref that triggered the workflow (e.g.
	// "main" for a push to main, "42/merge" for a pull_request event).
	envGitHubRefName = "GITHUB_REF_NAME"
)

// NewCommand creates an instance of the command and runner for the `rad app graph` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Shows the application graph for an application.",
		Long: `Shows the application graph for an application.

When invoked with the name of a deployed application using the --application flag,
the command queries the Radius control plane and prints the graph of live
resources. When invoked with a path to an app.bicep,
the command compiles the template and writes the resulting modeled graph to
./app-graph.json without contacting the control plane.

If the command runs inside a GitHub Actions runner (GITHUB_ACTIONS=true), the
modeled graph is committed to <source-branch>/app-graph.json on the radius-graph
orphan branch instead of the local filesystem. This is auto-detected; no flag
is required.`,
		Args: cobra.MaximumNArgs(1),
		Example: `
# Show graph for the deployed application named my-application.
rad app graph -a my-application

# Build the modeled graph for an app.bicep and write it to ./app-graph.json.
rad app graph ./app.bicep`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad app graph` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Bicep             bicep.Interface

	// Deployed-graph mode fields.
	ApplicationName string
	Workspace       *workspaces.Workspace

	// Modeled-graph mode field.
	BicepFilePath string

	Format string

	// GraphStore persists modeled graphs to the radius-graph orphan branch
	// when running in repo-radius mode. Defaulted in NewRunner; tests may
	// substitute a mock implementation.
	GraphStore persistence.Store
}

// NewRunner creates a new instance of the `rad app graph` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Bicep:             factory.GetBicep(),
		GraphStore:        factory.GetGraphStore(),
	}
}

// isModeledGraphArg returns true when the positional argument is a path to a
// Bicep file. The deployed-graph form takes an application name, which never
// carries a .bicep extension.
func isModeledGraphArg(arg string) bool {
	if arg == "" {
		return false
	}
	return strings.EqualFold(filepath.Ext(arg), bicepExtension)
}

// inRepoRadiusMode returns true when running inside a GitHub Actions runner.
// Per the design, this is the trigger for committing graph artifacts to the
// radius-graph orphan branch instead of writing them to the local filesystem.
func inRepoRadiusMode() bool {
	return strings.EqualFold(os.Getenv(envGitHubActions), "true")
}

// sourceBranch returns the source branch name to use as the namespace under
// which the modeled graph is committed on the orphan branch. For
// pull_request events GITHUB_HEAD_REF carries the source branch; for push
// events it falls back to GITHUB_REF_NAME.
func sourceBranch() string {
	if ref := os.Getenv(envGitHubHeadRef); ref != "" {
		return ref
	}
	return os.Getenv(envGitHubRefName)
}

// Validate runs validation for the `rad app graph` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	if len(args) == 1 && isModeledGraphArg(args[0]) {
		r.BicepFilePath = args[0]
		return nil
	}
	return r.validateDeployed(cmd, args)
}

// validateDeployed validates inputs for the deployed-graph form of the
// command, which queries the Radius control plane for a live application.
func (r *Runner) validateDeployed(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *r.Workspace)
	if err != nil {
		return err
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	// Validate that the application exists
	_, err = client.GetApplication(cmd.Context(), r.ApplicationName)
	if clients.Is404Error(err) {
		return clierrors.Message("Application %q does not exist or has been deleted.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad app graph` command.
func (r *Runner) Run(ctx context.Context) error {
	if r.BicepFilePath != "" {
		return r.runModeled(ctx)
	}
	return r.runDeployed(ctx)
}

// runDeployed implements the existing flow that fetches the deployed
// application graph from the Radius control plane.
func (r *Runner) runDeployed(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	applicationGraphResponse, err := client.GetApplicationGraph(ctx, r.ApplicationName)
	if err != nil {
		return err
	}

	switch r.Format {
	case output.FormatJson:
		return r.Output.WriteFormatted(r.Format, applicationGraphResponse, output.FormatterOptions{})
	default:
		graph := applicationGraphResponse.Resources
		d := display(graph, r.ApplicationName)
		r.Output.LogInfo(d)

		return nil
	}
}

// runModeled compiles the supplied Bicep file, builds the modeled
// application graph, and persists the result. When running inside a
// GitHub Actions runner the graph is committed to the radius-graph orphan
// branch under <source-branch>/app-graph.json; otherwise it is written to
// ./app-graph.json in the current working directory.
func (r *Runner) runModeled(ctx context.Context) error {
	r.Output.LogInfo("Compiling %s", r.BicepFilePath)
	template, err := r.Bicep.PrepareTemplate(r.BicepFilePath)
	if err != nil {
		return clierrors.Message("Failed to compile %q: %v", r.BicepFilePath, err)
	}

	graph, err := cligraph.BuildModeledGraph(template)
	if err != nil {
		return clierrors.Message("Failed to build modeled graph: %v", err)
	}

	if inRepoRadiusMode() {
		return r.persistToOrphanBranch(ctx, graph)
	}
	return r.writeToLocalFile(graph)
}

// writeToLocalFile serializes graph to ./app-graph.json in the current
// working directory.
func (r *Runner) writeToLocalFile(graph *corerpv20250801preview.ApplicationGraphResponse) error {
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal modeled graph: %w", err)
	}
	if err := os.WriteFile(defaultModeledGraphFile, data, 0o644); err != nil {
		return fmt.Errorf("write modeled graph to %q: %w", defaultModeledGraphFile, err)
	}
	absPath, err := filepath.Abs(defaultModeledGraphFile)
	if err != nil {
		absPath = defaultModeledGraphFile
	}
	r.Output.LogInfo("Parsed %d resources. Wrote modeled graph to %s", len(graph.Resources), absPath)
	return nil
}

// persistToOrphanBranch commits graph to <encoded-source-branch>/app-graph.json
// on the radius-graph orphan branch via the git-backed persistence Store.
//
// The raw branch name is encoded with url.QueryEscape before being used as
// the key namespace. Real PR branches routinely contain path separators
// ("feature/foo", "dependabot/..."), which the git store rejects in a
// single namespace segment. Percent-encoding collapses each branch to a
// single safe segment while keeping distinct branches distinct (so
// "feature/foo" and "feature-foo" do not collide).
func (r *Runner) persistToOrphanBranch(ctx context.Context, graph *corerpv20250801preview.ApplicationGraphResponse) error {
	branch := sourceBranch()
	if branch == "" {
		return clierrors.Message("Cannot determine source branch from GITHUB_HEAD_REF or GITHUB_REF_NAME; cannot persist modeled graph.")
	}

	if r.GraphStore == nil {
		return clierrors.Message("Modeled graph store is not configured.")
	}

	namespace := url.QueryEscape(branch)
	key := persistence.Key{Namespace: namespace, Name: modeledGraphKeyName}
	opts := persistence.SaveOptions{
		Message: fmt.Sprintf("radius: update modeled graph for %s", branch),
	}
	if err := r.GraphStore.Save(ctx, key, graph, opts); err != nil {
		return fmt.Errorf("commit modeled graph to %s branch: %w", gitstore.DefaultGraphBranch, err)
	}

	r.Output.LogInfo("Parsed %d resources. Committed %s/%s.json to branch %s", len(graph.Resources), namespace, modeledGraphKeyName, gitstore.DefaultGraphBranch)
	return nil
}
