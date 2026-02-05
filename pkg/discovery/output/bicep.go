// Package output provides output generation for discovery results.
package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/output/templates"
	"github.com/radius-project/radius/pkg/discovery/practices"
)

// BicepGenerator generates app.bicep output.
type BicepGenerator struct {
	appTmpl       *template.Template
	containerTmpl *template.Template
	resourceTmpl  *template.Template
	practices     *practices.TeamPractices
}

// NewBicepGenerator creates a new Bicep generator.
func NewBicepGenerator() (*BicepGenerator, error) {
	funcMap := template.FuncMap{
		"lower":        strings.ToLower,
		"upper":        strings.ToUpper,
		"title":        strings.Title, //nolint:staticcheck
		"safeName":     safeBicepName,
		"resourceName": resourceBicepName,
		"quote":        func(s string) string { return fmt.Sprintf("'%s'", s) },
	}

	appTmpl, err := template.New("application.bicep").Funcs(funcMap).Parse(applicationTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing application template: %w", err)
	}

	containerTmpl, err := template.New("container.bicep").Funcs(funcMap).Parse(containerTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing container template: %w", err)
	}

	resourceTmpl, err := template.New("resource.bicep").Funcs(funcMap).Parse(resourceTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing resource template: %w", err)
	}

	return &BicepGenerator{
		appTmpl:       appTmpl,
		containerTmpl: containerTmpl,
		resourceTmpl:  resourceTmpl,
	}, nil
}

// BicepGenerateOptions configures Bicep generation.
type BicepGenerateOptions struct {
	ApplicationName string
	Environment     string
	IncludeComments bool
	IncludeRecipes  bool
	Practices       *practices.TeamPractices
}

// WithPractices sets the team practices for the generator.
func (g *BicepGenerator) WithPractices(p *practices.TeamPractices) *BicepGenerator {
	g.practices = p
	return g
}

// ApplyNamingConvention applies the team naming convention to a resource name.
func (g *BicepGenerator) ApplyNamingConvention(name string, resourceType string) string {
	if g.practices == nil || g.practices.NamingConvention == nil || g.practices.NamingConvention.Pattern == "" {
		return name
	}

	values := map[string]string{
		"name":     name,
		"resource": resourceType,
	}
	if g.practices.Environment != "" {
		values["env"] = g.practices.Environment
	}

	result := g.practices.NamingConvention.ApplyNamingPattern(values)
	if result == "" {
		return name
	}
	return result
}

// GetRequiredTags returns the required tags from team practices.
func (g *BicepGenerator) GetRequiredTags() map[string]string {
	if g.practices == nil {
		return nil
	}
	return g.practices.Tags
}

// Generate writes Bicep output from discovery results.
func (g *BicepGenerator) Generate(result *dtypes.DiscoveryResult, opts BicepGenerateOptions, w io.Writer) error {
	// Apply practices from options if provided
	if opts.Practices != nil {
		g.practices = opts.Practices
	}

	// Prepare template data
	data := &bicepTemplateData{
		ApplicationName: opts.ApplicationName,
		Environment:     opts.Environment,
		IncludeComments: opts.IncludeComments,
		IncludeRecipes:  opts.IncludeRecipes,
		Services:        result.Services,
		Dependencies:    result.Dependencies,
		ResourceTypes:   result.ResourceTypes,
		Recipes:         result.Recipes,
		Practices:       g.practices,
	}

	// Set defaults
	if data.ApplicationName == "" {
		data.ApplicationName = filepath.Base(result.ProjectPath)
	}
	if data.Environment == "" {
		data.Environment = "default"
	}

	// Build Bicep content
	var buf bytes.Buffer

	// Write header comment if enabled
	if opts.IncludeComments {
		buf.WriteString(bicepHeader)
	}

	// Write extension imports - always include radius
	buf.WriteString("extension radius\n")

	// Add custom extension imports for non-built-in resource types
	// Per https://docs.radapp.io/tutorials/create-resource-type/
	customExtensions := getCustomExtensions(result.ResourceTypes)
	for _, ext := range customExtensions {
		buf.WriteString(fmt.Sprintf("extension %s\n", ext))
	}
	buf.WriteString("\n")

	// Write parameters
	buf.WriteString("@description('The Radius environment to deploy to')\n")
	buf.WriteString("param environment string\n\n")
	buf.WriteString("@description('The application name')\n")
	buf.WriteString(fmt.Sprintf("param applicationName string = '%s'\n\n", data.ApplicationName))

	// Write application resource
	if err := g.appTmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("generating application resource: %w", err)
	}

	// Track which dependencies have been processed
	processedDeps := make(map[string]bool)

	// Write infrastructure resources from explicit resource type mappings
	for _, rt := range result.ResourceTypes {
		dep := findDependency(result.Dependencies, rt.DependencyID)
		if dep == nil {
			continue
		}

		processedDeps[rt.DependencyID] = true

		resourceData := &bicepResourceData{
			DependencyID:    rt.DependencyID,
			Dependency:      dep,
			ResourceType:    &rt,
			IncludeComments: opts.IncludeComments,
		}

		if err := g.resourceTmpl.Execute(&buf, resourceData); err != nil {
			return fmt.Errorf("generating resource for %s: %w", rt.DependencyID, err)
		}
	}

	// Generate resources for dependencies without explicit mappings
	for _, dep := range result.Dependencies {
		if processedDeps[dep.ID] {
			continue
		}

		// Look up the resource type from dependency type
		depType := templates.GetDependencyByName(string(dep.Type))
		if depType == nil {
			// Unknown dependency type, skip
			continue
		}

		rt := dtypes.ResourceTypeMapping{
			DependencyID: dep.ID,
			ResourceType: dtypes.ResourceType{
				Name:       depType.ResourceType,
				APIVersion: depType.APIVersion,
			},
		}

		resourceData := &bicepResourceData{
			DependencyID:    dep.ID,
			Dependency:      &dep,
			ResourceType:    &rt,
			IncludeComments: opts.IncludeComments,
		}

		if err := g.resourceTmpl.Execute(&buf, resourceData); err != nil {
			return fmt.Errorf("generating resource for %s: %w", dep.ID, err)
		}
	}

	// Write container resources
	for _, svc := range result.Services {
		// Skip services that are bundled into other services
		// (e.g., frontend bundled into backend via multi-stage build)
		if svc.IsBundledInto != "" {
			continue
		}

		containerData := &bicepContainerData{
			Service:         &svc,
			Dependencies:    result.Dependencies,
			IncludeComments: opts.IncludeComments,
		}

		if err := g.containerTmpl.Execute(&buf, containerData); err != nil {
			return fmt.Errorf("generating container for %s: %w", svc.Name, err)
		}
	}

	// Write output
	_, err := w.Write(buf.Bytes())
	return err
}

// GenerateToFile writes Bicep output to a file.
func (g *BicepGenerator) GenerateToFile(result *dtypes.DiscoveryResult, opts BicepGenerateOptions, outputPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	var buf bytes.Buffer
	if err := g.Generate(result, opts, &buf); err != nil {
		return fmt.Errorf("generating bicep: %w", err)
	}

	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// Template data structures
type bicepTemplateData struct {
	ApplicationName string
	Environment     string
	IncludeComments bool
	IncludeRecipes  bool
	Services        []dtypes.Service
	Dependencies    []dtypes.DetectedDependency
	ResourceTypes   []dtypes.ResourceTypeMapping
	Recipes         []dtypes.RecipeMatch
	Practices       *practices.TeamPractices
}

type bicepContainerData struct {
	Service         *dtypes.Service
	Dependencies    []dtypes.DetectedDependency
	IncludeComments bool
}

type bicepResourceData struct {
	DependencyID    string
	Dependency      *dtypes.DetectedDependency
	ResourceType    *dtypes.ResourceTypeMapping
	IncludeComments bool
}

// Helper functions
func safeBicepName(name string) string {
	// Replace invalid characters with underscores
	result := strings.ReplaceAll(name, "-", "_")
	result = strings.ReplaceAll(result, ".", "_")
	result = strings.ReplaceAll(result, " ", "_")

	// Remove trailing _1, _2, etc. for cleaner naming convention
	// Following infrastructure naming practices: mysql, redis instead of mysql_1, redis_2
	if idx := strings.LastIndex(result, "_"); idx > 0 {
		suffix := result[idx+1:]
		// Check if suffix is just a number
		if _, err := strconv.Atoi(suffix); err == nil {
			result = result[:idx]
		}
	}

	return result
}

func resourceBicepName(dep dtypes.DetectedDependency) string {
	// Create a valid Bicep resource name from dependency
	name := strings.ToLower(string(dep.Type))
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func findDependency(deps []dtypes.DetectedDependency, id string) *dtypes.DetectedDependency {
	for i := range deps {
		if deps[i].ID == id {
			return &deps[i]
		}
	}
	return nil
}

// Bicep templates
const bicepHeader = `// ============================================================================
// Application Definition
// Generated by 'rad app generate' from discovery results
// 
// Review this file and customize as needed before deploying.
// See https://docs.radapp.io for more information.
// ============================================================================

`

const applicationTemplate = `
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: applicationName
  properties: {
    environment: environment
  }
}

`

const resourceTemplate = `{{ if .IncludeComments }}
// {{ .Dependency.Type }} resource: {{ .Dependency.Library }}
{{ end }}resource {{ safeName .DependencyID }} '{{ .ResourceType.ResourceType.Name }}@{{ .ResourceType.ResourceType.APIVersion }}' = {
  name: '{{ safeName .DependencyID }}'
  properties: {
    application: app.id
    environment: environment
  }
}

`

const containerTemplate = `{{ if .IncludeComments }}
// Container: {{ .Service.Name }}{{ if .Service.DependencyIDs }}
// Connections automatically inject environment variables for connected resources{{ end }}
{{ end }}resource {{ safeName .Service.Name }} 'Applications.Core/containers@2023-10-01-preview' = {
  name: '{{ .Service.Name }}'
  properties: {
    application: app.id
    container: {
      image: '{{ .Service.Name }}:latest' // TODO: Update with actual image
{{ if .Service.ExposedPorts }}      ports: {
{{ range $i, $port := .Service.ExposedPorts }}        http: {
          containerPort: {{ $port }}
        }
{{ end }}      }
{{ end }}    }{{ if .Service.DependencyIDs }}
    connections: {
{{ range $i, $depID := .Service.DependencyIDs }}      {{ safeName $depID }}: {
        source: {{ safeName $depID }}.id
      }
{{ end }}    }
{{ end }}  }
}

`

// getCustomExtensions returns the list of custom extension names needed for the resource types.
// Built-in Radius types (Applications.Core, Applications.Dapr, etc.) don't need custom extensions.
func getCustomExtensions(resourceTypes []dtypes.ResourceTypeMapping) []string {
	// Built-in namespaces that don't need custom extensions
	builtInNamespaces := map[string]bool{
		"Applications.Core":       true,
		"Applications.Dapr":       true,
		"Applications.Datastores": true,
		"Applications.Messaging":  true,
		"Microsoft.Resources":     true,
		"AWS":                     true,
	}

	// Collect unique custom namespaces
	customNamespaces := make(map[string]bool)
	for _, rt := range resourceTypes {
		parts := strings.Split(rt.ResourceType.Name, "/")
		if len(parts) == 2 {
			namespace := parts[0]
			if !builtInNamespaces[namespace] {
				// Extension name is namespace in lowercase without dots
				extensionName := strings.ToLower(strings.ReplaceAll(namespace, ".", ""))
				customNamespaces[extensionName] = true
			}
		}
	}

	// Convert to sorted slice
	var extensions []string
	for ext := range customNamespaces {
		extensions = append(extensions, ext)
	}
	return extensions
}
