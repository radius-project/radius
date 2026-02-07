package manifest

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CSProj represents a .NET .csproj file.
type CSProj struct {
	XMLName           xml.Name            `xml:"Project"`
	Sdk               string              `xml:"Sdk,attr"`
	PropertyGroups    []PropertyGroup     `xml:"PropertyGroup"`
	ItemGroups        []ItemGroup         `xml:"ItemGroup"`
	PackageReferences []PackageReference  `xml:"-"` // Flattened from ItemGroups
	ProjectReferences []ProjectReference  `xml:"-"` // Flattened from ItemGroups
}

// PropertyGroup represents a PropertyGroup in csproj.
type PropertyGroup struct {
	TargetFramework         string `xml:"TargetFramework"`
	TargetFrameworks        string `xml:"TargetFrameworks"`
	OutputType              string `xml:"OutputType"`
	RootNamespace           string `xml:"RootNamespace"`
	AssemblyName            string `xml:"AssemblyName"`
	ImplicitUsings          string `xml:"ImplicitUsings"`
	Nullable                string `xml:"Nullable"`
	IsPackable              string `xml:"IsPackable"`
	GenerateDocumentation   string `xml:"GenerateDocumentationFile"`
	Version                 string `xml:"Version"`
}

// ItemGroup represents an ItemGroup in csproj.
type ItemGroup struct {
	PackageReferences []PackageReference `xml:"PackageReference"`
	ProjectReferences []ProjectReference `xml:"ProjectReference"`
}

// PackageReference represents a NuGet package reference.
type PackageReference struct {
	Include string `xml:"Include,attr"`
	Version string `xml:"Version,attr"`
}

// ProjectReference represents a project reference.
type ProjectReference struct {
	Include string `xml:"Include,attr"`
}

// ParseCSProj parses a .csproj file.
func ParseCSProj(path string) (*CSProj, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading .csproj: %w", err)
	}

	var proj CSProj
	if err := xml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parsing .csproj: %w", err)
	}

	// Flatten PackageReferences and ProjectReferences from ItemGroups
	for _, ig := range proj.ItemGroups {
		proj.PackageReferences = append(proj.PackageReferences, ig.PackageReferences...)
		proj.ProjectReferences = append(proj.ProjectReferences, ig.ProjectReferences...)
	}

	return &proj, nil
}

// FindCSProj searches for .csproj files in the project.
func FindCSProj(projectPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			name := info.Name()
			if name == "bin" || name == "obj" || name == ".git" ||
				name == "packages" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(info.Name(), ".csproj") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// HasPackage checks if a NuGet package is referenced.
func (p *CSProj) HasPackage(name string) bool {
	name = strings.ToLower(name)
	for _, ref := range p.PackageReferences {
		if strings.ToLower(ref.Include) == name {
			return true
		}
	}
	return false
}

// PackageVersion returns the version of a referenced package.
func (p *CSProj) PackageVersion(name string) string {
	name = strings.ToLower(name)
	for _, ref := range p.PackageReferences {
		if strings.ToLower(ref.Include) == name {
			return ref.Version
		}
	}
	return ""
}

// AllPackages returns all packages as a map.
func (p *CSProj) AllPackages() map[string]string {
	result := make(map[string]string)
	for _, ref := range p.PackageReferences {
		result[ref.Include] = ref.Version
	}
	return result
}

// TargetFramework returns the target framework(s).
func (p *CSProj) TargetFramework() string {
	for _, pg := range p.PropertyGroups {
		if pg.TargetFramework != "" {
			return pg.TargetFramework
		}
		if pg.TargetFrameworks != "" {
			return pg.TargetFrameworks
		}
	}
	return ""
}

// IsWebProject checks if this is likely a web project.
func (p *CSProj) IsWebProject() bool {
	sdk := strings.ToLower(p.Sdk)
	return strings.Contains(sdk, "web") ||
		p.HasPackage("Microsoft.AspNetCore.App") ||
		p.HasPackage("Microsoft.NET.Sdk.Web")
}

// IsConsoleProject checks if this is a console application.
func (p *CSProj) IsConsoleProject() bool {
	for _, pg := range p.PropertyGroups {
		if strings.ToLower(pg.OutputType) == "exe" {
			return true
		}
	}
	return false
}
