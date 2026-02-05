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

// PythonAnalyzer analyzes Python projects.
type PythonAnalyzer struct {
	catalog *catalog.LibraryCatalog
}

// NewPythonAnalyzer creates a new Python analyzer.
func NewPythonAnalyzer(cat *catalog.LibraryCatalog) *PythonAnalyzer {
	return &PythonAnalyzer{catalog: cat}
}

// Name returns the analyzer identifier.
func (a *PythonAnalyzer) Name() string {
	return "python"
}

// Languages returns supported languages.
func (a *PythonAnalyzer) Languages() []dtypes.Language {
	return []dtypes.Language{dtypes.LanguagePython}
}

// CanAnalyze checks if this analyzer can process the project.
func (a *PythonAnalyzer) CanAnalyze(ctx context.Context, projectPath string) (bool, error) {
	files, err := manifest.FindRequirementsTxt(projectPath)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// Analyze scans the project for dependencies.
func (a *PythonAnalyzer) Analyze(ctx context.Context, projectPath string, opts AnalyzeOptions) (*AnalyzeResult, error) {
	result := NewAnalyzeResult(dtypes.LanguagePython)

	reqFiles, err := manifest.FindRequirementsTxt(projectPath)
	if err != nil {
		return nil, fmt.Errorf("finding requirements.txt files: %w", err)
	}

	depIDCounter := 0
	generateDepID := func(depType dtypes.DependencyType) string {
		depIDCounter++
		return fmt.Sprintf("%s-%d", depType, depIDCounter)
	}

	for _, reqPath := range reqFiles {
		reqs, err := manifest.ParseRequirementsTxt(reqPath)
		if err != nil {
			result.AddWarning(dtypes.DiscoveryWarning{
				Level:   dtypes.WarningWarning,
				Code:    "PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse %s: %v", reqPath, err),
				File:    reqPath,
			})
			continue
		}

		// Detect service from directory
		dir := filepath.Dir(reqPath)
		svcName := filepath.Base(dir)
		if svcName == "." || svcName == projectPath {
			svcName = filepath.Base(projectPath)
		}

		svc := dtypes.Service{
			Name:     svcName,
			Path:     dir,
			Language: dtypes.LanguagePython,
			Evidence: []dtypes.Evidence{
				{
					Type:    dtypes.EvidencePackageManifest,
					File:    reqPath,
					Line:    1,
					Snippet: "requirements.txt",
				},
			},
			Confidence: 0.7,
		}

		// Detect framework
		if reqs.HasPackage("flask") {
			svc.Framework = "Flask"
			svc.Confidence = 0.85
		} else if reqs.HasPackage("django") {
			svc.Framework = "Django"
			svc.Confidence = 0.85
		} else if reqs.HasPackage("fastapi") {
			svc.Framework = "FastAPI"
			svc.Confidence = 0.85
		}

		result.AddService(svc)

		// Analyze dependencies
		relPath, _ := filepath.Rel(projectPath, reqPath)

		for _, req := range reqs.Requirements {
			// Normalize package name for lookup
			pkgName := strings.ReplaceAll(strings.ToLower(req.Name), "-", "_")

			// Try multiple variations
			var entry catalog.LibraryEntry
			var found bool

			entry, found = a.catalog.Lookup(req.Name, dtypes.LanguagePython)
			if !found {
				entry, found = a.catalog.Lookup(pkgName, dtypes.LanguagePython)
			}

			if !found {
				continue
			}

			if entry.Confidence < opts.MinConfidence {
				continue
			}

			dep := dtypes.DetectedDependency{
				ID:            generateDepID(entry.DependencyType),
				Type:          entry.DependencyType,
				Name:          req.Name,
				Library:       req.Name,
				Version:       req.Version,
				Confidence:    entry.Confidence,
				ConnectionEnv: entry.ConnectionEnvVar,
				DefaultPort:   entry.DefaultPort,
				Evidence: []dtypes.Evidence{
					{
						Type:    dtypes.EvidencePackageManifest,
						File:    relPath,
						Line:    1,
						Snippet: fmt.Sprintf("%s%s", req.Name, req.Version),
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
		_ = Register(NewPythonAnalyzer(catalog.DefaultCatalog))
	}
}
