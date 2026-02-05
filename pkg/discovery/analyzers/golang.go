package analyzers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/analyzers/manifest"
	"github.com/radius-project/radius/pkg/discovery/catalog"
)

// GoAnalyzer analyzes Go projects.
type GoAnalyzer struct {
	catalog *catalog.LibraryCatalog
}

// NewGoAnalyzer creates a new Go analyzer.
func NewGoAnalyzer(cat *catalog.LibraryCatalog) *GoAnalyzer {
	return &GoAnalyzer{catalog: cat}
}

// Name returns the analyzer identifier.
func (a *GoAnalyzer) Name() string {
	return "go"
}

// Languages returns supported languages.
func (a *GoAnalyzer) Languages() []dtypes.Language {
	return []dtypes.Language{dtypes.LanguageGo}
}

// CanAnalyze checks if this analyzer can process the project.
func (a *GoAnalyzer) CanAnalyze(ctx context.Context, projectPath string) (bool, error) {
	files, err := manifest.FindGoMod(projectPath)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// Analyze scans the project for dependencies.
func (a *GoAnalyzer) Analyze(ctx context.Context, projectPath string, opts AnalyzeOptions) (*AnalyzeResult, error) {
	result := NewAnalyzeResult(dtypes.LanguageGo)

	goModFiles, err := manifest.FindGoMod(projectPath)
	if err != nil {
		return nil, fmt.Errorf("finding go.mod files: %w", err)
	}

	depIDCounter := 0
	generateDepID := func(depType dtypes.DependencyType) string {
		depIDCounter++
		return fmt.Sprintf("%s-%d", depType, depIDCounter)
	}

	for _, modPath := range goModFiles {
		goMod, err := manifest.ParseGoMod(modPath)
		if err != nil {
			result.AddWarning(dtypes.DiscoveryWarning{
				Level:   dtypes.WarningWarning,
				Code:    "PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse %s: %v", modPath, err),
				File:    modPath,
			})
			continue
		}

		// Detect service from module name
		dir := filepath.Dir(modPath)
		svcName := filepath.Base(goMod.Module)
		if svcName == "" {
			svcName = filepath.Base(dir)
		}

		svc := dtypes.Service{
			Name:     svcName,
			Path:     dir,
			Language: dtypes.LanguageGo,
			Evidence: []dtypes.Evidence{
				{
					Type:    dtypes.EvidencePackageManifest,
					File:    modPath,
					Line:    1,
					Snippet: fmt.Sprintf("module %s", goMod.Module),
				},
			},
			Confidence: 0.8,
		}

		result.AddService(svc)

		// Analyze direct dependencies only
		relPath, _ := filepath.Rel(projectPath, modPath)

		for _, req := range goMod.DirectDependencies() {
			entry, found := a.catalog.Lookup(req.Path, dtypes.LanguageGo)
			if !found {
				continue
			}

			if entry.Confidence < opts.MinConfidence {
				continue
			}

			dep := dtypes.DetectedDependency{
				ID:            generateDepID(entry.DependencyType),
				Type:          entry.DependencyType,
				Name:          filepath.Base(req.Path),
				Library:       req.Path,
				Version:       req.Version,
				Confidence:    entry.Confidence,
				ConnectionEnv: entry.ConnectionEnvVar,
				DefaultPort:   entry.DefaultPort,
				Evidence: []dtypes.Evidence{
					{
						Type:    dtypes.EvidencePackageManifest,
						File:    relPath,
						Line:    1,
						Snippet: fmt.Sprintf("require %s %s", req.Path, req.Version),
					},
				},
			}

			result.AddDependency(dep)
		}
	}

	return result, nil
}

func init() {
	if catalog.DefaultCatalog != nil {
		_ = Register(NewGoAnalyzer(catalog.DefaultCatalog))
	}
}
