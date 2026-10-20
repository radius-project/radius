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
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	cligraph "github.com/radius-project/radius/pkg/cli/graph"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	archivestore "github.com/radius-project/radius/pkg/graph/persistence/archive"
	"github.com/radius-project/radius/pkg/to"
	"github.com/spf13/cobra"
)

const (
	bicepExtension          = ".bicep"
	defaultModeledGraphFile = "app-graph.json"
	modeledGraphKeyName     = "app-graph"

	// envGitHubActions is set to "true" by GitHub Actions for every step
	// running inside a runner. When present, rad operates in repo-radius
	// mode and saves graph artifacts to the radius-graph archive.
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

When invoked with the name of a deployed application, either as an argument or
with the --application flag, the command queries the Radius control plane and
prints the deployed graph computed from stored resources (Kind: Connection
edges only).

When invoked with --application <name> AND a path to the application's
app.bicep, the command additionally compiles the template locally, extracts the
dependsOn edges implied by it, and sends them to the server. The server merges
them onto the deployed graph as Kind: Dependency edges. Any edge already present
as Kind: Connection wins; excluded types and unknown endpoints are dropped.

When invoked with only a path to an app.bicep (no --application), the command
compiles the template and writes the resulting modeled graph to
./app-graph.json without contacting the control plane.

If the modeled graph is built inside a GitHub Actions runner
(GITHUB_ACTIONS=true), it is saved to <source-branch>/app-graph.json in the
radius-graph OCI archive instead of the local filesystem. Configure
RADIUS_GRAPH_REGISTRY with an OCI repository and authenticate to the registry.
Local output does not require a registry.`,
		Args: cobra.MaximumNArgs(1),
		Example: `
# Show graph for the deployed application named my-application
rad app graph my-application

# Include icon SVG bytes inline in the JSON output
rad app graph my-application -o json --include-icons

# Enrich the deployed graph with dependsOn edges from a local app.bicep
rad app graph -a my-application ./app.bicep

# Build the modeled graph for an app.bicep and write it to ./app-graph.json
rad app graph ./app.bicep`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	cmd.Flags().Bool("include-icons", false, "When set, embeds each referenced resource type icon's SVG bytes in the response's icons map.")

	return cmd, runner
}

// Runner is the runner implementation for the `rad app graph` command.
//
// The command has three modes, selected in Validate:
//   - deployed: ApplicationName set, BicepFilePath empty.
//   - enriched: ApplicationName and BicepFilePath both set.
//   - modeled:  BicepFilePath set, ApplicationName empty.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	Bicep                   bicep.Interface
	Workspace               *workspaces.Workspace
	RadiusCoreClientFactory *corerpv20250801preview.ClientFactory

	ApplicationName string
	BicepFilePath   string

	// IncludeIcons, when true, causes the graph response to carry a deduped
	// `icons` map of hash → SVG bytes. When false the response has only
	// per-node `iconHash` values and consumers content-address the bytes
	// themselves.
	IncludeIcons bool

	Format string

	// GraphStore persists modeled graphs to the radius-graph archive
	// when running in repo-radius mode. Defaulted in NewRunner; tests may
	// substitute a mock implementation.
	GraphStore persistence.Store
}

// NewRunner creates a new instance of the `rad app graph` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Bicep:        factory.GetBicep(),
		GraphStore:   factory.GetGraphStore(),
	}
}

// isModeledGraphArg returns true when the positional argument is a path to a
// Bicep file. Application names never carry a .bicep extension.
func isModeledGraphArg(arg string) bool {
	if arg == "" {
		return false
	}
	return strings.EqualFold(filepath.Ext(arg), bicepExtension)
}

// inRepoRadiusMode returns true when running inside a GitHub Actions runner.
// Per the design, this is the trigger for saving graph artifacts to the
// radius-graph archive instead of writing them to the local filesystem.
func inRepoRadiusMode() bool {
	return strings.EqualFold(os.Getenv(envGitHubActions), "true")
}

// sourceBranch returns the source branch name to use as the namespace under
// which the modeled graph is saved in the archive. For
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

	r.IncludeIcons, err = cmd.Flags().GetBool("include-icons")
	if err != nil {
		return err
	}

	// A single positional .bicep argument claims the positional slot, so
	// the application name can only come from -a/--application. Without
	// it, the command builds the modeled graph offline; with it, the
	// command enriches the deployed graph.
	remainingArgs := args
	if len(args) == 1 && isModeledGraphArg(args[0]) {
		r.BicepFilePath = args[0]
		remainingArgs = nil

		// Only an absent flag selects modeled mode. An explicitly empty
		// value (for example --application "$APP" with APP unset) falls
		// through so RequireApplicationArgs rejects it.
		if !cmd.Flags().Changed("application") {
			return nil
		}
	}

	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, remainingArgs, *r.Workspace)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad app graph` command.
func (r *Runner) Run(ctx context.Context) error {
	if r.BicepFilePath != "" && r.ApplicationName == "" {
		return r.runModeled(ctx)
	}
	return r.runDeployed(ctx)
}

// runDeployed fetches the deployed application graph from the Radius control
// plane. When BicepFilePath is set, it first compiles the template and sends
// its dependsOn edges so the server can merge them into the deployed graph.
func (r *Runner) runDeployed(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = factory
	}

	appClient := r.RadiusCoreClientFactory.NewApplicationsClient()

	body := corerpv20250801preview.GetGraphRequest{}
	if r.IncludeIcons {
		body.IncludeIcons = to.Ptr(true)
	}
	if r.BicepFilePath != "" {
		displayPath := bicep.RedactTemplatePath(r.BicepFilePath)
		r.Output.LogInfo("Compiling %s", displayPath)
		template, _, err := r.Bicep.PrepareTemplate(ctx, r.BicepFilePath)
		if err != nil {
			return clierrors.Message("Failed to compile %q: %v", displayPath, err)
		}
		// ExtractDependsOnEdges returns nil when the template has no
		// eligible dependsOn edges, leaving body.DependsOnEdges nil so
		// the server sees the field as absent rather than an empty map.
		//
		// Pass the workspace scope so the extracted source and target
		// IDs match the deployed-graph IDs the server will merge them
		// against.
		body.DependsOnEdges = cligraph.ExtractDependsOnEdges(template, r.Workspace.Scope)
	}
	graphResponse, err := appClient.GetGraph(ctx, r.Workspace.Scope, r.ApplicationName, body, &corerpv20250801preview.ApplicationsClientGetGraphOptions{})
	if clients.Is404Error(err) {
		return clierrors.Message("Application %q does not exist or has been deleted.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	switch r.Format {
	case output.FormatJson:
		return r.Output.WriteFormatted(r.Format, graphResponse.ApplicationGraphResponse, output.FormatterOptions{})
	default:
		r.Output.LogInfo(display(graphResponse.Resources, r.ApplicationName))
		return nil
	}
}

// runModeled compiles the supplied Bicep file, builds the modeled
// application graph, and persists the result. When running inside a
// GitHub Actions runner the graph is committed to the radius-graph archive
// under <source-branch>/app-graph.json; otherwise it is written to
// ./app-graph.json in the current working directory.
func (r *Runner) runModeled(ctx context.Context) error {
	displayPath := bicep.RedactTemplatePath(r.BicepFilePath)
	r.Output.LogInfo("Compiling %s", displayPath)
	template, _, err := r.Bicep.PrepareTemplate(ctx, r.BicepFilePath)
	if err != nil {
		return clierrors.Message("Failed to compile %q: %v", displayPath, err)
	}

	graph, err := cligraph.BuildModeledGraph(template, r.IncludeIcons)
	if err != nil {
		return clierrors.Message("Failed to build modeled graph: %v", err)
	}

	if inRepoRadiusMode() {
		return r.persistToArchive(ctx, graph)
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

// persistToArchive commits graph to <encoded-source-branch>/app-graph.json
// in the radius-graph archive.
//
// The raw branch name is encoded with url.QueryEscape before being used as
// the key namespace. Real PR branches routinely contain path separators
// ("feature/foo", "dependabot/..."), which the archive store rejects in a
// single namespace segment. Percent-encoding collapses each branch to a
// single safe segment while keeping distinct branches distinct (so
// "feature/foo" and "feature-foo" do not collide).
func (r *Runner) persistToArchive(ctx context.Context, graph *corerpv20250801preview.ApplicationGraphResponse) error {
	branch := sourceBranch()
	if branch == "" {
		return clierrors.Message("Cannot determine source branch from GITHUB_HEAD_REF or GITHUB_REF_NAME; cannot save modeled graph.")
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
		return fmt.Errorf("save modeled graph to %s archive: %w", archivestore.DefaultGraphArchive, err)
	}

	r.Output.LogInfo("Parsed %d resources. Saved %s/%s.json to archive %s", len(graph.Resources), namespace, modeledGraphKeyName, archivestore.DefaultGraphArchive)
	return nil
}
