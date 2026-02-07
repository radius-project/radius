package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/discovery"
)

// DiscoverServicesSkill detects deployable services in a codebase.
type DiscoverServicesSkill struct{}

// NewDiscoverServicesSkill creates the discover_services skill.
func NewDiscoverServicesSkill() *DiscoverServicesSkill {
	return &DiscoverServicesSkill{}
}

// Name returns the skill identifier.
func (s *DiscoverServicesSkill) Name() string {
	return "discover_services"
}

// Description returns a human-readable description.
func (s *DiscoverServicesSkill) Description() string {
	return "Detects deployable services by finding Dockerfiles, main entry points, and application frameworks in a codebase."
}

// InputSchema returns the JSON Schema for input parameters.
func (s *DiscoverServicesSkill) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"projectPath": map[string]interface{}{
				"type":        "string",
				"description": "Path to the project directory to analyze",
			},
		},
		"required": []string{"projectPath"},
	}
}

// Execute runs the skill.
func (s *DiscoverServicesSkill) Execute(ctx context.Context, input SkillInput) (SkillOutput, error) {
	if input.ProjectPath == "" {
		return NewErrorOutput(fmt.Errorf("projectPath is required")), nil
	}

	var services []discovery.Service
	var warnings []string

	// Find Dockerfiles
	dockerServices, err := s.findDockerfiles(input.ProjectPath)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Error scanning for Dockerfiles: %v", err))
	} else {
		services = append(services, dockerServices...)
	}

	// Find services from package contexts (already in SkillContext)
	if input.Context != nil && len(input.Context.Services) > 0 {
		for _, svc := range input.Context.Services {
			if svcMap, ok := svc.(map[string]interface{}); ok {
				// Merge with existing services (deduplicate by path)
				_ = svcMap // TODO: merge logic
			}
		}
	}

	output := NewSuccessOutput(map[string]interface{}{
		"services": services,
		"count":    len(services),
	})
	output.Warnings = warnings

	return output, nil
}

func (s *DiscoverServicesSkill) findDockerfiles(projectPath string) ([]discovery.Service, error) {
	var services []discovery.Service

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" ||
				name == ".venv" || name == "venv" {
				return filepath.SkipDir
			}
			return nil
		}

		// Match Dockerfile patterns
		name := info.Name()
		isDockerfile := name == "Dockerfile" ||
			strings.HasPrefix(name, "Dockerfile.") ||
			strings.HasSuffix(name, ".Dockerfile")

		if !isDockerfile {
			return nil
		}

		dir := filepath.Dir(path)
		svcName := filepath.Base(dir)
		if svcName == "." || svcName == projectPath {
			svcName = filepath.Base(projectPath)
		}

		// Try to determine variant from Dockerfile name
		variant := ""
		if name != "Dockerfile" {
			if strings.HasPrefix(name, "Dockerfile.") {
				variant = strings.TrimPrefix(name, "Dockerfile.")
			} else if strings.HasSuffix(name, ".Dockerfile") {
				variant = strings.TrimSuffix(name, ".Dockerfile")
			}
		}

		if variant != "" && variant != svcName {
			svcName = fmt.Sprintf("%s-%s", svcName, variant)
		}

		relPath, _ := filepath.Rel(projectPath, path)

		svc := discovery.Service{
			Name: svcName,
			Path: dir,
			EntryPoint: discovery.EntryPoint{
				Type: discovery.EntryPointDockerfile,
				File: name,
			},
			Dockerfile: relPath,
			Evidence: []discovery.Evidence{
				{
					Type:    discovery.EvidencePackageManifest,
					File:    relPath,
					Line:    1,
					Snippet: "FROM ...",
				},
			},
			Confidence: 0.9,
		}

		// Try to detect exposed ports by reading Dockerfile
		ports := s.parseExposedPorts(path)
		if len(ports) > 0 {
			svc.ExposedPorts = ports
		}

		services = append(services, svc)
		return nil
	})

	return services, err
}

func (s *DiscoverServicesSkill) parseExposedPorts(dockerfilePath string) []int {
	var ports []int

	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return ports
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "EXPOSE") {
			continue
		}

		// Parse EXPOSE instruction
		parts := strings.Fields(line)
		for i := 1; i < len(parts); i++ {
			var port int
			portStr := parts[i]
			// Handle port/protocol format (e.g., 8080/tcp)
			if idx := strings.Index(portStr, "/"); idx > 0 {
				portStr = portStr[:idx]
			}
			if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil && port > 0 {
				ports = append(ports, port)
			}
		}
	}

	return ports
}

func init() {
	_ = Register(NewDiscoverServicesSkill())
}
