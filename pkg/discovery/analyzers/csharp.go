package analyzers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/analyzers/manifest"
	"github.com/radius-project/radius/pkg/discovery/catalog"
)

// CSharpAnalyzer analyzes C# projects.
type CSharpAnalyzer struct {
	catalog *catalog.LibraryCatalog
}

// NewCSharpAnalyzer creates a new C# analyzer.
func NewCSharpAnalyzer(cat *catalog.LibraryCatalog) *CSharpAnalyzer {
	return &CSharpAnalyzer{catalog: cat}
}

// Name returns the analyzer identifier.
func (a *CSharpAnalyzer) Name() string {
	return "csharp"
}

// Languages returns supported languages.
func (a *CSharpAnalyzer) Languages() []dtypes.Language {
	return []dtypes.Language{dtypes.LanguageCSharp}
}

// CanAnalyze checks if this analyzer can process the project.
func (a *CSharpAnalyzer) CanAnalyze(ctx context.Context, projectPath string) (bool, error) {
	files, err := manifest.FindCSProj(projectPath)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// Analyze scans the project for dependencies.
func (a *CSharpAnalyzer) Analyze(ctx context.Context, projectPath string, opts AnalyzeOptions) (*AnalyzeResult, error) {
	result := NewAnalyzeResult(dtypes.LanguageCSharp)

	csprojFiles, err := manifest.FindCSProj(projectPath)
	if err != nil {
		return nil, fmt.Errorf("finding .csproj files: %w", err)
	}

	depIDCounter := 0
	generateDepID := func(depType dtypes.DependencyType) string {
		depIDCounter++
		return fmt.Sprintf("%s-%d", depType, depIDCounter)
	}

	for _, projPath := range csprojFiles {
		proj, err := manifest.ParseCSProj(projPath)
		if err != nil {
			result.AddWarning(dtypes.DiscoveryWarning{
				Level:   dtypes.WarningWarning,
				Code:    "PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse %s: %v", projPath, err),
				File:    projPath,
			})
			continue
		}

		// Detect service from file name
		dir := filepath.Dir(projPath)
		svcName := strings.TrimSuffix(filepath.Base(projPath), ".csproj")

		svc := dtypes.Service{
			Name:     svcName,
			Path:     dir,
			Language: dtypes.LanguageCSharp,
			Evidence: []dtypes.Evidence{
				{
					Type:    dtypes.EvidencePackageManifest,
					File:    projPath,
					Line:    1,
					Snippet: filepath.Base(projPath),
				},
			},
			Confidence: 0.8,
		}

		// Detect framework
		if proj.IsWebProject() {
			svc.Framework = "ASP.NET Core"
			svc.Confidence = 0.9
		}

		// Detect entry point type
		if proj.IsConsoleProject() {
			svc.EntryPoint = dtypes.EntryPoint{
				Type: dtypes.EntryPointMain,
				File: "Program.cs",
			}
		}

		result.AddService(svc)

		// Analyze dependencies
		relPath, _ := filepath.Rel(projectPath, projPath)

		for _, ref := range proj.PackageReferences {
			entry, found := a.catalog.Lookup(ref.Include, dtypes.LanguageCSharp)
			if !found {
				continue
			}

			if entry.Confidence < opts.MinConfidence {
				continue
			}

			dep := dtypes.DetectedDependency{
				ID:            generateDepID(entry.DependencyType),
				Type:          entry.DependencyType,
				Name:          ref.Include,
				Library:       ref.Include,
				Version:       ref.Version,
				Confidence:    entry.Confidence,
				ConnectionEnv: entry.ConnectionEnvVar,
				DefaultPort:   entry.DefaultPort,
				Evidence: []dtypes.Evidence{
					{
						Type:    dtypes.EvidencePackageManifest,
						File:    relPath,
						Line:    1,
						Snippet: fmt.Sprintf(`<PackageReference Include="%s" Version="%s" />`, ref.Include, ref.Version),
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
		_ = Register(NewCSharpAnalyzer(catalog.DefaultCatalog))
	}
}
