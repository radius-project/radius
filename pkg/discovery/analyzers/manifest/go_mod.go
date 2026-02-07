package manifest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GoModule represents a Go module from go.mod.
type GoModule struct {
	Module      string
	GoVersion   string
	Require     []GoRequire
	Replace     map[string]string
	Exclude     []string
	Retract     []string
	IndirectMap map[string]bool
}

// GoRequire represents a require directive in go.mod.
type GoRequire struct {
	Path     string
	Version  string
	Indirect bool
}

// ParseGoMod parses a go.mod file.
func ParseGoMod(path string) (*GoModule, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening go.mod: %w", err)
	}
	defer file.Close()

	mod := &GoModule{
		Replace:     make(map[string]string),
		IndirectMap: make(map[string]bool),
	}

	scanner := bufio.NewScanner(file)
	var inRequire bool

	modulePattern := regexp.MustCompile(`^module\s+(.+)$`)
	goVersionPattern := regexp.MustCompile(`^go\s+([\d.]+)$`)
	requirePattern := regexp.MustCompile(`^\s*([^\s]+)\s+([^\s]+)(\s+//\s*indirect)?`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Module declaration
		if matches := modulePattern.FindStringSubmatch(line); len(matches) >= 2 {
			mod.Module = matches[1]
			continue
		}

		// Go version
		if matches := goVersionPattern.FindStringSubmatch(line); len(matches) >= 2 {
			mod.GoVersion = matches[1]
			continue
		}

		// Start of require block
		if line == "require (" {
			inRequire = true
			continue
		}

		// End of require block
		if line == ")" {
			inRequire = false
			continue
		}

		// Single-line require
		if strings.HasPrefix(line, "require ") {
			line = strings.TrimPrefix(line, "require ")
			if matches := requirePattern.FindStringSubmatch(line); len(matches) >= 3 {
				req := GoRequire{
					Path:     matches[1],
					Version:  matches[2],
					Indirect: len(matches) >= 4 && matches[3] != "",
				}
				mod.Require = append(mod.Require, req)
				mod.IndirectMap[req.Path] = req.Indirect
			}
			continue
		}

		// Inside require block
		if inRequire {
			if matches := requirePattern.FindStringSubmatch(line); len(matches) >= 3 {
				req := GoRequire{
					Path:     matches[1],
					Version:  matches[2],
					Indirect: len(matches) >= 4 && matches[3] != "",
				}
				mod.Require = append(mod.Require, req)
				mod.IndirectMap[req.Path] = req.Indirect
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning go.mod: %w", err)
	}

	return mod, nil
}

// FindGoMod searches for go.mod files in the project.
func FindGoMod(projectPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == ".git" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == "go.mod" {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// DirectDependencies returns only direct (non-indirect) dependencies.
func (m *GoModule) DirectDependencies() []GoRequire {
	var direct []GoRequire
	for _, req := range m.Require {
		if !req.Indirect {
			direct = append(direct, req)
		}
	}
	return direct
}

// HasDependency checks if a dependency exists.
func (m *GoModule) HasDependency(path string) bool {
	for _, req := range m.Require {
		if req.Path == path || strings.HasPrefix(req.Path, path+"/") {
			return true
		}
	}
	return false
}

// DependencyVersion returns the version of a dependency.
func (m *GoModule) DependencyVersion(path string) string {
	for _, req := range m.Require {
		if req.Path == path || strings.HasPrefix(req.Path, path+"/") {
			return req.Version
		}
	}
	return ""
}

// AllDependencies returns a map of all dependencies to their versions.
func (m *GoModule) AllDependencies(includeIndirect bool) map[string]string {
	result := make(map[string]string)
	for _, req := range m.Require {
		if includeIndirect || !req.Indirect {
			result[req.Path] = req.Version
		}
	}
	return result
}
