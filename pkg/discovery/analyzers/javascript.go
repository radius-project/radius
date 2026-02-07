package analyzers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/analyzers/manifest"
	"github.com/radius-project/radius/pkg/discovery/catalog"
)

// JavaScriptAnalyzer analyzes JavaScript/TypeScript projects.
type JavaScriptAnalyzer struct {
	catalog *catalog.LibraryCatalog
}

// NewJavaScriptAnalyzer creates a new JavaScript analyzer.
func NewJavaScriptAnalyzer(cat *catalog.LibraryCatalog) *JavaScriptAnalyzer {
	return &JavaScriptAnalyzer{catalog: cat}
}

// Name returns the analyzer identifier.
func (a *JavaScriptAnalyzer) Name() string {
	return "javascript"
}

// Languages returns supported languages.
func (a *JavaScriptAnalyzer) Languages() []dtypes.Language {
	return []dtypes.Language{dtypes.LanguageJavaScript, dtypes.LanguageTypeScript}
}

// CanAnalyze checks if this analyzer can process the project.
func (a *JavaScriptAnalyzer) CanAnalyze(ctx context.Context, projectPath string) (bool, error) {
	files, err := manifest.FindPackageJSON(projectPath)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// Analyze scans the project for dependencies.
func (a *JavaScriptAnalyzer) Analyze(ctx context.Context, projectPath string, opts AnalyzeOptions) (*AnalyzeResult, error) {
	result := NewAnalyzeResult(dtypes.LanguageJavaScript)

	packageJSONFiles, err := manifest.FindPackageJSON(projectPath)
	if err != nil {
		return nil, fmt.Errorf("finding package.json files: %w", err)
	}

	depIDCounter := 0
	generateDepID := func(depType dtypes.DependencyType) string {
		depIDCounter++
		return fmt.Sprintf("%s-%d", depType, depIDCounter)
	}

	for _, pkgPath := range packageJSONFiles {
		pkg, err := manifest.ParsePackageJSON(pkgPath)
		if err != nil {
			result.AddWarning(dtypes.DiscoveryWarning{
				Level:   dtypes.WarningWarning,
				Code:    "PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse %s: %v", pkgPath, err),
				File:    pkgPath,
			})
			continue
		}

		// Determine language (TypeScript vs JavaScript)
		lang := dtypes.LanguageJavaScript
		if _, hasTS := pkg.Dependencies["typescript"]; hasTS {
			lang = dtypes.LanguageTypeScript
		}
		if _, hasTS := pkg.DevDependencies["typescript"]; hasTS {
			lang = dtypes.LanguageTypeScript
		}
		result.Language = lang

		// Detect service
		if pkg.Name != "" {
			svc := dtypes.Service{
				Name:     pkg.Name,
				Path:     filepath.Dir(pkgPath),
				Language: lang,
				Evidence: []dtypes.Evidence{
					{
						Type:    dtypes.EvidencePackageManifest,
						File:    pkgPath,
						Line:    1,
						Snippet: fmt.Sprintf(`"name": "%s"`, pkg.Name),
					},
				},
				Confidence: 0.8,
			}

			// Detect entry point
			if pkg.Main != "" {
				svc.EntryPoint = dtypes.EntryPoint{
					Type: dtypes.EntryPointMain,
					File: pkg.Main,
				}
			}

			// Check for start script
			if startScript, ok := pkg.Scripts["start"]; ok {
				svc.EntryPoint.Command = startScript
			}

			result.AddService(svc)
		}

		// Analyze dependencies
		deps := pkg.AllDependencies(opts.IncludeDevDeps)
		relPath, _ := filepath.Rel(projectPath, pkgPath)

		for libName, version := range deps {
			entry, found := a.catalog.Lookup(libName, dtypes.LanguageJavaScript)
			if !found {
				continue
			}

			if entry.Confidence < opts.MinConfidence {
				continue
			}

			dep := dtypes.DetectedDependency{
				ID:            generateDepID(entry.DependencyType),
				Type:          entry.DependencyType,
				Name:          libName,
				Library:       libName,
				Version:       version,
				Confidence:    entry.Confidence,
				ConnectionEnv: entry.ConnectionEnvVar,
				DefaultPort:   entry.DefaultPort,
				Evidence: []dtypes.Evidence{
					{
						Type:    dtypes.EvidencePackageManifest,
						File:    relPath,
						Line:    1,
						Snippet: fmt.Sprintf(`"%s": "%s"`, libName, version),
					},
				},
			}

			result.AddDependency(dep)
		}
	}

	return result, nil
}

func init() {
	// Register with default registry when catalog is available
	if catalog.DefaultCatalog != nil {
		_ = Register(NewJavaScriptAnalyzer(catalog.DefaultCatalog))
	}
}
