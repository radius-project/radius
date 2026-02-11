package manifest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PythonRequirement represents a single requirement from requirements.txt.
type PythonRequirement struct {
	Name    string
	Version string
	Extras  []string
}

// Requirements represents a Python requirements.txt file.
type Requirements struct {
	Requirements []PythonRequirement
}

// ParseRequirementsTxt parses a requirements.txt file.
func ParseRequirementsTxt(path string) (*Requirements, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening requirements.txt: %w", err)
	}
	defer file.Close()

	var reqs []PythonRequirement
	scanner := bufio.NewScanner(file)

	// Pattern: package[extra1,extra2]>=version
	reqPattern := regexp.MustCompile(`^([a-zA-Z0-9_-]+)(\[[^\]]+\])?\s*([<>=!~]+\s*[\d.a-zA-Z]+)?`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip options like -r, -e, --
		if strings.HasPrefix(line, "-") {
			continue
		}

		matches := reqPattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			req := PythonRequirement{
				Name: strings.ToLower(matches[1]),
			}

			// Parse extras [extra1,extra2]
			if len(matches) >= 3 && matches[2] != "" {
				extraStr := strings.Trim(matches[2], "[]")
				req.Extras = strings.Split(extraStr, ",")
			}

			// Parse version constraint
			if len(matches) >= 4 && matches[3] != "" {
				req.Version = strings.TrimSpace(matches[3])
			}

			reqs = append(reqs, req)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning requirements.txt: %w", err)
	}

	return &Requirements{Requirements: reqs}, nil
}

// FindRequirementsTxt searches for requirements.txt files in the project.
func FindRequirementsTxt(projectPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			name := info.Name()
			if name == "venv" || name == ".venv" || name == "__pycache__" ||
				name == ".git" || name == "site-packages" || name == ".tox" {
				return filepath.SkipDir
			}
			return nil
		}

		name := info.Name()
		if name == "requirements.txt" ||
			strings.HasPrefix(name, "requirements-") && strings.HasSuffix(name, ".txt") ||
			strings.HasPrefix(name, "requirements_") && strings.HasSuffix(name, ".txt") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// HasPackage checks if a package exists in requirements.
func (r *Requirements) HasPackage(name string) bool {
	name = strings.ToLower(name)
	for _, req := range r.Requirements {
		if req.Name == name {
			return true
		}
	}
	return false
}

// PackageVersion returns the version constraint for a package.
func (r *Requirements) PackageVersion(name string) string {
	name = strings.ToLower(name)
	for _, req := range r.Requirements {
		if req.Name == name {
			return req.Version
		}
	}
	return ""
}

// AllPackages returns a map of all packages to their versions.
func (r *Requirements) AllPackages() map[string]string {
	result := make(map[string]string)
	for _, req := range r.Requirements {
		result[req.Name] = req.Version
	}
	return result
}
