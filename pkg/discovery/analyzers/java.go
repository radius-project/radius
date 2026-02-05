package analyzers

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
	"github.com/radius-project/radius/pkg/discovery/analyzers/manifest"
	"github.com/radius-project/radius/pkg/discovery/catalog"
)

// JavaAnalyzer analyzes Java projects using Maven.
type JavaAnalyzer struct {
	catalog *catalog.LibraryCatalog
}

// NewJavaAnalyzer creates a new Java analyzer.
func NewJavaAnalyzer(cat *catalog.LibraryCatalog) *JavaAnalyzer {
	return &JavaAnalyzer{catalog: cat}
}

// Name returns the analyzer identifier.
func (a *JavaAnalyzer) Name() string {
	return "java"
}

// Languages returns supported languages.
func (a *JavaAnalyzer) Languages() []dtypes.Language {
	return []dtypes.Language{dtypes.LanguageJava}
}

// CanAnalyze checks if this analyzer can process the project.
func (a *JavaAnalyzer) CanAnalyze(ctx context.Context, projectPath string) (bool, error) {
	files, err := manifest.FindPOM(projectPath)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

// Analyze scans the project for dependencies.
func (a *JavaAnalyzer) Analyze(ctx context.Context, projectPath string, opts AnalyzeOptions) (*AnalyzeResult, error) {
	result := NewAnalyzeResult(dtypes.LanguageJava)

	pomFiles, err := manifest.FindPOM(projectPath)
	if err != nil {
		return nil, fmt.Errorf("finding pom.xml files: %w", err)
	}

	depIDCounter := 0
	generateDepID := func(depType dtypes.DependencyType) string {
		depIDCounter++
		return fmt.Sprintf("%s-%d", depType, depIDCounter)
	}

	for _, pomPath := range pomFiles {
		pom, err := manifest.ParsePOM(pomPath)
		if err != nil {
			result.AddWarning(dtypes.DiscoveryWarning{
				Level:   dtypes.WarningWarning,
				Code:    "PARSE_ERROR",
				Message: fmt.Sprintf("Failed to parse %s: %v", pomPath, err),
				File:    pomPath,
			})
			continue
		}

		// Detect service from artifact
		dir := filepath.Dir(pomPath)
		svcName := pom.ArtifactID
		if svcName == "" {
			svcName = filepath.Base(dir)
		}

		svc := dtypes.Service{
			Name:     svcName,
			Path:     dir,
			Language: dtypes.LanguageJava,
			Evidence: []dtypes.Evidence{
				{
					Type:    dtypes.EvidencePackageManifest,
					File:    pomPath,
					Line:    1,
					Snippet: fmt.Sprintf("<artifactId>%s</artifactId>", pom.ArtifactID),
				},
			},
			Confidence: 0.8,
		}

		// Detect framework
		if pom.HasDependency("org.springframework.boot:spring-boot-starter") ||
			pom.HasDependency("org.springframework.boot:spring-boot-starter-web") {
			svc.Framework = "Spring Boot"
			svc.Confidence = 0.9
		} else if pom.HasDependency("io.quarkus:quarkus-core") {
			svc.Framework = "Quarkus"
			svc.Confidence = 0.9
		} else if pom.HasDependency("io.micronaut:micronaut-runtime") {
			svc.Framework = "Micronaut"
			svc.Confidence = 0.9
		}

		result.AddService(svc)

		// Analyze dependencies
		relPath, _ := filepath.Rel(projectPath, pomPath)

		for _, dep := range pom.Dependencies {
			fullName := dep.FullName()
			entry, found := a.catalog.Lookup(fullName, dtypes.LanguageJava)
			if !found {
				continue
			}

			if entry.Confidence < opts.MinConfidence {
				continue
			}

			version := pom.ResolveVersion(dep.Version)

			detectedDep := dtypes.DetectedDependency{
				ID:            generateDepID(entry.DependencyType),
				Type:          entry.DependencyType,
				Name:          dep.ArtifactID,
				Library:       fullName,
				Version:       version,
				Confidence:    entry.Confidence,
				ConnectionEnv: entry.ConnectionEnvVar,
				DefaultPort:   entry.DefaultPort,
				Evidence: []dtypes.Evidence{
					{
						Type:    dtypes.EvidencePackageManifest,
						File:    relPath,
						Line:    1,
						Snippet: fmt.Sprintf("<dependency>%s:%s</dependency>", fullName, version),
					},
				},
			}

			result.AddDependency(detectedDep)
		}
	}

	return result, nil
}

func init() {
	if catalog.DefaultCatalog != nil {
		_ = Register(NewJavaAnalyzer(catalog.DefaultCatalog))
	}
}
