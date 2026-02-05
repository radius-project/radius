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

package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/recipes"
	"github.com/radius-project/radius/pkg/discovery/skills"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
)

const (
	flagDiscoveryPath   = "discovery"
	flagAppName         = "app-name"
	flagEnvironment     = "environment"
	flagOutputPath      = "output"
	flagIncludeComments = "comments"
	flagIncludeRecipes  = "recipes"
	flagGenerateRecipes = "generate-recipes"
	flagRecipeProfile   = "recipe-profile"
	flagAddDependency   = "add-dependency"
	flagUpdate          = "update"
	flagConflictMode    = "on-conflict"
	flagVerbose         = "verbose"
	flagDryRun          = "dry-run"
	flagValidate        = "validate"
	flagCloudProvider   = "cloud-provider"
)

// NewCommand creates an instance of the `rad app generate` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Radius application definition from discovery results",
		Long: `Generates a Radius application definition (app.bicep) from discovery results.

This command reads the discovery.md file created by 'rad app discover' and
generates a complete Bicep file defining your Radius application, including:
- Application resource
- Container resources for each discovered service
- Infrastructure resources for detected dependencies (databases, caches, etc.)

The generated Bicep file can be customized and then deployed using 'rad deploy'.`,
		Example: `
# Generate app.bicep from discovery results
rad app generate

# Generate with custom application name
rad app generate --app-name myapp

# Generate with verbose output
rad app generate --verbose

# Dry run (show what would be generated without writing files)
rad app generate --dry-run

# Include comments in generated Bicep
rad app generate --comments

# Include recipe references for infrastructure resources
rad app generate --recipes

# Generate environment configuration files (env.bicep)
rad app generate --recipes --generate-recipes

# Generate and validate the output
rad app generate --validate

# Specify custom input/output paths
rad app generate --discovery ./custom/discovery.json --output ./deploy/app.bicep
`,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP(flagDiscoveryPath, "d", "", "Path to discovery.json (default: ./radius/discovery.json)")
	cmd.Flags().String(flagAppName, "", "Application name (default: project directory name)")
	cmd.Flags().StringP(flagEnvironment, "e", "", "Target Radius environment")
	cmd.Flags().StringP(flagOutputPath, "o", "", "Output path for app.bicep (default: ./radius/app.bicep)")
	cmd.Flags().Bool(flagIncludeComments, true, "Include helpful comments in generated Bicep")
	cmd.Flags().Bool(flagIncludeRecipes, true, "Discover and include recipe references for infrastructure resources")
	cmd.Flags().Bool(flagGenerateRecipes, true, "Generate env.bicep file to register discovered recipes with the environment")
	cmd.Flags().String(flagRecipeProfile, "", "Recipe profile for environment-specific recipe sets (e.g., dev, staging, prod)")
	cmd.Flags().StringArrayP(flagAddDependency, "a", nil, "Add manual infrastructure dependency (can be specified multiple times, e.g., --add-dependency postgres)")
	cmd.Flags().Bool(flagUpdate, false, "Update existing app.bicep using diff/patch mode (preserves manual changes)")
	cmd.Flags().String(flagConflictMode, "ask", "Conflict handling mode when app.bicep exists: ask, overwrite, merge, diff, skip")
	cmd.Flags().BoolP(flagVerbose, "v", false, "Enable verbose output")
	cmd.Flags().Bool(flagDryRun, false, "Show generated Bicep without writing files")
	cmd.Flags().Bool(flagValidate, false, "Validate generated Bicep after generation")
	cmd.Flags().String(flagCloudProvider, "", "Cloud provider for recipe generation (azure, aws, kubernetes). Auto-detected if not specified.")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad app generate` command.
type Runner struct {
	Output output.Interface

	DiscoveryPath   string
	AppName         string
	Environment     string
	OutputPath      string
	IncludeComments bool
	IncludeRecipes  bool
	GenerateRecipes bool
	RecipeProfile   string
	AddDependencies []string
	UpdateMode      bool
	ConflictMode    string
	Verbose         bool
	DryRun          bool
	ValidateBicep   bool
	CloudProvider   string
}

// NewRunner creates an instance of the runner for the `rad app generate` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
	}
}

// Validate implements the framework.Runner interface.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Get flags
	discoveryPath, _ := cmd.Flags().GetString(flagDiscoveryPath)
	appName, _ := cmd.Flags().GetString(flagAppName)
	environment, _ := cmd.Flags().GetString(flagEnvironment)
	outputPath, _ := cmd.Flags().GetString(flagOutputPath)
	includeComments, _ := cmd.Flags().GetBool(flagIncludeComments)
	includeRecipes, _ := cmd.Flags().GetBool(flagIncludeRecipes)
	generateRecipes, _ := cmd.Flags().GetBool(flagGenerateRecipes)
	recipeProfile, _ := cmd.Flags().GetString(flagRecipeProfile)
	addDependencies, _ := cmd.Flags().GetStringArray(flagAddDependency)
	updateMode, _ := cmd.Flags().GetBool(flagUpdate)
	conflictMode, _ := cmd.Flags().GetString(flagConflictMode)
	verbose, _ := cmd.Flags().GetBool(flagVerbose)
	dryRun, _ := cmd.Flags().GetBool(flagDryRun)
	validate, _ := cmd.Flags().GetBool(flagValidate)
	cloudProvider, _ := cmd.Flags().GetString(flagCloudProvider)

	// If --generate-recipes is set, implicitly enable --recipes
	if generateRecipes {
		includeRecipes = true
	}

	// Get project path from args or use current directory
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	// Resolve project path to absolute
	var err error
	projectPath, err = filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolving project path: %w", err)
	}

	// Set defaults relative to project path
	if discoveryPath == "" {
		discoveryPath = filepath.Join(projectPath, "radius", "discovery.json")
	}
	if outputPath == "" {
		outputPath = filepath.Join(projectPath, "radius", "app.bicep")
	}

	// Resolve paths to absolute (for user-provided paths)
	discoveryPath, err = filepath.Abs(discoveryPath)
	if err != nil {
		return fmt.Errorf("resolving discovery path: %w", err)
	}

	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	// If output path is a directory, append default filename
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		outputPath = filepath.Join(outputPath, "app.bicep")
	}

	// If discovery path is a directory, append default filename
	if info, err := os.Stat(discoveryPath); err == nil && info.IsDir() {
		discoveryPath = filepath.Join(discoveryPath, "discovery.json")
	}

	// Validate discovery file exists
	if _, err := os.Stat(discoveryPath); os.IsNotExist(err) {
		return fmt.Errorf("discovery file not found: %s\n\nRun 'rad app discover' first to analyze your codebase", discoveryPath)
	}

	r.DiscoveryPath = discoveryPath
	r.AppName = appName
	r.Environment = environment
	r.OutputPath = outputPath
	r.IncludeComments = includeComments
	r.IncludeRecipes = includeRecipes
	r.GenerateRecipes = generateRecipes
	r.RecipeProfile = recipeProfile
	r.AddDependencies = addDependencies
	r.UpdateMode = updateMode
	r.ConflictMode = conflictMode
	r.Verbose = verbose
	r.DryRun = dryRun
	r.ValidateBicep = validate
	r.CloudProvider = cloudProvider

	return nil
}

// Run implements the framework.Runner interface.
func (r *Runner) Run(ctx context.Context) error {
	// Print header
	r.Output.LogInfo("")
	r.Output.LogInfo("============================================================================")
	r.Output.LogInfo("                    Radius Application Generator")
	r.Output.LogInfo("============================================================================")
	r.Output.LogInfo("")

	// Load discovery results
	r.Output.LogInfo("Loading discovery results from: %s", r.DiscoveryPath)

	discoveryResult, err := r.loadDiscoveryResult()
	if err != nil {
		return fmt.Errorf("loading discovery results: %w", err)
	}

	if r.Verbose {
		r.Output.LogInfo("  Found %d services", len(discoveryResult.Services))
		r.Output.LogInfo("  Found %d dependencies", len(discoveryResult.Dependencies))
	}

	outputDir := filepath.Dir(r.OutputPath)

	// PHASE 1: Generate Resource Types (types/<namespace>.yaml)
	r.Output.LogInfo("")
	r.Output.LogInfo("── Phase 1: Generate Resource Types ───────────────────────────────────────")

	// Generate types.yaml files separated by namespace
	typesDir := filepath.Join(outputDir, "types")
	if err := r.generateTypesYAML(discoveryResult, outputDir); err != nil {
		r.Output.LogInfo("  ⚠ Failed to generate types: %v", err)
	} else {
		r.Output.LogInfo("  ✓ Generated: %s/", typesDir)
		r.Output.LogInfo("  Resource types: %d", len(discoveryResult.ResourceTypes))
	}
	r.displayResourceTypes(discoveryResult)

	// Generate Bicep extensions from types
	extensionsDir := filepath.Join(outputDir, "extensions")
	if err := r.generateBicepExtensions(discoveryResult, outputDir); err != nil {
		r.Output.LogInfo("  ⚠ Failed to generate Bicep extensions: %v", err)
	} else if len(discoveryResult.ResourceTypes) > 0 {
		r.Output.LogInfo("  ✓ Generated Bicep extensions: %s/", extensionsDir)
	}

	// Generate bicepconfig.json
	if err := r.generateBicepConfig(discoveryResult, outputDir); err != nil {
		r.Output.LogInfo("  ⚠ Failed to generate bicepconfig.json: %v", err)
	} else {
		r.Output.LogInfo("  ✓ Generated: %s", filepath.Join(outputDir, "bicepconfig.json"))
	}

	// PHASE 2: Generate Recipes and Environment
	r.Output.LogInfo("")
	r.Output.LogInfo("── Phase 2: Generate Recipes and Environment ────────────────────────────────")
	r.displayRecipes(discoveryResult)

	// Generate recipe modules in recipes/ folder
	recipesDir := filepath.Join(outputDir, "recipes")
	if len(discoveryResult.Recipes) > 0 || len(discoveryResult.ResourceTypes) > 0 {
		if err := r.generateRecipeModules(discoveryResult, recipesDir); err != nil {
			r.Output.LogInfo("  ⚠ Failed to generate recipes: %v", err)
		} else {
			r.Output.LogInfo("  ✓ Generated recipes: %s/", recipesDir)
		}
	}

	envPath := filepath.Join(outputDir, "env.bicep")
	if len(discoveryResult.Recipes) > 0 || len(discoveryResult.ResourceTypes) > 0 {
		if err := r.generateEnvBicep(discoveryResult, envPath); err != nil {
			r.Output.LogInfo("  ⚠ Failed to generate env.bicep: %v", err)
		} else {
			r.Output.LogInfo("  ✓ Generated: %s", envPath)
		}
	} else {
		r.Output.LogInfo("  ℹ No recipes to generate (use --recipes to discover recipes)")
	}

	// PHASE 3: Generate app.bicep
	r.Output.LogInfo("")
	r.Output.LogInfo("── Phase 3: Generate app.bicep ─────────────────────────────────────────────")

	// Create generate skill
	generateSkill, err := skills.NewGenerateAppDefinitionSkill()
	if err != nil {
		return fmt.Errorf("creating generate skill: %w", err)
	}

	// Prepare input
	input := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: discoveryResult,
		ApplicationName: r.AppName,
		Environment:     r.Environment,
		IncludeComments: r.IncludeComments,
		IncludeRecipes:  r.IncludeRecipes,
	}

	if !r.DryRun {
		input.OutputPath = r.OutputPath
	}

	// Generate application definition
	generateOutput, err := generateSkill.Execute(input)
	if err != nil {
		return fmt.Errorf("generating application definition: %w", err)
	}

	// Display summary
	r.displayGenerationSummary(generateOutput)

	// Handle dry run
	if r.DryRun {
		r.Output.LogInfo("")
		r.Output.LogInfo("Generated Bicep content (dry run):")
		r.Output.LogInfo("============================================================================")
		r.Output.LogInfo(generateOutput.BicepContent)
		r.Output.LogInfo("============================================================================")
		return nil
	}

	// Validate if requested
	if r.ValidateBicep {
		r.Output.LogInfo("")
		r.Output.LogInfo("Validating generated Bicep...")

		validateSkill := skills.NewValidateAppDefinitionSkill()
		validateInput := &skills.ValidateAppDefinitionInput{
			FilePath:        r.OutputPath,
			DiscoveryResult: discoveryResult,
			StrictMode:      true,
		}

		validateOutput, err := validateSkill.Execute(validateInput)
		if err != nil {
			return fmt.Errorf("validating generated Bicep: %w", err)
		}

		r.displayValidationResults(validateOutput)

		if !validateOutput.Valid {
			return fmt.Errorf("generated Bicep has validation errors")
		}
	}

	// Success message
	r.Output.LogInfo("")
	r.Output.LogInfo("============================================================================")
	r.Output.LogInfo("Application definition generated successfully!")
	r.Output.LogInfo("============================================================================")
	r.Output.LogInfo("")
	r.Output.LogInfo("Output:")
	r.Output.LogInfo("  • types/:           %s", filepath.Join(outputDir, "types"))
	r.Output.LogInfo("  • extensions/:      %s", filepath.Join(outputDir, "extensions"))
	r.Output.LogInfo("  • bicepconfig.json: %s", filepath.Join(outputDir, "bicepconfig.json"))
	if len(discoveryResult.ResourceTypes) > 0 {
		r.Output.LogInfo("  • recipes/:         %s", filepath.Join(outputDir, "recipes"))
		r.Output.LogInfo("  • env.bicep:        %s", filepath.Join(outputDir, "env.bicep"))
	}
	r.Output.LogInfo("  • app.bicep:        %s", r.OutputPath)
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. Review and customize the generated files")
	r.Output.LogInfo("  2. Update container image references in app.bicep")
	if len(discoveryResult.ResourceTypes) > 0 {
		r.Output.LogInfo("  3. Deploy environment first: rad deploy %s", filepath.Join(outputDir, "env.bicep"))
		r.Output.LogInfo("  4. Deploy application: rad deploy %s", r.OutputPath)
	} else {
		r.Output.LogInfo("  3. Deploy with: rad deploy %s", r.OutputPath)
	}
	r.Output.LogInfo("")

	return nil
}

func (r *Runner) loadDiscoveryResult() (*discovery.DiscoveryResult, error) {
	data, err := os.ReadFile(r.DiscoveryPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var result discovery.DiscoveryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing discovery results: %w", err)
	}

	return &result, nil
}

func (r *Runner) displayResourceTypes(result *discovery.DiscoveryResult) {
	if len(result.ResourceTypes) == 0 && len(result.Dependencies) == 0 {
		r.Output.LogInfo("  No resource types to generate.")
		return
	}

	r.Output.LogInfo("  Mapping dependencies to Radius Resource Types:")
	r.Output.LogInfo("  (Type definitions from resource-types-contrib)")
	r.Output.LogInfo("")

	// Show mapped resource types
	processedDeps := make(map[string]bool)
	for _, rt := range result.ResourceTypes {
		r.Output.LogInfo("  ✓ %s → %s", rt.DependencyID, rt.ResourceType.Name)
		processedDeps[rt.DependencyID] = true
	}

	// Show unmapped dependencies
	for _, dep := range result.Dependencies {
		if !processedDeps[dep.ID] {
			r.Output.LogInfo("  ⚠ %s → (no mapping for type: %s)", dep.ID, dep.Type)
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("  Total: %d resource types mapped", len(result.ResourceTypes))
}

func (r *Runner) displayRecipes(result *discovery.DiscoveryResult) {
	if !r.IncludeRecipes {
		r.Output.LogInfo("  Recipe discovery skipped (use --recipes to enable)")
		return
	}

	r.Output.LogInfo("  Discovering recipes from configured sources:")
	r.Output.LogInfo("  (Priority: 1. Local → 2. resource-types-contrib → 3. Azure Verified Modules)")
	r.Output.LogInfo("")

	if len(result.Recipes) == 0 {
		// Try to discover recipes now
		recipeMatches, err := r.discoverRecipes(result)
		if err != nil {
			r.Output.LogInfo("  ⚠ Error discovering recipes: %v", err)
			return
		}

		if len(recipeMatches) == 0 {
			r.Output.LogInfo("  No matching recipes found.")
			r.Output.LogInfo("  Tip: Configure recipe sources with 'rad recipe source add'")
			return
		}

		// Update result with discovered recipes
		result.Recipes = recipeMatches
	}

	// Detect cloud provider
	cloudProvider := r.getCloudProvider(result)
	r.Output.LogInfo("  Cloud provider detected: %s", cloudProvider)
	r.Output.LogInfo("")

	// Build a map of dependency IDs to their resource types
	depToResourceType := make(map[string]string)
	for _, rtMatch := range result.ResourceTypes {
		depToResourceType[rtMatch.DependencyID] = rtMatch.ResourceType.Name
	}

	// Filter to show only the highest-priority recipe per resource type
	bestRecipesByType := make(map[string]discovery.RecipeMatch)
	for _, recipe := range result.Recipes {
		resourceType := depToResourceType[recipe.DependencyID]
		if resourceType == "" {
			continue
		}
		existing, exists := bestRecipesByType[resourceType]
		if !exists || isHigherPriorityRecipe(recipe, existing) {
			bestRecipesByType[resourceType] = recipe
		}
	}

	// Display recipes that will be generated for each resource type
	for _, rtMatch := range result.ResourceTypes {
		resourceType := rtMatch.ResourceType.Name
		shortName := resourceType[strings.LastIndex(resourceType, "/")+1:]

		if recipe, hasRecipe := bestRecipesByType[resourceType]; hasRecipe {
			sourceType := string(recipe.Recipe.SourceType)
			if sourceType == "" {
				sourceType = "unknown"
			}
			r.Output.LogInfo("  ✓ %s → recipes/%s/ [%s]", shortName, shortName, sourceType)
		} else {
			// No matched recipe - will use AVM or default template
			if cloudProvider == "azure" {
				r.Output.LogInfo("  ✓ %s → recipes/%s/ [avm]", shortName, shortName)
			} else {
				r.Output.LogInfo("  ✓ %s → recipes/%s/ [%s]", shortName, shortName, cloudProvider)
			}
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("  Total: %d recipes to generate", len(result.ResourceTypes))
}

func (r *Runner) discoverRecipes(result *discovery.DiscoveryResult) ([]discovery.RecipeMatch, error) {
	// Create recipe registry with sources in priority order:
	// 1. Local/Internal recipes (highest priority)
	// 2. resource-types-contrib
	// 3. AVM (Azure Verified Modules) - fallback
	registry := recipes.NewRegistry()

	// 1. Add local-terraform source first (highest priority)
	// This discovers recipes from existing TF modules in the project
	localTFSource, err := recipes.NewLocalTerraformSource(recipes.SourceConfig{
		Name: "local-terraform",
		URL:  result.ProjectPath,
	})
	if err == nil {
		_ = registry.Register(localTFSource)
	}

	// 2. Add resource-types-contrib source (kubernetes provider by default)
	// This is the community-maintained repository of recipes
	contribSourceK8s, err := recipes.NewContribSource(recipes.ContribSourceConfig{
		Name:     "resource-types-contrib-kubernetes",
		Provider: "kubernetes",
	})
	if err == nil {
		_ = registry.Register(contribSourceK8s)
	}

	// Also add Azure provider from contrib if available
	contribSourceAzure, err := recipes.NewContribSource(recipes.ContribSourceConfig{
		Name:     "resource-types-contrib-azure",
		Provider: "azure",
	})
	if err == nil {
		_ = registry.Register(contribSourceAzure)
	}

	// Also add AWS provider from contrib if available
	contribSourceAWS, err := recipes.NewContribSource(recipes.ContribSourceConfig{
		Name:     "resource-types-contrib-aws",
		Provider: "aws",
	})
	if err == nil {
		_ = registry.Register(contribSourceAWS)
	}

	// 3. Add AVM source as fallback (Azure only)
	avmSource, err := recipes.NewAVMSource(recipes.SourceConfig{
		Name: "azure-verified-modules",
		URL:  "",
	})
	if err == nil {
		_ = registry.Register(avmSource)
	}

	// Create matcher with priority order
	// PreferredSources defines the priority: local → contrib → avm
	matcher := recipes.NewMatcher(registry, recipes.MatcherOptions{
		MinConfidence: 0.3,
		MaxMatches:    3,
		PreferredSources: []string{
			"local-terraform",
			"resource-types-contrib-kubernetes",
			"resource-types-contrib-azure",
			"resource-types-contrib-aws",
			"azure-verified-modules",
		},
		CloudProvider: r.getCloudProvider(result),
	})

	// Match recipes for each resource type
	return matcher.Match(context.Background(), result.ResourceTypes)
}

// getCloudProvider attempts to detect the cloud provider from the discovery result.
// If the --cloud-provider flag was specified, that value takes precedence.
func (r *Runner) getCloudProvider(result *discovery.DiscoveryResult) string {
	// If cloud provider was explicitly specified via flag, use it
	if r.CloudProvider != "" {
		return r.CloudProvider
	}

	// Check terraform providers from extracted infrastructure
	for _, source := range result.Practices.ExtractedFrom {
		if source.Type == "terraform" {
			for _, provider := range source.Providers {
				providerLower := strings.ToLower(provider)
				if providerLower == "azurerm" || strings.Contains(providerLower, "azure") {
					return "azure"
				}
				if providerLower == "aws" || strings.Contains(providerLower, "amazon") {
					return "aws"
				}
				if providerLower == "google" || strings.Contains(providerLower, "gcp") {
					return "gcp"
				}
			}
		}
	}

	// Check for cloud-specific dependencies or services
	for _, dep := range result.Dependencies {
		depType := strings.ToLower(string(dep.Type))
		depName := strings.ToLower(dep.Name)
		depLibrary := strings.ToLower(dep.Library)

		// Azure indicators
		if strings.Contains(depType, "azure") || strings.Contains(depName, "azure") ||
			strings.Contains(depLibrary, "@azure/") ||
			strings.Contains(depType, "cosmosdb") || strings.Contains(depType, "servicebus") {
			return "azure"
		}

		// AWS indicators
		if strings.Contains(depType, "aws") || strings.Contains(depName, "aws") ||
			strings.Contains(depLibrary, "aws-sdk") || strings.Contains(depLibrary, "@aws-sdk/") ||
			strings.Contains(depType, "dynamodb") || strings.Contains(depType, "sqs") ||
			strings.Contains(depType, "sns") || strings.Contains(depType, "s3") {
			return "aws"
		}

		// GCP indicators
		if strings.Contains(depType, "gcp") || strings.Contains(depName, "gcp") ||
			strings.Contains(depLibrary, "@google-cloud/") ||
			strings.Contains(depType, "firestore") || strings.Contains(depType, "pubsub") {
			return "gcp"
		}
	}

	// Also check service evidence for cloud SDK usage
	for _, svc := range result.Services {
		for _, ev := range svc.Evidence {
			snippet := strings.ToLower(ev.Snippet)
			// Azure SDKs
			if strings.Contains(snippet, "@azure/") || strings.Contains(snippet, "azure-") {
				return "azure"
			}
			// AWS SDKs
			if strings.Contains(snippet, "@aws-sdk/") || strings.Contains(snippet, "aws-sdk") ||
				strings.Contains(snippet, "boto3") {
				return "aws"
			}
			// GCP SDKs
			if strings.Contains(snippet, "@google-cloud/") || strings.Contains(snippet, "google-cloud-") {
				return "gcp"
			}
		}
	}

	// Default to kubernetes if no cloud provider detected
	return "kubernetes"
}

func (r *Runner) displayGenerationSummary(output *skills.GenerateAppDefinitionOutput) {
	r.Output.LogInfo("")
	r.Output.LogInfo("  Generated resources: %d", output.ResourceCount)

	if len(output.Warnings) > 0 {
		r.Output.LogInfo("")
		r.Output.LogInfo("  Warnings:")
		for _, warning := range output.Warnings {
			r.Output.LogInfo("    ⚠ %s", warning)
		}
	}
}

func (r *Runner) displaySummary(output *skills.GenerateAppDefinitionOutput) {
	r.Output.LogInfo("")
	r.Output.LogInfo("Generation Summary:")
	r.Output.LogInfo("  Resources generated: %d", output.ResourceCount)

	if len(output.Warnings) > 0 {
		r.Output.LogInfo("")
		r.Output.LogInfo("Warnings:")
		for _, warning := range output.Warnings {
			r.Output.LogInfo("  ⚠ %s", warning)
		}
	}
}

func (r *Runner) displayValidationResults(output *skills.ValidateAppDefinitionOutput) {
	if output.Valid {
		r.Output.LogInfo("  ✓ Validation passed")
	} else {
		r.Output.LogInfo("  ✗ Validation failed")
	}

	if len(output.Issues) > 0 {
		r.Output.LogInfo("")
		r.Output.LogInfo("Validation Issues:")
		for _, issue := range output.Issues {
			var icon string
			switch issue.Severity {
			case "error":
				icon = "✗"
			case "warning":
				icon = "⚠"
			default:
				icon = "ℹ"
			}
			r.Output.LogInfo("  %s [%s] %s", icon, issue.Severity, issue.Message)
		}
	}

	if r.Verbose && output.BicepCompileOutput != "" {
		r.Output.LogInfo("")
		r.Output.LogInfo("Bicep Compiler Output:")
		r.Output.LogInfo("%s", output.BicepCompileOutput)
	}
}

// generateTypesYAML generates types.yaml files separated by namespace
// Flow: 1. Check if type exists in resource-types-contrib → fetch from there
//  2. If not in contrib → generate dynamically from AVM
func (r *Runner) generateTypesYAML(result *discovery.DiscoveryResult, outputDir string) error {
	if len(result.ResourceTypes) == 0 {
		return nil
	}

	// Create types directory
	typesDir := filepath.Join(outputDir, "types")
	if err := os.MkdirAll(typesDir, 0755); err != nil {
		return fmt.Errorf("creating types directory: %w", err)
	}

	// Group resource types by namespace
	typesByNamespace := make(map[string][]dtypes.ResourceTypeMapping)
	for _, rtMatch := range result.ResourceTypes {
		parts := strings.Split(rtMatch.ResourceType.Name, "/")
		if len(parts) == 2 {
			namespace := parts[0]
			typesByNamespace[namespace] = append(typesByNamespace[namespace], rtMatch)
		}
	}

	// Generate separate file for each namespace
	for namespace, types := range typesByNamespace {
		var yaml strings.Builder

		// Header comment
		yaml.WriteString("# Resource Type Definitions\n")
		yaml.WriteString("# Auto-generated by rad app generate\n")
		yaml.WriteString("#\n")
		yaml.WriteString(fmt.Sprintf("# Generated: %s\n\n", time.Now().Format(time.RFC3339)))

		yaml.WriteString(fmt.Sprintf("namespace: %s\n", namespace))
		yaml.WriteString("types:\n")

		for _, rtMatch := range types {
			rt := rtMatch.ResourceType
			parts := strings.Split(rt.Name, "/")
			if len(parts) != 2 {
				continue
			}
			typeName := parts[1]

			// Check if type exists in resource-types-contrib
			contribYAML, source := fetchTypeFromContribOrAVM(rt.Name, rt.APIVersion)

			yaml.WriteString(fmt.Sprintf("  # Source: %s\n", source))
			yaml.WriteString(fmt.Sprintf("  %s:\n", typeName))
			yaml.WriteString(contribYAML)
		}

		// Write namespace file
		filename := filepath.Join(typesDir, namespace+".yaml")
		if err := os.WriteFile(filename, []byte(yaml.String()), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
	}

	return nil
}

// contribTypes maps resource types to their paths in resource-types-contrib
// Only types that actually exist in https://github.com/radius-project/resource-types-contrib
var contribTypes = map[string]string{
	"Radius.Data/mySqlDatabases":      "Data/mySqlDatabases/mySqlDatabases.yaml",
	"Radius.Data/postgreSqlDatabases": "Data/postgreSqlDatabases/postgreSqlDatabases.yaml",
}

// fetchTypeFromContribOrAVM fetches type definition from contrib if available, otherwise generates from AVM
func fetchTypeFromContribOrAVM(resourceType, apiVersion string) (string, string) {
	// Check if type exists in resource-types-contrib
	if contribPath, exists := contribTypes[resourceType]; exists {
		// Try to fetch from contrib
		yamlContent, err := fetchFromContrib(contribPath, resourceType)
		if err == nil && yamlContent != "" {
			return yamlContent, "resource-types-contrib"
		}
	}

	// Type not in contrib - generate from AVM pattern
	return generateFromAVM(resourceType, apiVersion), "generated-from-avm"
}

// fetchFromContrib fetches the type definition from resource-types-contrib
func fetchFromContrib(contribPath, resourceType string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/radius-project/resource-types-contrib/main/%s", contribPath)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Extract just the type definition content (skip the type name line)
	content := string(body)
	lines := strings.Split(content, "\n")
	var result strings.Builder
	inTypes := false
	inTargetType := false

	parts := strings.Split(resourceType, "/")
	targetTypeName := ""
	if len(parts) == 2 {
		targetTypeName = parts[1]
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "types:") {
			inTypes = true
			continue
		}
		if inTypes && strings.HasPrefix(strings.TrimSpace(line), targetTypeName+":") {
			inTargetType = true
			continue // Skip the type name line, caller adds it
		}
		if inTargetType {
			// Check for end of type definition
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// If we hit a non-indented line that's not empty, we're done
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				break
			}
			// Check if this is a new type (less indented than the content)
			if len(line) >= 2 && line[0] == ' ' && line[1] != ' ' {
				break
			}
			result.WriteString(line + "\n")
		}
	}

	return result.String(), nil
}

// generateFromAVM generates a type definition based on AVM patterns
func generateFromAVM(resourceType, apiVersion string) string {
	var yaml strings.Builder

	typeInfo := getAVMTypeInfo(resourceType)

	yaml.WriteString("    description: |\n")
	yaml.WriteString(fmt.Sprintf("      %s\n", typeInfo.Description))
	yaml.WriteString("    apiVersions:\n")
	yaml.WriteString(fmt.Sprintf("      '%s':\n", apiVersion))
	yaml.WriteString("        schema:\n")
	yaml.WriteString("          type: object\n")
	yaml.WriteString("          properties:\n")
	yaml.WriteString("            environment:\n")
	yaml.WriteString("              type: string\n")
	yaml.WriteString("              description: \"(Required) The Radius EnvironmentID.\"\n")
	yaml.WriteString("            application:\n")
	yaml.WriteString("              type: string\n")
	yaml.WriteString("              description: \"(Optional) The Radius Application ID.\"\n")

	for _, prop := range typeInfo.Properties {
		yaml.WriteString(fmt.Sprintf("            %s:\n", prop.Name))
		yaml.WriteString(fmt.Sprintf("              type: %s\n", prop.Type))
		yaml.WriteString(fmt.Sprintf("              description: \"%s\"\n", prop.Description))
		if prop.ReadOnly {
			yaml.WriteString("              readOnly: true\n")
		}
	}

	yaml.WriteString("          required:\n")
	yaml.WriteString("            - environment\n")

	return yaml.String()
}

// ResourceTypeProperty defines a property for a resource type
type ResourceTypeProperty struct {
	Name        string
	Type        string
	Description string
	ReadOnly    bool
}

// ResourceTypeInfo contains metadata about a resource type
type ResourceTypeInfo struct {
	Description string
	Properties  []ResourceTypeProperty
}

// getAVMTypeInfo returns type info derived from AVM patterns for types not in contrib
func getAVMTypeInfo(resourceType string) ResourceTypeInfo {
	// These are generated based on AVM module patterns
	avmTypeInfo := map[string]ResourceTypeInfo{
		"Radius.Data/redisCaches": {
			Description: "Redis cache resource (generated from AVM pattern: avm/res/cache/redis)",
			Properties: []ResourceTypeProperty{
				{Name: "host", Type: "string", Description: "(Read-only) The host name.", ReadOnly: true},
				{Name: "port", Type: "integer", Description: "(Read-only) The port number.", ReadOnly: true},
				{Name: "password", Type: "string", Description: "(Read-only) The password.", ReadOnly: true},
			},
		},
		"Radius.Data/mongoDatabases": {
			Description: "MongoDB database resource (generated from AVM pattern: avm/res/document-db/database-account)",
			Properties: []ResourceTypeProperty{
				{Name: "database", Type: "string", Description: "(Optional) The database name."},
				{Name: "connectionString", Type: "string", Description: "(Read-only) The connection string.", ReadOnly: true},
				{Name: "host", Type: "string", Description: "(Read-only) The host name.", ReadOnly: true},
				{Name: "port", Type: "integer", Description: "(Read-only) The port number.", ReadOnly: true},
			},
		},
		"Radius.Messaging/rabbitMQQueues": {
			Description: "RabbitMQ queue resource (generated from AVM pattern)",
			Properties: []ResourceTypeProperty{
				{Name: "queue", Type: "string", Description: "(Optional) The queue name."},
				{Name: "host", Type: "string", Description: "(Read-only) The host name.", ReadOnly: true},
				{Name: "port", Type: "integer", Description: "(Read-only) The port number.", ReadOnly: true},
			},
		},
		"Radius.Network/loadBalancers": {
			Description: "Load balancer resource (generated from AVM pattern: avm/res/network/load-balancer)",
			Properties: []ResourceTypeProperty{
				{Name: "hostname", Type: "string", Description: "(Read-only) The hostname.", ReadOnly: true},
				{Name: "port", Type: "integer", Description: "(Read-only) The port.", ReadOnly: true},
				{Name: "scheme", Type: "string", Description: "(Read-only) The scheme (http/https).", ReadOnly: true},
			},
		},
	}

	if info, ok := avmTypeInfo[resourceType]; ok {
		return info
	}

	// Default for unknown types
	parts := strings.Split(resourceType, "/")
	typeName := resourceType
	if len(parts) == 2 {
		typeName = parts[1]
	}
	return ResourceTypeInfo{
		Description: fmt.Sprintf("%s resource (generated dynamically)", typeName),
		Properties:  []ResourceTypeProperty{},
	}
}

// generateEnvBicep generates a Bicep file that configures the environment with recipes
// The generated file should be deployed using: rad deploy env.bicep -p environment=<env-name>
func (r *Runner) generateEnvBicep(result *discovery.DiscoveryResult, outputPath string) error {
	var bicep strings.Builder

	// Detect preferred IaC language from local repo
	preferTerraform := r.detectPreferredIaC(result) == "terraform"

	// Detect cloud provider
	cloudProvider := r.getCloudProvider(result)
	isAzure := cloudProvider == "azure"

	// Header
	bicep.WriteString("// Auto-generated by rad app generate\n")
	bicep.WriteString("// Environment configuration with recipe registrations\n")
	bicep.WriteString("// Deploy with: rad deploy env.bicep -p environment=<environment-name>\n")
	if isAzure {
		bicep.WriteString("// Azure cloud provider detected - Azure scope parameters included\n")
	}
	bicep.WriteString("\n")

	// Extension import
	bicep.WriteString("extension radius\n\n")

	// Parameters
	bicep.WriteString("@description('The name of the Radius environment')\n")
	bicep.WriteString("param environment string\n\n")

	// Azure-specific parameters
	if isAzure {
		bicep.WriteString("@description('Azure subscription ID for recipe deployments')\n")
		bicep.WriteString("param azureSubscriptionId string\n\n")

		bicep.WriteString("@description('Azure resource group name for recipe deployments')\n")
		bicep.WriteString("param azureResourceGroup string\n\n")
	}

	// Collect all resource types from the discovery
	allResourceTypes := make(map[string]string) // resourceType -> dependencyID
	for _, rtMatch := range result.ResourceTypes {
		allResourceTypes[rtMatch.ResourceType.Name] = rtMatch.DependencyID
	}

	// Select the best recipe per resource type (priority based on local IaC preference)
	bestRecipeByType := make(map[string]dtypes.RecipeMatch)
	for _, recipe := range result.Recipes {
		rt := ""
		for _, rtMatch := range result.ResourceTypes {
			if rtMatch.DependencyID == recipe.DependencyID {
				rt = rtMatch.ResourceType.Name
				break
			}
		}
		if rt == "" {
			rt = "Applications.Core/extenders"
		}

		// Check if we already have a recipe for this type
		existing, exists := bestRecipeByType[rt]
		if !exists {
			bestRecipeByType[rt] = recipe
		} else {
			// Compare priority based on preferred IaC
			newPriority := getRecipeSourcePriorityWithPreference(string(recipe.Recipe.SourceType), recipe.Recipe.SourceLocation, preferTerraform)
			existingPriority := getRecipeSourcePriorityWithPreference(string(existing.Recipe.SourceType), existing.Recipe.SourceLocation, preferTerraform)
			if newPriority < existingPriority {
				bestRecipeByType[rt] = recipe
			}
		}
	}

	// Build the recipes object for the environment
	iacType := "bicep"
	if preferTerraform {
		iacType = "terraform"
	}
	bicep.WriteString("// Environment with recipe registrations\n")
	bicep.WriteString(fmt.Sprintf("// Preferred IaC: %s (detected from local infrastructure)\n", iacType))
	bicep.WriteString(fmt.Sprintf("// Cloud provider: %s\n", cloudProvider))
	bicep.WriteString("// Priority: local terraform > terraform recipes > bicep recipes\n")
	bicep.WriteString("resource env 'Applications.Core/environments@2023-10-01-preview' = {\n")
	bicep.WriteString("  name: environment\n")
	bicep.WriteString("  location: 'global'\n")
	bicep.WriteString("  properties: {\n")
	bicep.WriteString("    compute: {\n")
	bicep.WriteString("      kind: 'kubernetes'\n")
	bicep.WriteString("      namespace: environment\n")
	bicep.WriteString("    }\n")

	// Add Azure provider configuration if needed
	if isAzure {
		bicep.WriteString("    providers: {\n")
		bicep.WriteString("      azure: {\n")
		bicep.WriteString("        scope: '/subscriptions/${azureSubscriptionId}/resourceGroups/${azureResourceGroup}'\n")
		bicep.WriteString("      }\n")
		bicep.WriteString("    }\n")
	}

	bicep.WriteString("    recipes: {\n")

	// Generate recipes for ALL resource types (with placeholders for missing recipes)
	for resourceType := range allResourceTypes {
		_, hasRecipe := bestRecipeByType[resourceType]

		bicep.WriteString(fmt.Sprintf("      '%s': {\n", resourceType))

		// Get short name for the recipe folder
		parts := strings.Split(resourceType, "/")
		shortName := parts[len(parts)-1]

		// Always use the generated recipe in the recipes/ folder
		// Derive the git URL for the template path
		projectDir := filepath.Dir(filepath.Dir(outputPath))
		gitURL := r.detectGitRemoteURL(projectDir)
		var templatePath string
		if gitURL != "" {
			templatePath = fmt.Sprintf("git::%s//radius/recipes/%s", gitURL, shortName)
		} else {
			templatePath = fmt.Sprintf("git::https://github.com/<your-org>/<your-repo>.git//radius/recipes/%s", shortName)
		}

		if hasRecipe {
			bicep.WriteString(fmt.Sprintf("        // Recipe for %s (generated from discovery)\n", shortName))
		} else {
			bicep.WriteString(fmt.Sprintf("        // Recipe for %s (auto-generated template)\n", shortName))
		}
		bicep.WriteString("        // NOTE: Commit and push the recipes/ folder before deploying\n")
		bicep.WriteString("        default: {\n")
		bicep.WriteString("          templateKind: 'terraform'\n")
		bicep.WriteString(fmt.Sprintf("          templatePath: '%s'\n", templatePath))
		bicep.WriteString("        }\n")
		bicep.WriteString("      }\n")
	}

	bicep.WriteString("    }\n")
	bicep.WriteString("  }\n")
	bicep.WriteString("}\n")

	return os.WriteFile(outputPath, []byte(bicep.String()), 0644)
}

// generateRecipeModules generates Terraform recipe modules in the recipes/ folder.
// For each resource type, it creates a Radius-compatible Terraform module.
// Recipe source priority: 1. Local → 2. resource-types-contrib → 3. AVM (Azure Verified Modules)
func (r *Runner) generateRecipeModules(result *discovery.DiscoveryResult, recipesDir string) error {
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		return fmt.Errorf("creating recipes directory: %w", err)
	}

	// Build a map of dependency IDs to their resource types
	depToResourceType := make(map[string]string)
	for _, rtMatch := range result.ResourceTypes {
		depToResourceType[rtMatch.DependencyID] = rtMatch.ResourceType.Name
	}

	// Build a map of resource types to their best matched recipe source
	recipeSourceByType := make(map[string]string)
	for _, recipeMatch := range result.Recipes {
		resourceType := depToResourceType[recipeMatch.DependencyID]
		if resourceType == "" {
			continue
		}
		// If we don't have a source yet, or this one is higher priority, use it
		currentSource := recipeSourceByType[resourceType]
		newSource := string(recipeMatch.Recipe.SourceType)
		if currentSource == "" || isHigherPrioritySource(newSource, currentSource) {
			recipeSourceByType[resourceType] = newSource
		}
	}

	// Collect all resource types
	resourceTypes := make(map[string]bool)
	for _, rtMatch := range result.ResourceTypes {
		resourceTypes[rtMatch.ResourceType.Name] = true
	}

	// Detect cloud provider for recipe selection
	cloudProvider := r.getCloudProvider(result)

	// Generate a recipe module for each resource type
	for resourceType := range resourceTypes {
		// Get the short name from the resource type (e.g., "redisCaches" from "Radius.Data/redisCaches")
		parts := strings.Split(resourceType, "/")
		shortName := parts[len(parts)-1]

		// Create recipe directory
		recipeDir := filepath.Join(recipesDir, shortName)
		if err := os.MkdirAll(recipeDir, 0755); err != nil {
			return fmt.Errorf("creating recipe directory for %s: %w", shortName, err)
		}

		// Determine recipe source for this type
		source := recipeSourceByType[resourceType]

		// Generate the recipe module with source-aware template selection
		if err := r.generateRecipeModuleWithSource(resourceType, recipeDir, source, cloudProvider); err != nil {
			return fmt.Errorf("generating recipe for %s: %w", resourceType, err)
		}

		r.Output.LogInfo("    ✓ %s → recipes/%s/", resourceType, shortName)
	}

	return nil
}

// isHigherPrioritySource returns true if newSource has higher priority than currentSource.
// Priority: local > resource-types-contrib > avm > builtin
func isHigherPrioritySource(newSource, currentSource string) bool {
	priority := map[string]int{
		"local-terraform":                   1,
		"resource-types-contrib-kubernetes": 2,
		"resource-types-contrib-azure":      2,
		"resource-types-contrib-aws":        2,
		"azure-verified-modules":            3,
		"builtin":                           4,
	}
	newPriority, ok1 := priority[newSource]
	currentPriority, ok2 := priority[currentSource]
	if !ok1 {
		newPriority = 5
	}
	if !ok2 {
		currentPriority = 5
	}
	return newPriority < currentPriority
}

// isHigherPriorityRecipe returns true if newRecipe has higher priority than currentRecipe.
// Priority: local-terraform > resource-types-contrib > avm > builtin
func isHigherPriorityRecipe(newRecipe, currentRecipe discovery.RecipeMatch) bool {
	return isHigherPrioritySource(string(newRecipe.Recipe.SourceType), string(currentRecipe.Recipe.SourceType))
}

// generateRecipeModuleWithSource generates a recipe module using source-aware template selection.
// Priority: 1. Local TF (copy from repo) → 2. resource-types-contrib → 3. AVM (generate)
//
// When local terraform is detected, we copy the relevant resources from the local repo.
// When no local TF exists, we generate from AVM.
func (r *Runner) generateRecipeModuleWithSource(resourceType, recipeDir, source, cloudProvider string) error {
	var recipeContent string

	// Check if this resource type has an AVM module available
	hasAVMModule := r.hasAVMModuleForType(resourceType)

	// Check if local terraform source was detected for this specific resource type
	isLocalTF := strings.HasPrefix(source, "local")

	// Check if Azure cloud provider is available
	isAzureAvailable := cloudProvider == "azure"

	// Determine template based on source and cloud provider
	switch {
	case isLocalTF:
		// Local TF detected - generate recipe based on local terraform
		// Comment indicates this is adapted from local repo
		recipeContent = r.getLocalTFRecipeTemplate(resourceType)
	case isAzureAvailable && hasAVMModule:
		// No local TF but Azure available and AVM module exists
		// Generate from AVM
		recipeContent = r.getAVMRecipeTemplate(resourceType)
	case strings.HasPrefix(source, "resource-types-contrib"):
		// Use resource-types-contrib template
		recipeContent = r.getContribRecipeTemplate(resourceType)
	case hasAVMModule:
		// Has AVM module - generate from AVM
		recipeContent = r.getAVMRecipeTemplate(resourceType)
	default:
		// Default to Kubernetes-based template
		recipeContent = r.getRecipeTemplate(resourceType)
	}

	mainTfPath := filepath.Join(recipeDir, "main.tf")
	return os.WriteFile(mainTfPath, []byte(recipeContent), 0644)
}

// getLocalTFRecipeTemplate returns a Radius recipe template based on local terraform resources.
// This wraps the resource definitions found in the local infra/ folder.
func (r *Runner) getLocalTFRecipeTemplate(resourceType string) string {
	switch resourceType {
	case "Radius.Data/redisCaches", "Applications.Datastores/redisCaches":
		return r.getLocalRedisRecipeTemplate()
	case "Radius.Data/mySqlDatabases", "Applications.Datastores/sqlDatabases":
		return r.getLocalMySQLRecipeTemplate()
	case "Radius.Data/postgreSqlDatabases":
		return r.getLocalPostgreSQLRecipeTemplate()
	default:
		// Fall back to AVM if no local template available
		return r.getAVMRecipeTemplate(resourceType)
	}
}

// getLocalRedisRecipeTemplate returns a recipe template copied from local terraform (infra/main.tf).
func (r *Runner) getLocalRedisRecipeTemplate() string {
	return `# Radius Recipe: Azure Cache for Redis
# Copied from local repository: infra/main.tf
# This recipe wraps the Redis resource definitions from your existing terraform code.

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 3.71.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type        = any
}

locals {
  name           = var.context.resource.name
  tags           = try(var.context.resource.tags, {})
  resource_group = try(var.context.azure.resourceGroup.name, "radius-${var.context.environment.name}")
  location       = try(var.context.azure.location, "eastus")
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

# Azure Cache for Redis
# Copied from local infra/main.tf - azurerm_redis_cache resource
resource "azurerm_redis_cache" "main" {
  name                = "${local.name}-${random_string.suffix.result}"
  location            = local.location
  resource_group_name = local.resource_group
  capacity            = 0
  family              = "C"
  sku_name            = "Basic"

  tags = merge(local.tags, {
    "radapp.io/resource"    = var.context.resource.id
    "radapp.io/environment" = var.context.environment.id
  })
}

output "result" {
  description = "Recipe output values for Radius"
  sensitive   = true
  value = {
    values = {
      host = azurerm_redis_cache.main.hostname
      port = azurerm_redis_cache.main.port
      tls  = false
    }
    secrets = {
      password   = azurerm_redis_cache.main.primary_access_key
      connection = azurerm_redis_cache.main.primary_connection_string
    }
    resources = [
      azurerm_redis_cache.main.id
    ]
  }
}
`
}

// getLocalMySQLRecipeTemplate returns a recipe template copied from local terraform (infra/main.tf).
func (r *Runner) getLocalMySQLRecipeTemplate() string {
	return `# Radius Recipe: Azure Database for MySQL
# Copied from local repository: infra/main.tf
# This recipe wraps the MySQL resource definitions from your existing terraform code.

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 3.71.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type        = any
}

locals {
  name           = var.context.resource.name
  tags           = try(var.context.resource.tags, {})
  resource_group = try(var.context.azure.resourceGroup.name, "radius-${var.context.environment.name}")
  location       = try(var.context.azure.location, "eastus")
  database_name  = var.context.resource.name
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

resource "random_password" "admin_password" {
  length           = 16
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

# Azure Database for MySQL Flexible Server
# Copied from local infra/main.tf - azurerm_mysql_flexible_server resource
resource "azurerm_mysql_flexible_server" "main" {
  name                   = "${local.name}-${random_string.suffix.result}"
  resource_group_name    = local.resource_group
  location               = local.location
  administrator_login    = "mysqladmin"
  administrator_password = random_password.admin_password.result
  sku_name               = "B_Standard_B1ms"
  version                = "8.0.21"

  tags = merge(local.tags, {
    "radapp.io/resource"    = var.context.resource.id
    "radapp.io/environment" = var.context.environment.id
  })
}

# MySQL Database
# Copied from local infra/main.tf - azurerm_mysql_flexible_database resource
resource "azurerm_mysql_flexible_database" "main" {
  name                = local.database_name
  resource_group_name = local.resource_group
  server_name         = azurerm_mysql_flexible_server.main.name
  charset             = "utf8mb4"
  collation           = "utf8mb4_unicode_ci"
}

# Firewall rule to allow Azure services
resource "azurerm_mysql_flexible_server_firewall_rule" "allow_azure" {
  name                = "AllowAzureServices"
  resource_group_name = local.resource_group
  server_name         = azurerm_mysql_flexible_server.main.name
  start_ip_address    = "0.0.0.0"
  end_ip_address      = "0.0.0.0"
}

output "result" {
  description = "Recipe output values for Radius"
  sensitive   = true
  value = {
    values = {
      host     = azurerm_mysql_flexible_server.main.fqdn
      port     = 3306
      database = azurerm_mysql_flexible_database.main.name
      username = "mysqladmin"
    }
    secrets = {
      password         = random_password.admin_password.result
      connectionString = "mysql://mysqladmin:${random_password.admin_password.result}@${azurerm_mysql_flexible_server.main.fqdn}:3306/${azurerm_mysql_flexible_database.main.name}"
    }
    resources = [
      azurerm_mysql_flexible_server.main.id
    ]
  }
}
`
}

// getLocalPostgreSQLRecipeTemplate returns a recipe template copied from local terraform.
func (r *Runner) getLocalPostgreSQLRecipeTemplate() string {
	// Fall back to AVM since PostgreSQL follows similar pattern
	return r.getAVMPostgreSQLRecipeTemplate()
}

// hasAVMModuleForType returns true if there's an AVM module available for the resource type.
func (r *Runner) hasAVMModuleForType(resourceType string) bool {
	avmSupportedTypes := map[string]bool{
		"Radius.Data/redisCaches":              true,
		"Applications.Datastores/redisCaches":  true,
		"Radius.Network/loadBalancers":         true,
		"Radius.Data/mySqlDatabases":           true,
		"Applications.Datastores/sqlDatabases": true,
		"Radius.Data/postgreSqlDatabases":      true,
	}
	return avmSupportedTypes[resourceType]
}

// generateRecipeModule generates a single Terraform recipe module for a resource type.
func (r *Runner) generateRecipeModule(resourceType, recipeDir string) error {
	// Determine the recipe template based on resource type
	recipeContent := r.getRecipeTemplate(resourceType)

	mainTfPath := filepath.Join(recipeDir, "main.tf")
	return os.WriteFile(mainTfPath, []byte(recipeContent), 0644)
}

// getRecipeTemplate returns a Terraform recipe template for the given resource type.
func (r *Runner) getRecipeTemplate(resourceType string) string {
	// Map resource types to recipe templates
	switch resourceType {
	case "Radius.Data/redisCaches", "Applications.Datastores/redisCaches":
		return r.getRedisRecipeTemplate()
	case "Radius.Data/mySqlDatabases", "Applications.Datastores/sqlDatabases":
		return r.getMySQLRecipeTemplate()
	case "Radius.Data/postgreSqlDatabases":
		return r.getPostgreSQLRecipeTemplate()
	case "Radius.Data/mongoDatabases", "Applications.Datastores/mongoDatabases":
		return r.getMongoDBRecipeTemplate()
	case "Radius.Network/loadBalancers":
		return r.getLoadBalancerRecipeTemplate()
	case "Radius.Messaging/rabbitMQQueues", "Applications.Messaging/rabbitMQQueues":
		return r.getRabbitMQRecipeTemplate()
	default:
		return r.getGenericRecipeTemplate(resourceType)
	}
}

// getContribRecipeTemplate returns a recipe template from resource-types-contrib.
// These are Kubernetes-based templates that follow the contrib repository patterns.
func (r *Runner) getContribRecipeTemplate(resourceType string) string {
	// For now, contrib templates are the same as built-in Kubernetes templates
	// In the future, these could be fetched from the contrib repository
	return r.getRecipeTemplate(resourceType)
}

// getAVMRecipeTemplate returns a Terraform recipe that wraps an Azure Verified Module.
// These recipes use AVM modules as the underlying implementation with Radius input/output.
func (r *Runner) getAVMRecipeTemplate(resourceType string) string {
	switch resourceType {
	case "Radius.Data/redisCaches", "Applications.Datastores/redisCaches":
		return r.getAVMRedisRecipeTemplate()
	case "Radius.Network/loadBalancers":
		return r.getAVMLoadBalancerRecipeTemplate()
	case "Radius.Data/mySqlDatabases", "Applications.Datastores/sqlDatabases":
		return r.getAVMMySQLRecipeTemplate()
	case "Radius.Data/postgreSqlDatabases":
		return r.getAVMPostgreSQLRecipeTemplate()
	default:
		// Fall back to Kubernetes template if no AVM module available
		return r.getRecipeTemplate(resourceType)
	}
}

// getAVMRedisRecipeTemplate returns a Terraform recipe for Azure Redis using AVM module.
func (r *Runner) getAVMRedisRecipeTemplate() string {
	return `# Radius Recipe: Azure Cache for Redis
# Auto-generated by rad app generate - no local terraform found for this resource type
# Uses Azure Verified Module (AVM): Azure/avm-res-cache-redis/azurerm

terraform {
  required_version = ">= 1.9.0, < 2.0.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0.0, < 5.0.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type        = any
}

locals {
  name           = var.context.resource.name
  tags           = try(var.context.resource.tags, {})
  resource_group = try(var.context.azure.resourceGroup.name, "radius-${var.context.environment.name}")
  location       = try(var.context.azure.location, "eastus")
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

# Azure Cache for Redis using AVM module
module "redis" {
  source  = "Azure/avm-res-cache-redis/azurerm"
  version = "0.4.0"

  name                = "${local.name}-${random_string.suffix.result}"
  resource_group_name = local.resource_group
  location            = local.location

  # Use Basic SKU for cost efficiency
  sku_name = "Basic"
  capacity = 0

  # Enable non-SSL port for development
  enable_non_ssl_port = true

  # Disable telemetry
  enable_telemetry = false

  tags = merge(local.tags, {
    "radapp.io/resource"    = var.context.resource.id
    "radapp.io/environment" = var.context.environment.id
  })
}

output "result" {
  description = "Recipe output values for Radius"
  sensitive   = true
  value = {
    values = {
      host = module.redis.resource.hostname
      port = module.redis.resource.port
      tls  = false
    }
    secrets = {
      password   = module.redis.resource.primary_access_key
      connection = module.redis.resource.primary_connection_string
    }
    resources = [
      module.redis.resource_id
    ]
  }
}
`
}

// getAVMLoadBalancerRecipeTemplate returns a Terraform recipe for Azure Load Balancer using AVM module.
func (r *Runner) getAVMLoadBalancerRecipeTemplate() string {
	return `# Radius Recipe: Azure Load Balancer
# Auto-generated by rad app generate - no local terraform found for this resource type
# Uses Azure Verified Module (AVM): Azure/avm-res-network-loadbalancer/azurerm

terraform {
  required_version = ">= 1.9.0, < 2.0.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0.0, < 5.0.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type        = any
}

locals {
  name           = var.context.resource.name
  tags           = try(var.context.resource.tags, {})
  resource_group = try(var.context.azure.resourceGroup.name, "radius-${var.context.environment.name}")
  location       = try(var.context.azure.location, "eastus")
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

# Azure Load Balancer using AVM module
module "loadbalancer" {
  source  = "Azure/avm-res-network-loadbalancer/azurerm"
  version = "0.5.0"

  name                = "${local.name}-lb-${random_string.suffix.result}"
  resource_group_name = local.resource_group
  location            = local.location

  # Frontend IP configuration with public IP
  frontend_ip_configurations = {
    frontend = {
      name                     = "frontend"
      create_public_ip_address = true
      public_ip_address_resource_name = "${local.name}-pip-${random_string.suffix.result}"
    }
  }

  tags = merge(local.tags, {
    "radapp.io/resource"    = var.context.resource.id
    "radapp.io/environment" = var.context.environment.id
  })
}

output "result" {
  description = "Recipe output values for Radius"
  value = {
    values = {
      hostname  = try(module.loadbalancer.azurerm_public_ip["frontend"].ip_address, "")
      publicIP  = try(module.loadbalancer.azurerm_public_ip["frontend"].ip_address, "")
      port      = 80
      lbId      = module.loadbalancer.resource_id
    }
    resources = [
      module.loadbalancer.resource_id
    ]
  }
}
`
}

// getAVMMySQLRecipeTemplate returns a Terraform recipe for Azure MySQL using native azurerm resources.
// Note: Using native resources instead of AVM module due to Terraform version compatibility.
func (r *Runner) getAVMMySQLRecipeTemplate() string {
	return `# Radius Recipe: Azure Database for MySQL
# Auto-generated by rad app generate - no local terraform found for this resource type
# Uses native azurerm resources (AVM modules require Terraform < 1.13.0, Radius uses newer version)

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 3.71.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type        = any
}

locals {
  name           = var.context.resource.name
  tags           = try(var.context.resource.tags, {})
  resource_group = try(var.context.azure.resourceGroup.name, "radius-${var.context.environment.name}")
  location       = try(var.context.azure.location, "eastus")
  sku_name       = "B_Standard_B1s"
  mysql_version  = "8.0.21"
  database_name  = var.context.resource.name
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

resource "random_password" "admin_password" {
  length           = 16
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

# Azure Database for MySQL Flexible Server using native azurerm resources
resource "azurerm_mysql_flexible_server" "mysql" {
  name                   = "${local.name}-${random_string.suffix.result}"
  resource_group_name    = local.resource_group
  location               = local.location
  administrator_login    = "mysqladmin"
  administrator_password = random_password.admin_password.result
  sku_name               = local.sku_name
  version                = local.mysql_version

  tags = merge(local.tags, {
    "radapp.io/resource"    = var.context.resource.id
    "radapp.io/environment" = var.context.environment.id
  })
}

resource "azurerm_mysql_flexible_database" "database" {
  name                = local.database_name
  resource_group_name = local.resource_group
  server_name         = azurerm_mysql_flexible_server.mysql.name
  charset             = "utf8mb4"
  collation           = "utf8mb4_unicode_ci"
}

# Firewall rule to allow Azure services
resource "azurerm_mysql_flexible_server_firewall_rule" "allow_azure" {
  name                = "AllowAzureServices"
  resource_group_name = local.resource_group
  server_name         = azurerm_mysql_flexible_server.mysql.name
  start_ip_address    = "0.0.0.0"
  end_ip_address      = "0.0.0.0"
}

output "result" {
  description = "Recipe output values for Radius"
  sensitive   = true
  value = {
    values = {
      host     = azurerm_mysql_flexible_server.mysql.fqdn
      port     = 3306
      database = azurerm_mysql_flexible_database.database.name
      username = "mysqladmin"
    }
    secrets = {
      password         = random_password.admin_password.result
      connectionString = "mysql://mysqladmin:${random_password.admin_password.result}@${azurerm_mysql_flexible_server.mysql.fqdn}:3306/${azurerm_mysql_flexible_database.database.name}"
    }
    resources = [
      azurerm_mysql_flexible_server.mysql.id
    ]
  }
}
`
}

// getAVMPostgreSQLRecipeTemplate returns a Terraform recipe wrapping the Azure PostgreSQL AVM module.
func (r *Runner) getAVMPostgreSQLRecipeTemplate() string {
	return `# Radius Recipe: Azure Database for PostgreSQL (AVM)
# Auto-generated by rad app generate
# Uses Azure Verified Module: avm/res/db-for-postgre-sql/flexible-server

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 3.71.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.5.0"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type        = any
}

locals {
  name           = var.context.resource.name
  tags           = try(var.context.resource.tags, {})
  resource_group = try(var.context.azure.resourceGroup.name, "radius-${var.context.environment.name}")
  location       = try(var.context.azure.location, "eastus")
  sku_name       = "B_Standard_B1ms"
  pg_version     = "15"
  database_name  = var.context.resource.name
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

resource "random_password" "admin_password" {
  length           = 16
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

# Azure Database for PostgreSQL Flexible Server using AVM module
module "postgresql" {
  source  = "Azure/avm-res-dbforpostgresql-flexibleserver/azurerm"
  version = "~> 0.1"

  name                = "${local.name}-${random_string.suffix.result}"
  resource_group_name = local.resource_group
  location            = local.location
  sku_name            = local.sku_name
  version             = local.pg_version

  administrator_login    = "pgadmin"
  administrator_password = random_password.admin_password.result

  databases = {
    (local.database_name) = {
      charset   = "UTF8"
      collation = "en_US.utf8"
    }
  }

  tags = merge(local.tags, {
    "radapp.io/resource"    = var.context.resource.id
    "radapp.io/environment" = var.context.environment.id
  })
}

output "result" {
  description = "Recipe output values for Radius"
  sensitive   = true
  value = {
    values = {
      host     = module.postgresql.fqdn
      port     = 5432
      database = local.database_name
      username = "pgadmin"
    }
    secrets = {
      password         = random_password.admin_password.result
      connectionString = "postgresql://pgadmin:${random_password.admin_password.result}@${module.postgresql.fqdn}:5432/${local.database_name}"
    }
    resources = [
      module.postgresql.resource_id
    ]
  }
}
`
}

// getRedisRecipeTemplate returns a Terraform recipe for Redis on Kubernetes.
func (r *Runner) getRedisRecipeTemplate() string {
	return `# Radius Recipe: Redis Cache for Kubernetes
# Auto-generated by rad app generate

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
  port      = 6379
}

resource "kubernetes_deployment" "redis" {
  metadata {
    name      = local.name
    namespace = local.namespace
    labels = {
      app                     = local.name
      "radapp.io/resource"    = var.context.resource.id
      "radapp.io/environment" = var.context.environment.id
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = local.name
      }
    }

    template {
      metadata {
        labels = {
          app = local.name
        }
      }

      spec {
        container {
          name  = "redis"
          image = "redis:7-alpine"

          port {
            container_port = local.port
          }

          resources {
            limits = {
              memory = "128Mi"
              cpu    = "250m"
            }
            requests = {
              memory = "64Mi"
              cpu    = "100m"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "redis" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }

  spec {
    selector = {
      app = local.name
    }

    port {
      port        = local.port
      target_port = local.port
    }

    type = "ClusterIP"
  }
}

output "result" {
  value = {
    values = {
      host     = "${kubernetes_service.redis.metadata[0].name}.${local.namespace}.svc.cluster.local"
      port     = local.port
      password = ""
    }
  }
}
`
}

// getMySQLRecipeTemplate returns a Terraform recipe for MySQL on Kubernetes.
func (r *Runner) getMySQLRecipeTemplate() string {
	return `# Radius Recipe: MySQL Database for Kubernetes
# Auto-generated by rad app generate

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
  port      = 3306
  database  = "appdb"
  username  = "radius"
}

resource "random_password" "mysql" {
  length  = 16
  special = false
}

resource "kubernetes_secret" "mysql" {
  metadata {
    name      = "${local.name}-secret"
    namespace = local.namespace
  }

  data = {
    MYSQL_ROOT_PASSWORD = random_password.mysql.result
    MYSQL_PASSWORD      = random_password.mysql.result
  }
}

resource "kubernetes_deployment" "mysql" {
  metadata {
    name      = local.name
    namespace = local.namespace
    labels = {
      app                     = local.name
      "radapp.io/resource"    = var.context.resource.id
      "radapp.io/environment" = var.context.environment.id
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = local.name
      }
    }

    template {
      metadata {
        labels = {
          app = local.name
        }
      }

      spec {
        container {
          name  = "mysql"
          image = "mysql:8.0"

          port {
            container_port = local.port
          }

          env {
            name  = "MYSQL_DATABASE"
            value = local.database
          }

          env {
            name  = "MYSQL_USER"
            value = local.username
          }

          env {
            name = "MYSQL_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.mysql.metadata[0].name
                key  = "MYSQL_PASSWORD"
              }
            }
          }

          env {
            name = "MYSQL_ROOT_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.mysql.metadata[0].name
                key  = "MYSQL_ROOT_PASSWORD"
              }
            }
          }

          resources {
            limits = {
              memory = "512Mi"
              cpu    = "500m"
            }
            requests = {
              memory = "256Mi"
              cpu    = "250m"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "mysql" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }

  spec {
    selector = {
      app = local.name
    }

    port {
      port        = local.port
      target_port = local.port
    }

    type = "ClusterIP"
  }
}

output "result" {
  value = {
    values = {
      host     = "${kubernetes_service.mysql.metadata[0].name}.${local.namespace}.svc.cluster.local"
      port     = local.port
      database = local.database
      username = local.username
      password = random_password.mysql.result
    }
  }
  sensitive = true
}
`
}

// getPostgreSQLRecipeTemplate returns a Terraform recipe for PostgreSQL on Kubernetes.
func (r *Runner) getPostgreSQLRecipeTemplate() string {
	return `# Radius Recipe: PostgreSQL Database for Kubernetes
# Auto-generated by rad app generate

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
  port      = 5432
  database  = "appdb"
  username  = "radius"
}

resource "random_password" "postgres" {
  length  = 16
  special = false
}

resource "kubernetes_deployment" "postgres" {
  metadata {
    name      = local.name
    namespace = local.namespace
    labels = {
      app                     = local.name
      "radapp.io/resource"    = var.context.resource.id
      "radapp.io/environment" = var.context.environment.id
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = local.name
      }
    }

    template {
      metadata {
        labels = {
          app = local.name
        }
      }

      spec {
        container {
          name  = "postgres"
          image = "postgres:15-alpine"

          port {
            container_port = local.port
          }

          env {
            name  = "POSTGRES_DB"
            value = local.database
          }

          env {
            name  = "POSTGRES_USER"
            value = local.username
          }

          env {
            name  = "POSTGRES_PASSWORD"
            value = random_password.postgres.result
          }

          resources {
            limits = {
              memory = "512Mi"
              cpu    = "500m"
            }
            requests = {
              memory = "256Mi"
              cpu    = "250m"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "postgres" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }

  spec {
    selector = {
      app = local.name
    }

    port {
      port        = local.port
      target_port = local.port
    }

    type = "ClusterIP"
  }
}

output "result" {
  value = {
    values = {
      host     = "${kubernetes_service.postgres.metadata[0].name}.${local.namespace}.svc.cluster.local"
      port     = local.port
      database = local.database
      username = local.username
      password = random_password.postgres.result
    }
  }
  sensitive = true
}
`
}

// getMongoDBRecipeTemplate returns a Terraform recipe for MongoDB on Kubernetes.
func (r *Runner) getMongoDBRecipeTemplate() string {
	return `# Radius Recipe: MongoDB for Kubernetes
# Auto-generated by rad app generate

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
  port      = 27017
  database  = "appdb"
}

resource "random_password" "mongo" {
  length  = 16
  special = false
}

resource "kubernetes_deployment" "mongo" {
  metadata {
    name      = local.name
    namespace = local.namespace
    labels = {
      app                     = local.name
      "radapp.io/resource"    = var.context.resource.id
      "radapp.io/environment" = var.context.environment.id
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = local.name
      }
    }

    template {
      metadata {
        labels = {
          app = local.name
        }
      }

      spec {
        container {
          name  = "mongo"
          image = "mongo:6"

          port {
            container_port = local.port
          }

          env {
            name  = "MONGO_INITDB_ROOT_USERNAME"
            value = "root"
          }

          env {
            name  = "MONGO_INITDB_ROOT_PASSWORD"
            value = random_password.mongo.result
          }

          resources {
            limits = {
              memory = "512Mi"
              cpu    = "500m"
            }
            requests = {
              memory = "256Mi"
              cpu    = "250m"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "mongo" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }

  spec {
    selector = {
      app = local.name
    }

    port {
      port        = local.port
      target_port = local.port
    }

    type = "ClusterIP"
  }
}

output "result" {
  value = {
    values = {
      host             = "${kubernetes_service.mongo.metadata[0].name}.${local.namespace}.svc.cluster.local"
      port             = local.port
      database         = local.database
      connectionString = "mongodb://root:${random_password.mongo.result}@${kubernetes_service.mongo.metadata[0].name}.${local.namespace}.svc.cluster.local:${local.port}/${local.database}"
    }
  }
  sensitive = true
}
`
}

// getLoadBalancerRecipeTemplate returns a Terraform recipe for a load balancer on Kubernetes.
func (r *Runner) getLoadBalancerRecipeTemplate() string {
	return `# Radius Recipe: Load Balancer for Kubernetes
# Auto-generated by rad app generate
# Note: This creates an Nginx ingress controller for load balancing

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
  port      = 80
}

resource "kubernetes_deployment" "nginx" {
  metadata {
    name      = local.name
    namespace = local.namespace
    labels = {
      app                     = local.name
      "radapp.io/resource"    = var.context.resource.id
      "radapp.io/environment" = var.context.environment.id
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = local.name
      }
    }

    template {
      metadata {
        labels = {
          app = local.name
        }
      }

      spec {
        container {
          name  = "nginx"
          image = "nginx:alpine"

          port {
            container_port = local.port
          }

          resources {
            limits = {
              memory = "128Mi"
              cpu    = "100m"
            }
            requests = {
              memory = "64Mi"
              cpu    = "50m"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "nginx" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }

  spec {
    selector = {
      app = local.name
    }

    port {
      port        = local.port
      target_port = local.port
    }

    type = "LoadBalancer"
  }
}

output "result" {
  value = {
    values = {
      hostname = kubernetes_service.nginx.status[0].load_balancer[0].ingress[0].hostname
      port     = local.port
      scheme   = "http"
    }
  }
}
`
}

// getRabbitMQRecipeTemplate returns a Terraform recipe for RabbitMQ on Kubernetes.
func (r *Runner) getRabbitMQRecipeTemplate() string {
	return `# Radius Recipe: RabbitMQ for Kubernetes
# Auto-generated by rad app generate

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
  port      = 5672
  mgmt_port = 15672
}

resource "random_password" "rabbitmq" {
  length  = 16
  special = false
}

resource "kubernetes_deployment" "rabbitmq" {
  metadata {
    name      = local.name
    namespace = local.namespace
    labels = {
      app                     = local.name
      "radapp.io/resource"    = var.context.resource.id
      "radapp.io/environment" = var.context.environment.id
    }
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = local.name
      }
    }

    template {
      metadata {
        labels = {
          app = local.name
        }
      }

      spec {
        container {
          name  = "rabbitmq"
          image = "rabbitmq:3-management-alpine"

          port {
            container_port = local.port
          }

          port {
            container_port = local.mgmt_port
          }

          env {
            name  = "RABBITMQ_DEFAULT_USER"
            value = "radius"
          }

          env {
            name  = "RABBITMQ_DEFAULT_PASS"
            value = random_password.rabbitmq.result
          }

          resources {
            limits = {
              memory = "256Mi"
              cpu    = "250m"
            }
            requests = {
              memory = "128Mi"
              cpu    = "100m"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "rabbitmq" {
  metadata {
    name      = local.name
    namespace = local.namespace
  }

  spec {
    selector = {
      app = local.name
    }

    port {
      name        = "amqp"
      port        = local.port
      target_port = local.port
    }

    port {
      name        = "management"
      port        = local.mgmt_port
      target_port = local.mgmt_port
    }

    type = "ClusterIP"
  }
}

output "result" {
  value = {
    values = {
      host     = "${kubernetes_service.rabbitmq.metadata[0].name}.${local.namespace}.svc.cluster.local"
      port     = local.port
      queue    = "default"
      username = "radius"
      password = random_password.rabbitmq.result
    }
  }
  sensitive = true
}
`
}

// getGenericRecipeTemplate returns a generic Terraform recipe template.
func (r *Runner) getGenericRecipeTemplate(resourceType string) string {
	parts := strings.Split(resourceType, "/")
	shortName := parts[len(parts)-1]

	return fmt.Sprintf(`# Radius Recipe: %s for Kubernetes
# Auto-generated by rad app generate
# TODO: Customize this recipe for your specific resource type

terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
  }
}

variable "context" {
  description = "Radius-provided context for the recipe"
  type = any
}

locals {
  name      = var.context.resource.name
  namespace = var.context.runtime.kubernetes.namespace
}

# TODO: Add your resource definitions here
# Example:
# resource "kubernetes_deployment" "%s" {
#   metadata {
#     name      = local.name
#     namespace = local.namespace
#   }
#   ...
# }

output "result" {
  value = {
    values = {
      # TODO: Add output values that your application needs
      # Example:
      # host = "..."
      # port = 8080
    }
  }
}
`, shortName, strings.ToLower(shortName))
}

// detectPreferredIaC detects the preferred IaC language from the local repository.
func (r *Runner) detectPreferredIaC(result *discovery.DiscoveryResult) string {
	// Check if any local-terraform recipes were found
	for _, recipe := range result.Recipes {
		st := strings.ToLower(string(recipe.Recipe.SourceType))
		if strings.Contains(st, "terraform") {
			return "terraform"
		}
	}

	// Check practices for terraform files
	for _, src := range result.Practices.ExtractedFrom {
		if string(src.Type) == "terraform" {
			return "terraform"
		}
	}

	return "bicep"
}

// getAVMRecipeForType returns an AVM recipe for the given resource type if available.
// If preferTerraform is true, returns terraform module references.
func (r *Runner) getAVMRecipeForType(resourceType string, preferTerraform bool) *dtypes.Recipe {
	if preferTerraform {
		return r.getAVMTerraformRecipe(resourceType)
	}
	return r.getAVMBicepRecipe(resourceType)
}

// getAVMTerraformRecipe returns AVM terraform module references.
// AVM modules are available on Terraform Registry: https://registry.terraform.io/namespaces/Azure
func (r *Runner) getAVMTerraformRecipe(resourceType string) *dtypes.Recipe {
	// Map resource types to AVM Terraform modules
	// See: https://azure.github.io/Azure-Verified-Modules/indexes/terraform/
	avmModules := map[string]dtypes.Recipe{
		"Radius.Data/mySqlDatabases": {
			Name:           "avm-mysql",
			Description:    "Azure Database for MySQL using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-dbformysql-flexibleserver/azurerm",
			Version:        "0.3.0",
		},
		"Radius.Data/postgreSqlDatabases": {
			Name:           "avm-postgresql",
			Description:    "Azure Database for PostgreSQL using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-dbforpostgresql-flexibleserver/azurerm",
			Version:        "0.1.1",
		},
		"Radius.Data/redisCaches": {
			Name:           "avm-redis",
			Description:    "Azure Cache for Redis using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-cache-redis/azurerm",
			Version:        "0.1.5",
		},
		"Radius.Data/mongoDatabases": {
			Name:           "avm-cosmosdb",
			Description:    "Azure Cosmos DB for MongoDB using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-documentdb-databaseaccount/azurerm",
			Version:        "0.2.0",
		},
		"Radius.Network/loadBalancers": {
			Name:           "avm-loadbalancer",
			Description:    "Azure Load Balancer using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-network-loadbalancer/azurerm",
			Version:        "0.2.2",
		},
		"Radius.Messaging/rabbitMQQueues": {
			Name:           "avm-servicebus",
			Description:    "Azure Service Bus using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-servicebus-namespace/azurerm",
			Version:        "0.2.1",
		},
		"Radius.Security/secrets": {
			Name:           "avm-keyvault",
			Description:    "Azure Key Vault using AVM Terraform module",
			SourceType:     dtypes.RecipeSourceTerraform,
			SourceLocation: "Azure/avm-res-keyvault-vault/azurerm",
			Version:        "0.9.1",
		},
	}

	if recipe, exists := avmModules[resourceType]; exists {
		return &recipe
	}
	return nil
}

// getAVMBicepRecipe returns AVM Bicep module references.
func (r *Runner) getAVMBicepRecipe(resourceType string) *dtypes.Recipe {
	// Map resource types to AVM Bicep modules
	avmModules := map[string]dtypes.Recipe{
		"Radius.Data/mySqlDatabases": {
			Name:           "avm-mysql",
			Description:    "Azure Database for MySQL using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/db-for-my-sql/flexible-server:0.4.1",
		},
		"Radius.Data/postgreSqlDatabases": {
			Name:           "avm-postgresql",
			Description:    "Azure Database for PostgreSQL using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/db-for-postgre-sql/flexible-server:0.3.0",
		},
		"Radius.Data/redisCaches": {
			Name:           "avm-redis",
			Description:    "Azure Cache for Redis using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/cache/redis:0.3.1",
		},
		"Radius.Data/mongoDatabases": {
			Name:           "avm-cosmosdb",
			Description:    "Azure Cosmos DB for MongoDB using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/document-db/database-account:0.8.1",
		},
		"Radius.Network/loadBalancers": {
			Name:           "avm-loadbalancer",
			Description:    "Azure Load Balancer using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/network/load-balancer:0.4.0",
		},
		"Radius.Messaging/rabbitMQQueues": {
			Name:           "avm-servicebus",
			Description:    "Azure Service Bus Queue using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/service-bus/namespace:0.10.1",
		},
		"Radius.Security/secrets": {
			Name:           "avm-keyvault",
			Description:    "Azure Key Vault using AVM Bicep module",
			SourceType:     dtypes.RecipeSourceAVM,
			SourceLocation: "br/public:avm/res/key-vault/vault:0.9.0",
		},
	}

	if recipe, exists := avmModules[resourceType]; exists {
		return &recipe
	}
	return nil
}

// getRecipeSourcePriorityWithPreference returns priority based on IaC preference.
func getRecipeSourcePriorityWithPreference(sourceType, sourceLocation string, preferTerraform bool) int {
	loc := strings.ToLower(sourceLocation)
	st := strings.ToLower(sourceType)
	isTerraform := strings.Contains(st, "terraform") || strings.Contains(loc, ".tf")

	if preferTerraform {
		// Prefer terraform recipes
		switch {
		case strings.Contains(loc, "local") && isTerraform:
			return 1 // Local terraform (highest)
		case isTerraform:
			return 2 // Any terraform recipe
		case strings.Contains(loc, "local"):
			return 3 // Local bicep
		case strings.Contains(loc, "contrib"):
			return 4 // resource-types-contrib
		case st == "avm":
			return 5 // AVM bicep
		default:
			return 6
		}
	}

	// Default: prefer local, then contrib, then avm
	switch {
	case strings.Contains(loc, "local") && isTerraform:
		return 1
	case st == "local" || strings.Contains(loc, "local"):
		return 2
	case strings.Contains(loc, "contrib"):
		return 3
	case st == "avm":
		return 4
	default:
		return 5
	}
}

// getRecipeSourcePriority returns priority for recipe sources (lower = higher priority)
func getRecipeSourcePriority(sourceType, sourceLocation string) int {
	return getRecipeSourcePriorityWithPreference(sourceType, sourceLocation, false)
}

// sanitizeBicepName converts a name to a valid Bicep identifier
func sanitizeBicepName(name string) string {
	// Replace common invalid characters
	result := strings.ReplaceAll(name, "-", "_")
	result = strings.ReplaceAll(result, ".", "_")
	result = strings.ReplaceAll(result, "/", "_")
	return result
}

// getTemplateKind determines the template kind for a recipe
func getTemplateKind(recipe dtypes.Recipe) string {
	// Check source type first
	st := string(recipe.SourceType)
	if recipe.SourceType == dtypes.RecipeSourceTerraform {
		return "terraform"
	}
	// Check for local-terraform (stored as "local-terraform" in SourceType)
	if strings.Contains(strings.ToLower(st), "terraform") {
		return "terraform"
	}
	// Check source location for terraform indicators
	loc := strings.ToLower(recipe.SourceLocation)
	if strings.Contains(loc, ".tf") || strings.Contains(loc, "terraform") {
		return "terraform"
	}
	return "bicep"
}

// formatTemplatePath formats the template path for a recipe to be invocable by Radius.
// For local terraform modules, generates a Git URL or placeholder for publishing.
// For registry modules, ensures proper format.
func (r *Runner) formatTemplatePath(recipe dtypes.Recipe, outputPath string) string {
	loc := recipe.SourceLocation
	st := strings.ToLower(string(recipe.SourceType))

	// Handle local-terraform recipes - need Git URL for Radius to invoke
	if strings.Contains(st, "local-terraform") || strings.Contains(st, "local") {
		// Derive project path from output path
		// outputPath is like /path/to/project/radius/env.bicep
		// We need to go up two levels: radius/ -> project/
		projectDir := filepath.Dir(filepath.Dir(outputPath))
		gitURL := r.detectGitRemoteURL(projectDir)
		if gitURL != "" {
			// Format: git::https://github.com/org/repo.git//path/to/module
			// NOTE: The local terraform module must be committed and pushed to the repo
			// for Radius to fetch it during deployment
			return fmt.Sprintf("git::%s//%s", gitURL, loc)
		}
		// No git remote - provide a placeholder that user needs to update
		return fmt.Sprintf("git::https://github.com/<your-org>/<your-repo>.git//%s", loc)
	}

	// Handle terraform registry modules
	if strings.Contains(st, "terraform") {
		// If it's already a registry path, return as-is
		if strings.HasPrefix(loc, "registry.terraform.io/") {
			return loc
		}
		// Format: Azure/module-name/azurerm -> registry.terraform.io/Azure/module-name/azurerm
		if strings.Count(loc, "/") == 2 && !strings.Contains(loc, "://") {
			return fmt.Sprintf("registry.terraform.io/%s", loc)
		}
	}

	// Handle bicep registry modules (br: prefix)
	if strings.HasPrefix(loc, "br:") || strings.HasPrefix(loc, "br/") {
		return loc
	}

	// Default: return as-is
	if loc == "" {
		return fmt.Sprintf("br:ghcr.io/radius-project/recipes/%s:latest", recipe.Name)
	}
	return loc
}

// formatAVMTemplatePath formats the template path for an AVM recipe.
func (r *Runner) formatAVMTemplatePath(recipe *dtypes.Recipe) string {
	loc := recipe.SourceLocation

	// Terraform registry modules
	if recipe.SourceType == dtypes.RecipeSourceTerraform {
		// Format: Azure/module-name/azurerm -> registry.terraform.io/Azure/module-name/azurerm
		if !strings.HasPrefix(loc, "registry.terraform.io/") {
			return fmt.Sprintf("registry.terraform.io/%s", loc)
		}
		return loc
	}

	// Bicep modules - already in br: format
	return loc
}

// detectGitRemoteURL attempts to detect the git remote URL for the project.
func (r *Runner) detectGitRemoteURL(projectDir string) string {
	if projectDir == "" {
		return ""
	}

	// Check if .git directory exists
	gitDir := filepath.Join(projectDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return ""
	}

	// Try to read git config for remote URL
	gitConfig := filepath.Join(gitDir, "config")
	data, err := os.ReadFile(gitConfig)
	if err != nil {
		return ""
	}

	// Parse git config to find remote origin URL
	lines := strings.Split(string(data), "\n")
	inRemoteOrigin := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[remote \"origin\"]" {
			inRemoteOrigin = true
			continue
		}
		if inRemoteOrigin && strings.HasPrefix(line, "[") {
			break
		}
		if inRemoteOrigin && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			// Convert SSH URL to HTTPS
			if strings.HasPrefix(url, "git@github.com:") {
				url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
			}
			// Remove .git suffix if present
			url = strings.TrimSuffix(url, ".git")
			return url + ".git"
		}
	}

	return ""
}

// generateBicepExtensions generates Bicep extension .tgz files from types YAML
// Uses: rad bicep publish-extension -f <yaml> --target <extension>.tgz
// Per: https://docs.radapp.io/tutorials/create-resource-type/
func (r *Runner) generateBicepExtensions(result *discovery.DiscoveryResult, outputDir string) error {
	if len(result.ResourceTypes) == 0 {
		return nil
	}

	// Create extensions directory
	extensionsDir := filepath.Join(outputDir, "extensions")
	if err := os.MkdirAll(extensionsDir, 0755); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}

	// Get unique namespaces
	namespaces := make(map[string]bool)
	for _, rtMatch := range result.ResourceTypes {
		parts := strings.Split(rtMatch.ResourceType.Name, "/")
		if len(parts) == 2 {
			namespaces[parts[0]] = true
		}
	}

	// Generate extension for each namespace
	typesDir := filepath.Join(outputDir, "types")
	for namespace := range namespaces {
		typesFile := filepath.Join(typesDir, namespace+".yaml")

		// Check if types file exists
		if _, err := os.Stat(typesFile); os.IsNotExist(err) {
			continue
		}

		// Extension name follows Radius convention: lowercase without dots
		// e.g., "Radius.Data" -> "radiusdata"
		extensionName := strings.ToLower(strings.ReplaceAll(namespace, ".", ""))
		tgzPath := filepath.Join(extensionsDir, extensionName+".tgz")

		// Use rad bicep publish-extension -f <yaml> --target <tgz>
		if err := r.runRadBicepPublishExtension(typesFile, tgzPath); err != nil {
			r.Output.LogInfo("    ⚠ %s: %v", extensionName, err)
			continue
		}

		r.Output.LogInfo("    ✓ %s.tgz", extensionName)
	}

	return nil
}

// runRadBicepPublishExtension runs: rad bicep publish-extension -f <yaml> --target <tgz> --force
func (r *Runner) runRadBicepPublishExtension(yamlPath string, targetPath string) error {
	// Get the path to the current rad executable
	radPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find rad executable: %w", err)
	}

	// Run: rad bicep publish-extension -f <yaml> --target <tgz> --force
	cmd := exec.CommandContext(context.Background(), radPath,
		"bicep", "publish-extension",
		"-f", yamlPath,
		"--target", targetPath,
		"--force",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rad bicep publish-extension failed: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// generateBicepConfig generates bicepconfig.json with extension references
// Follows the pattern from: https://docs.radapp.io/tutorials/create-resource-type/
func (r *Runner) generateBicepConfig(result *discovery.DiscoveryResult, outputDir string) error {
	// Get Radius version for extension references
	ver := version.Channel()
	if ver == "" {
		ver = "latest"
	}

	// Build extensions map with standard Radius extensions
	extensions := map[string]string{
		"radius": fmt.Sprintf("br:biceptypes.azurecr.io/radius:%s", ver),
	}

	// Add local extensions for dynamic/custom types
	namespaces := make(map[string]bool)
	for _, rtMatch := range result.ResourceTypes {
		parts := strings.Split(rtMatch.ResourceType.Name, "/")
		if len(parts) == 2 {
			namespace := parts[0]
			// Only add if not a built-in Radius type
			if !isBuiltInRadiusType(namespace) {
				namespaces[namespace] = true
			}
		}
	}

	// Add local extension paths for custom resource types
	// Extension name is namespace in lowercase without dots (e.g., "Radius.Data" -> "radiusdata")
	extensionsDir := filepath.Join(outputDir, "extensions")
	for namespace := range namespaces {
		extensionName := strings.ToLower(strings.ReplaceAll(namespace, ".", ""))
		tgzPath := filepath.Join(extensionsDir, extensionName+".tgz")

		// Check if the .tgz extension file exists
		if _, err := os.Stat(tgzPath); err == nil {
			// Use relative path from bicepconfig.json to extension
			relPath := "extensions/" + extensionName + ".tgz"
			extensions[extensionName] = relPath
		} else {
			// Fallback: check for extension files directory
			extFilesDir := filepath.Join(extensionsDir, namespace)
			if _, err := os.Stat(extFilesDir); err == nil {
				// If we have the extension files but not .tgz, reference the directory
				extensions[extensionName] = "extensions/" + namespace
			}
		}
	}

	// Build config structure following Radius docs
	config := map[string]interface{}{
		"experimentalFeaturesEnabled": map[string]bool{
			"extensibility": true,
		},
		"extensions": extensions,
	}

	// Write bicepconfig.json
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling bicepconfig.json: %w", err)
	}

	configPath := filepath.Join(outputDir, "bicepconfig.json")
	return os.WriteFile(configPath, configBytes, 0644)
}

// isBuiltInRadiusType checks if a namespace is a built-in Radius type
func isBuiltInRadiusType(namespace string) bool {
	builtIn := map[string]bool{
		"Applications.Core":       true,
		"Applications.Dapr":       true,
		"Applications.Datastores": true,
		"Applications.Messaging":  true,
	}
	return builtIn[namespace]
}
