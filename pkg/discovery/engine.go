// Package discovery provides automatic application discovery for Radius.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/discovery/analyzers"
	"github.com/radius-project/radius/pkg/discovery/catalog"
	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/output"
	"github.com/radius-project/radius/pkg/discovery/practices"
	"github.com/radius-project/radius/pkg/discovery/resourcetypes"
	"github.com/radius-project/radius/pkg/version"
)

// Engine orchestrates the discovery workflow.
type Engine struct {
	analyzerRegistry *analyzers.Registry
	libraryCatalog   *catalog.LibraryCatalog
	resourceCatalog  *resourcetypes.Catalog
	markdownGen      *output.MarkdownGenerator
}

// NewEngine creates a new discovery engine.
func NewEngine() (*Engine, error) {
	mdGen, err := output.NewMarkdownGenerator()
	if err != nil {
		return nil, fmt.Errorf("creating markdown generator: %w", err)
	}

	return &Engine{
		analyzerRegistry: analyzers.DefaultRegistry,
		libraryCatalog:   catalog.DefaultCatalog,
		resourceCatalog:  resourcetypes.DefaultCatalog,
		markdownGen:      mdGen,
	}, nil
}

// NewEngineWithCatalogs creates an engine with custom catalogs.
func NewEngineWithCatalogs(
	analyzerReg *analyzers.Registry,
	libCatalog *catalog.LibraryCatalog,
	resCatalog *resourcetypes.Catalog,
) (*Engine, error) {
	mdGen, err := output.NewMarkdownGenerator()
	if err != nil {
		return nil, fmt.Errorf("creating markdown generator: %w", err)
	}

	return &Engine{
		analyzerRegistry: analyzerReg,
		libraryCatalog:   libCatalog,
		resourceCatalog:  resCatalog,
		markdownGen:      mdGen,
	}, nil
}

// DiscoverOptions configures the discovery process.
type DiscoverOptions struct {
	ProjectPath    string
	MinConfidence  float64
	IncludeDevDeps bool
	OutputPath     string
	Verbose        bool
}

// DefaultDiscoverOptions returns sensible defaults.
func DefaultDiscoverOptions(projectPath string) DiscoverOptions {
	return DiscoverOptions{
		ProjectPath:    projectPath,
		MinConfidence:  0.5,
		IncludeDevDeps: false,
		OutputPath:     filepath.Join(projectPath, "radius", "discovery.md"),
		Verbose:        false,
	}
}

// Discover analyzes a project and returns discovery results.
func (e *Engine) Discover(ctx context.Context, opts DiscoverOptions) (*DiscoveryResult, error) {
	// Validate project path
	if _, err := os.Stat(opts.ProjectPath); os.IsNotExist(err) {
		return nil, ErrProjectNotFound
	}

	result := &DiscoveryResult{
		ProjectPath:     opts.ProjectPath,
		AnalyzedAt:      time.Now(),
		AnalyzerVersion: version.Version(),
		Services:        make([]Service, 0),
		Dependencies:    make([]DetectedDependency, 0),
		ResourceTypes:   make([]ResourceTypeMapping, 0),
		Recipes:         make([]RecipeMatch, 0),
		Warnings:        make([]DiscoveryWarning, 0),
	}

	// Run analyzers
	analyzeOpts := analyzers.AnalyzeOptions{
		MinConfidence:  opts.MinConfidence,
		IncludeDevDeps: opts.IncludeDevDeps,
	}

	analyzerResults, err := e.analyzerRegistry.DetectAndAnalyze(ctx, opts.ProjectPath, analyzeOpts)
	if err != nil {
		return nil, fmt.Errorf("running analyzers: %w", err)
	}

	if len(analyzerResults) == 0 {
		result.Warnings = append(result.Warnings, DiscoveryWarning{
			Level:   WarningWarning,
			Code:    "NO_LANGUAGE",
			Message: "No supported programming language detected in the project",
		})
		return result, nil
	}

	// Aggregate results from all analyzers
	for _, ar := range analyzerResults {
		result.Services = append(result.Services, ar.Services...)
		result.Dependencies = append(result.Dependencies, ar.Dependencies...)

		for _, w := range ar.Warnings {
			result.Warnings = append(result.Warnings, w)
		}
	}

	// Enrich with Docker/Compose information
	e.enrichWithDockerInfo(ctx, opts.ProjectPath, result)

	// Detect team practices from IaC files
	detectedPractices := e.detectTeamPractices(opts.ProjectPath)
	if detectedPractices != nil {
		result.Practices = *detectedPractices
	}

	// Map dependencies to Resource Types
	for _, dep := range result.Dependencies {
		var mapping dtypes.ResourceTypeMapping
		var found bool

		// First try the local catalog
		if e.resourceCatalog != nil {
			entry, catalogFound := e.resourceCatalog.Lookup(dep.Type)
			if catalogFound {
				mapping = resourcetypes.Match(dep, entry)
				found = true
			}
		}

		// If not found in catalog, try contrib lookup
		if !found {
			contribType, err := resourcetypes.LookupFromContrib(ctx, dep.Type)
			if err == nil && contribType != nil {
				entry := contribType.ToResourceTypeEntry(dep.Type)
				mapping = resourcetypes.Match(dep, entry)
				found = true
			}
		}

		if found {
			result.ResourceTypes = append(result.ResourceTypes, mapping)
		}
	}

	// Calculate overall confidence
	result.Confidence = e.calculateConfidence(result)

	// Link dependencies to services
	e.linkDependenciesToServices(result)

	return result, nil
}

// DiscoverAndWrite runs discovery and writes the output.
func (e *Engine) DiscoverAndWrite(ctx context.Context, opts DiscoverOptions) (*DiscoveryResult, error) {
	result, err := e.Discover(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Write output files
	if opts.OutputPath != "" {
		// Write markdown output
		if err := e.markdownGen.GenerateToFile(result, opts.OutputPath); err != nil {
			return nil, fmt.Errorf("writing discovery output: %w", err)
		}

		// Also write JSON output for programmatic use
		jsonPath := strings.TrimSuffix(opts.OutputPath, ".md") + ".json"
		if err := e.writeJSONOutput(result, jsonPath); err != nil {
			return nil, fmt.Errorf("writing JSON output: %w", err)
		}
	}

	return result, nil
}

// writeJSONOutput writes the discovery result as JSON.
func (e *Engine) writeJSONOutput(result *DiscoveryResult, path string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (e *Engine) calculateConfidence(result *DiscoveryResult) float64 {
	if len(result.Dependencies) == 0 && len(result.Services) == 0 {
		return 0.0
	}

	var totalConfidence float64
	count := 0

	for _, svc := range result.Services {
		totalConfidence += svc.Confidence
		count++
	}

	for _, dep := range result.Dependencies {
		totalConfidence += dep.Confidence
		count++
	}

	if count == 0 {
		return 0.0
	}

	return totalConfidence / float64(count)
}

func (e *Engine) linkDependenciesToServices(result *DiscoveryResult) {
	// Create a map of dependency IDs by the files they were found in
	depByPath := make(map[string][]string)
	for _, dep := range result.Dependencies {
		for _, ev := range dep.Evidence {
			dir := filepath.Dir(ev.File)
			depByPath[dir] = append(depByPath[dir], dep.ID)
		}
	}

	// Link dependencies to services based on path proximity
	for i := range result.Services {
		svc := &result.Services[i]
		relPath, _ := filepath.Rel(result.ProjectPath, svc.Path)
		if relPath == "" {
			relPath = "."
		}

		// Find dependencies in the same directory or parent
		if deps, ok := depByPath[relPath]; ok {
			svc.DependencyIDs = append(svc.DependencyIDs, deps...)
		}
		if deps, ok := depByPath["."]; ok {
			svc.DependencyIDs = append(svc.DependencyIDs, deps...)
		}
	}

	// Also update UsedBy on dependencies
	svcByPath := make(map[string]string)
	for _, svc := range result.Services {
		relPath, _ := filepath.Rel(result.ProjectPath, svc.Path)
		svcByPath[relPath] = svc.Name
	}

	for i := range result.Dependencies {
		dep := &result.Dependencies[i]
		for _, ev := range dep.Evidence {
			dir := filepath.Dir(ev.File)
			if svcName, ok := svcByPath[dir]; ok {
				dep.UsedBy = append(dep.UsedBy, svcName)
			}
		}
	}
}

// detectTeamPractices scans for IaC files and extracts team practices.
func (e *Engine) detectTeamPractices(projectPath string) *dtypes.TeamPractices {
	var allPractices []*practices.TeamPractices

	// Try Terraform parser
	tfParser := practices.NewTerraformParser(projectPath)
	if tfPractices, err := tfParser.Parse(); err == nil && tfPractices != nil {
		allPractices = append(allPractices, tfPractices)
	}

	// Try Bicep parser
	bicepParser := practices.NewBicepParser(projectPath)
	if bicepPractices, err := bicepParser.Parse(); err == nil && bicepPractices != nil {
		allPractices = append(allPractices, bicepPractices)
	}

	// Try loading from config file
	configPath := filepath.Join(projectPath, ".radius", "team-practices.yaml")
	if _, err := os.Stat(configPath); err == nil {
		if cfg, err := practices.LoadConfigFromFile(configPath); err == nil && cfg != nil {
			allPractices = append(allPractices, &cfg.Practices)
		}
	}

	if len(allPractices) == 0 {
		return nil
	}

	// Merge all practices (later sources take precedence)
	merged := allPractices[0]
	for i := 1; i < len(allPractices); i++ {
		merged.Merge(allPractices[i])
	}

	// Convert to dtypes.TeamPractices
	return convertPractices(merged)
}

// convertPractices converts practices.TeamPractices to dtypes.TeamPractices.
func convertPractices(p *practices.TeamPractices) *dtypes.TeamPractices {
	if p == nil {
		return nil
	}

	result := &dtypes.TeamPractices{
		Tags:              p.Tags,
		Environment:       p.Environment,
		Region:            p.Region,
		EncryptionEnabled: p.Security.EncryptionEnabled,
		PrivateNetworking: p.Security.PrivateNetworking,
		DefaultTier:       p.Sizing.DefaultTier,
	}

	if p.NamingConvention != nil {
		result.NamingConvention = dtypes.NamingPattern{
			Pattern:    p.NamingConvention.Pattern,
			Examples:   p.NamingConvention.Examples,
			Confidence: p.NamingConvention.Confidence,
		}
	}

	// Convert sources
	for _, src := range p.Sources {
		// Convert resources
		var resources []dtypes.IaCResource
		for _, r := range src.Resources {
			resources = append(resources, dtypes.IaCResource{
				Type: r.Type,
				Name: r.Name,
			})
		}

		result.ExtractedFrom = append(result.ExtractedFrom, dtypes.PracticeSource{
			Type:      dtypes.PracticeSourceType(src.Type),
			FilePath:  src.FilePath,
			Resources: resources,
			Providers: src.Providers,
		})
	}

	return result
}

// enrichWithDockerInfo parses Dockerfile and docker-compose files to enrich discovery results.
func (e *Engine) enrichWithDockerInfo(ctx context.Context, projectPath string, result *DiscoveryResult) {
	// Parse Docker Compose files
	composeParser := analyzers.NewComposeParser()
	composeFiles, _ := composeParser.FindComposeFiles(ctx, projectPath)

	// Collect compose info for bundled service detection
	var allComposeServices []analyzers.ComposeService

	for _, composePath := range composeFiles {
		composeInfo, err := composeParser.Parse(ctx, composePath)
		if err != nil {
			result.Warnings = append(result.Warnings, DiscoveryWarning{
				Level:   WarningWarning,
				Code:    "COMPOSE_PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse compose file %s: %v", composePath, err),
			})
			continue
		}

		// Collect all services for bundled detection
		allComposeServices = append(allComposeServices, composeInfo.Services...)

		// Enrich services with compose info
		e.enrichServicesFromCompose(composeInfo, result)

		// Add infrastructure services as dependencies
		e.addInfrastructureFromCompose(composeInfo, result)

		// Record compose file as evidence
		result.Practices.ExtractedFrom = append(result.Practices.ExtractedFrom, PracticeSource{
			Type:     PracticeSourceType("compose"),
			FilePath: composePath,
		})
	}

	// Parse Dockerfiles
	dockerfileParser := analyzers.NewDockerfileParser()
	dockerfiles, _ := dockerfileParser.FindDockerfiles(ctx, projectPath)

	for _, dockerfilePath := range dockerfiles {
		dockerInfo, err := dockerfileParser.Parse(ctx, dockerfilePath)
		if err != nil {
			result.Warnings = append(result.Warnings, DiscoveryWarning{
				Level:   WarningWarning,
				Code:    "DOCKERFILE_PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse Dockerfile %s: %v", dockerfilePath, err),
			})
			continue
		}

		// Enrich services with dockerfile info
		e.enrichServicesFromDockerfile(dockerInfo, projectPath, result)

		// Detect bundled services from COPY --from instructions
		if len(dockerInfo.BundledStages) > 0 && len(allComposeServices) > 0 {
			bundledMap := dockerfileParser.DetectBundledServices(dockerInfo, allComposeServices)
			e.markBundledServices(bundledMap, result)
		}
	}
}

// enrichServicesFromCompose enriches detected services with compose file information.
func (e *Engine) enrichServicesFromCompose(compose *analyzers.ComposeInfo, result *DiscoveryResult) {
	for _, composeSvc := range compose.GetApplicationServices() {
		// Try to match with existing services by name
		matched := false
		for i := range result.Services {
			svc := &result.Services[i]
			if strings.EqualFold(svc.Name, composeSvc.Name) {
				// Enrich with compose info
				if len(composeSvc.Ports) > 0 && len(svc.ExposedPorts) == 0 {
					svc.ExposedPorts = composeSvc.Ports
				}
				if composeSvc.Build != nil && composeSvc.Build.Dockerfile != "" {
					svc.Dockerfile = composeSvc.Build.Dockerfile
				}
				// Add depends_on as evidence of dependencies
				svc.Evidence = append(svc.Evidence, Evidence{
					Type:    EvidenceImport,
					File:    compose.Path,
					Snippet: fmt.Sprintf("compose service: %s", composeSvc.Name),
				})
				matched = true
				break
			}
		}

		// If not matched, add as new service
		if !matched && composeSvc.Build != nil {
			newSvc := Service{
				Name:         composeSvc.Name,
				Path:         filepath.Join(filepath.Dir(compose.Path), composeSvc.Build.Context),
				ExposedPorts: composeSvc.Ports,
				Confidence:   0.75,
				Evidence: []Evidence{
					{
						Type:    EvidenceImport,
						File:    compose.Path,
						Snippet: fmt.Sprintf("compose service with build context: %s", composeSvc.Build.Context),
					},
				},
			}
			if composeSvc.Build.Target != "" {
				newSvc.EntryPoint = EntryPoint{
					Type: EntryPointDockerfile,
					File: composeSvc.Build.Dockerfile,
				}
			}
			result.Services = append(result.Services, newSvc)
		}
	}
}

// addInfrastructureFromCompose adds infrastructure services as dependencies.
func (e *Engine) addInfrastructureFromCompose(compose *analyzers.ComposeInfo, result *DiscoveryResult) {
	for _, infraSvc := range compose.GetInfrastructureServices() {
		// Check if we already detected this dependency
		alreadyDetected := false
		for i := range result.Dependencies {
			dep := &result.Dependencies[i]
			if strings.EqualFold(string(dep.Type), infraSvc.InfrastructureType) {
				// Add compose evidence
				dep.Evidence = append(dep.Evidence, Evidence{
					Type:    EvidenceImport,
					File:    compose.Path,
					Snippet: fmt.Sprintf("compose service: %s (image: %s)", infraSvc.Name, infraSvc.Image),
				})
				alreadyDetected = true
				break
			}
		}

		if !alreadyDetected {
			// Add as new dependency
			depType := dtypes.DependencyType(infraSvc.InfrastructureType)
			newDep := DetectedDependency{
				ID:         fmt.Sprintf("%s-%d", infraSvc.InfrastructureType, len(result.Dependencies)+1),
				Type:       depType,
				Name:       infraSvc.Name,
				Library:    infraSvc.Image,
				Confidence: 0.9, // High confidence from explicit compose definition
				Evidence: []Evidence{
					{
						Type:    EvidenceImport,
						File:    compose.Path,
						Snippet: fmt.Sprintf("compose service: %s (image: %s)", infraSvc.Name, infraSvc.Image),
					},
				},
			}

			result.Dependencies = append(result.Dependencies, newDep)

			// Also add resource type mapping if catalog is available
			if e.resourceCatalog != nil {
				if entry, found := e.resourceCatalog.Lookup(depType); found {
					mapping := resourcetypes.Match(newDep, entry)
					result.ResourceTypes = append(result.ResourceTypes, mapping)
				}
			}
		}
	}
}

// enrichServicesFromDockerfile enriches services with Dockerfile information.
func (e *Engine) enrichServicesFromDockerfile(dockerfile *analyzers.DockerfileInfo, projectPath string, result *DiscoveryResult) {
	dockerfileDir := filepath.Dir(dockerfile.Path)
	relPath, _ := filepath.Rel(projectPath, dockerfileDir)
	if relPath == "" || relPath == "." {
		relPath = projectPath
	}

	// Try to match with existing services by path
	for i := range result.Services {
		svc := &result.Services[i]
		svcRel, _ := filepath.Rel(projectPath, svc.Path)

		// Match if dockerfile is in same directory or service directory
		if svcRel == relPath || strings.HasPrefix(relPath, svcRel) || dockerfileDir == projectPath {
			// Enrich with dockerfile info
			if svc.Dockerfile == "" {
				svc.Dockerfile = dockerfile.Path
			}
			if len(dockerfile.ExposedPorts) > 0 && len(svc.ExposedPorts) == 0 {
				svc.ExposedPorts = dockerfile.ExposedPorts
			}
			if svc.EntryPoint.Type == "" {
				svc.EntryPoint = dockerfile.ToEntryPoint()
			}
			svc.Evidence = append(svc.Evidence, Evidence{
				Type:    EvidenceImport,
				File:    dockerfile.Path,
				Snippet: fmt.Sprintf("Dockerfile with base: %s", dockerfile.BaseImage),
			})
			return
		}
	}

	// If no match and dockerfile is in a subdirectory, add as new service
	if relPath != "." && relPath != projectPath {
		serviceName := filepath.Base(dockerfileDir)
		result.Services = append(result.Services, Service{
			Name:         serviceName,
			Path:         dockerfileDir,
			Dockerfile:   dockerfile.Path,
			ExposedPorts: dockerfile.ExposedPorts,
			EntryPoint:   dockerfile.ToEntryPoint(),
			Confidence:   0.7,
			Evidence: []Evidence{
				{
					Type:    EvidenceImport,
					File:    dockerfile.Path,
					Snippet: fmt.Sprintf("Dockerfile detected with base: %s", dockerfile.BaseImage),
				},
			},
		})
	}
}

// markBundledServices updates services with bundling relationships.
// bundledMap maps service names to the service they are bundled into.
func (e *Engine) markBundledServices(bundledMap map[string]string, result *DiscoveryResult) {
	for serviceName, bundledInto := range bundledMap {
		// Mark the bundled service
		for i := range result.Services {
			if strings.EqualFold(result.Services[i].Name, serviceName) {
				result.Services[i].IsBundledInto = bundledInto
				result.Services[i].Evidence = append(result.Services[i].Evidence, Evidence{
					Type:    EvidenceImport,
					File:    "Dockerfile",
					Snippet: fmt.Sprintf("Bundled into %s via multi-stage build (COPY --from)", bundledInto),
				})
				break
			}
		}

		// Mark the target service that bundles others
		for i := range result.Services {
			if strings.EqualFold(result.Services[i].Name, bundledInto) {
				// Avoid duplicates
				alreadyListed := false
				for _, existing := range result.Services[i].BundlesServices {
					if strings.EqualFold(existing, serviceName) {
						alreadyListed = true
						break
					}
				}
				if !alreadyListed {
					result.Services[i].BundlesServices = append(result.Services[i].BundlesServices, serviceName)
				}
				break
			}
		}
	}
}
