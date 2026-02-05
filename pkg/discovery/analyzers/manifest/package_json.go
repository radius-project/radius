// Package manifest provides parsers for language-specific package manifest files.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PackageJSON represents a Node.js package.json file.
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Main            string            `json:"main,omitempty"`
	Scripts         map[string]string `json:"scripts,omitempty"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
}

// ParsePackageJSON parses a package.json file.
func ParsePackageJSON(path string) (*PackageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading package.json: %w", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}

	return &pkg, nil
}

// FindPackageJSON searches for package.json files in the project.
func FindPackageJSON(projectPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == "package.json" {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// AllDependencies returns all dependencies (prod + dev if requested).
func (p *PackageJSON) AllDependencies(includeDevDeps bool) map[string]string {
	deps := make(map[string]string)

	for name, version := range p.Dependencies {
		deps[name] = version
	}

	if includeDevDeps {
		for name, version := range p.DevDependencies {
			deps[name] = version
		}
	}

	return deps
}

// HasDependency checks if a dependency exists.
func (p *PackageJSON) HasDependency(name string) bool {
	_, inDeps := p.Dependencies[name]
	_, inDevDeps := p.DevDependencies[name]
	return inDeps || inDevDeps
}

// DependencyVersion returns the version of a dependency, or empty string if not found.
func (p *PackageJSON) DependencyVersion(name string) string {
	if v, ok := p.Dependencies[name]; ok {
		return v
	}
	if v, ok := p.DevDependencies[name]; ok {
		return v
	}
	return ""
}
