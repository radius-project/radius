// Package analyzers provides language-specific analyzers for detecting
// infrastructure dependencies in codebases.
package analyzers

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
)

// DockerfileInfo contains information extracted from a Dockerfile.
type DockerfileInfo struct {
	// Path is the path to the Dockerfile.
	Path string `json:"path"`

	// BaseImage is the primary base image (from final stage or first FROM).
	BaseImage string `json:"baseImage"`

	// Stages contains all build stages defined in the Dockerfile.
	Stages []DockerStage `json:"stages,omitempty"`

	// ExposedPorts are the ports exposed via EXPOSE instructions.
	ExposedPorts []int `json:"exposedPorts,omitempty"`

	// EntryCommand is the CMD or ENTRYPOINT command.
	EntryCommand string `json:"entryCommand,omitempty"`

	// EntryPoint is the ENTRYPOINT if defined separately from CMD.
	EntryPoint string `json:"entryPoint,omitempty"`

	// WorkDir is the WORKDIR if defined.
	WorkDir string `json:"workDir,omitempty"`

	// EnvironmentVars are ENV variables defined in the Dockerfile.
	EnvironmentVars map[string]string `json:"environmentVars,omitempty"`

	// BundledStages lists stages that are bundled into the final image via COPY --from.
	// This is used to detect services bundled together (e.g., client build copied into backend).
	BundledStages []string `json:"bundledStages,omitempty"`
}

// DockerStage represents a build stage in a multi-stage Dockerfile.
type DockerStage struct {
	Name      string `json:"name,omitempty"`
	BaseImage string `json:"baseImage"`
	IsFinal   bool   `json:"isFinal"`
}

// DockerfileParser parses Dockerfile files to extract deployment information.
type DockerfileParser struct{}

// NewDockerfileParser creates a new DockerfileParser.
func NewDockerfileParser() *DockerfileParser {
	return &DockerfileParser{}
}

// FindDockerfiles searches for Dockerfile files in the given path.
func (p *DockerfileParser) FindDockerfiles(ctx context.Context, projectPath string) ([]string, error) {
	var dockerfiles []string

	dockerfileNames := []string{
		"Dockerfile",
		"Dockerfile.dev",
		"Dockerfile.prod",
		"Dockerfile.production",
		"dockerfile",
	}

	// Check root directory
	for _, name := range dockerfileNames {
		path := filepath.Join(projectPath, name)
		if _, err := os.Stat(path); err == nil {
			dockerfiles = append(dockerfiles, path)
		}
	}

	// Check subdirectories (one level deep for service-specific Dockerfiles)
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return dockerfiles, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range dockerfileNames {
			path := filepath.Join(projectPath, entry.Name(), name)
			if _, err := os.Stat(path); err == nil {
				dockerfiles = append(dockerfiles, path)
			}
		}
	}

	return dockerfiles, nil
}

// Parse extracts information from a Dockerfile.
func (p *DockerfileParser) Parse(ctx context.Context, dockerfilePath string) (*DockerfileInfo, error) {
	file, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info := &DockerfileInfo{
		Path:            dockerfilePath,
		EnvironmentVars: make(map[string]string),
	}

	var stages []DockerStage
	var currentStage *DockerStage
	bundledStagesMap := make(map[string]bool)

	// Regex patterns
	fromPattern := regexp.MustCompile(`(?i)^FROM\s+(\S+)(?:\s+AS\s+(\S+))?`)
	exposePattern := regexp.MustCompile(`(?i)^EXPOSE\s+(.+)`)
	cmdPattern := regexp.MustCompile(`(?i)^CMD\s+(.+)`)
	entrypointPattern := regexp.MustCompile(`(?i)^ENTRYPOINT\s+(.+)`)
	workdirPattern := regexp.MustCompile(`(?i)^WORKDIR\s+(\S+)`)
	envPattern := regexp.MustCompile(`(?i)^ENV\s+(\S+)[=\s]+(.+)`)
	copyFromPattern := regexp.MustCompile(`(?i)^COPY\s+--from=(\S+)\s+`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// FROM instruction - new stage
		if matches := fromPattern.FindStringSubmatch(line); matches != nil {
			if currentStage != nil {
				stages = append(stages, *currentStage)
			}
			currentStage = &DockerStage{
				BaseImage: matches[1],
			}
			if len(matches) > 2 && matches[2] != "" {
				currentStage.Name = matches[2]
			}
			continue
		}

		// COPY --from instruction - detect bundled stages
		if matches := copyFromPattern.FindStringSubmatch(line); matches != nil {
			stageName := matches[1]
			// Track bundled stages (excluding numeric indices which reference external images)
			if _, err := strconv.Atoi(stageName); err != nil {
				bundledStagesMap[stageName] = true
			}
			continue
		}

		// EXPOSE instruction
		if matches := exposePattern.FindStringSubmatch(line); matches != nil {
			ports := parseExposesPorts(matches[1])
			info.ExposedPorts = append(info.ExposedPorts, ports...)
			continue
		}

		// CMD instruction
		if matches := cmdPattern.FindStringSubmatch(line); matches != nil {
			info.EntryCommand = parseDockerCommand(matches[1])
			continue
		}

		// ENTRYPOINT instruction
		if matches := entrypointPattern.FindStringSubmatch(line); matches != nil {
			info.EntryPoint = parseDockerCommand(matches[1])
			continue
		}

		// WORKDIR instruction
		if matches := workdirPattern.FindStringSubmatch(line); matches != nil {
			info.WorkDir = matches[1]
			continue
		}

		// ENV instruction
		if matches := envPattern.FindStringSubmatch(line); matches != nil {
			key := matches[1]
			value := strings.TrimSpace(matches[2])
			// Remove quotes if present
			value = strings.Trim(value, `"'`)
			info.EnvironmentVars[key] = value
			continue
		}
	}

	// Add final stage
	if currentStage != nil {
		currentStage.IsFinal = true
		stages = append(stages, *currentStage)
	}

	info.Stages = stages

	// Convert bundled stages map to slice
	for stage := range bundledStagesMap {
		info.BundledStages = append(info.BundledStages, stage)
	}

	// Set base image from final stage
	if len(stages) > 0 {
		info.BaseImage = stages[len(stages)-1].BaseImage
	}

	return info, scanner.Err()
}

// parseExposesPorts parses port specifications from EXPOSE instruction.
func parseExposesPorts(portSpec string) []int {
	var ports []int
	parts := strings.Fields(portSpec)
	for _, part := range parts {
		// Handle port/protocol format (e.g., "8080/tcp")
		port := strings.Split(part, "/")[0]
		if p, err := strconv.Atoi(port); err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

// parseDockerCommand parses CMD/ENTRYPOINT format (JSON array or shell form).
func parseDockerCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)

	// JSON array format: ["node", "app.js"]
	if strings.HasPrefix(cmd, "[") && strings.HasSuffix(cmd, "]") {
		// Extract command parts
		inner := strings.TrimPrefix(strings.TrimSuffix(cmd, "]"), "[")
		var parts []string
		for _, part := range strings.Split(inner, ",") {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, `"'`)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, " ")
	}

	return cmd
}

// ToEntryPoint converts DockerfileInfo to a dtypes.EntryPoint.
func (info *DockerfileInfo) ToEntryPoint() dtypes.EntryPoint {
	command := info.EntryCommand
	if info.EntryPoint != "" {
		if command != "" {
			command = info.EntryPoint + " " + command
		} else {
			command = info.EntryPoint
		}
	}

	return dtypes.EntryPoint{
		Type:    dtypes.EntryPointDockerfile,
		File:    info.Path,
		Command: command,
	}
}

// DetectBundledServices analyzes Dockerfile stages to determine which services
// are bundled together in production. Returns a map of service name -> bundled into service name.
// For example, if "client-build" stage is copied into "final" stage, and compose has
// a "client" service targeting "client-dev", then client is bundled into backend.
func (p *DockerfileParser) DetectBundledServices(dockerfileInfo *DockerfileInfo, composeServices []ComposeService) map[string]string {
	bundledInto := make(map[string]string)

	// Map stage names to service names
	// Convention: stage "client-build" or "client-dev" maps to service "client"
	stageToService := make(map[string]string)
	for _, svc := range composeServices {
		if svc.Build != nil && svc.Build.Target != "" {
			// Map the target stage to this service
			stageToService[svc.Build.Target] = svc.Name
			// Also map related stages (e.g., client-dev -> client, client-build -> client)
			baseName := extractBaseName(svc.Build.Target)
			if baseName != svc.Build.Target {
				stageToService[baseName+"-build"] = svc.Name
				stageToService[baseName+"-dev"] = svc.Name
				stageToService[baseName+"-base"] = svc.Name
			}
		}
	}

	// Find the final/production stage
	var finalStageName string
	for _, stage := range dockerfileInfo.Stages {
		if stage.IsFinal {
			finalStageName = stage.Name
			break
		}
	}

	// Detect which services are bundled based on COPY --from
	for _, bundledStage := range dockerfileInfo.BundledStages {
		serviceName := stageToService[bundledStage]
		if serviceName == "" {
			// Try extracting base name
			baseName := extractBaseName(bundledStage)
			serviceName = baseName
		}

		// Find which service owns the final stage
		var targetService string
		for _, svc := range composeServices {
			if svc.Build != nil {
				// Check if this service uses a production/final stage
				if svc.Build.Target == finalStageName || svc.Build.Target == "" {
					targetService = svc.Name
					break
				}
				// Check if this is the backend (common convention)
				if strings.Contains(strings.ToLower(svc.Name), "backend") ||
					strings.Contains(strings.ToLower(svc.Name), "server") ||
					strings.Contains(strings.ToLower(svc.Name), "api") {
					targetService = svc.Name
				}
			}
		}

		if serviceName != "" && targetService != "" && serviceName != targetService {
			bundledInto[serviceName] = targetService
		}
	}

	return bundledInto
}

// extractBaseName extracts the base service name from a stage name.
// e.g., "client-build" -> "client", "backend-dev" -> "backend"
func extractBaseName(stageName string) string {
	suffixes := []string{"-build", "-dev", "-base", "-prod", "-production", "-final", "-test"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(stageName, suffix) {
			return strings.TrimSuffix(stageName, suffix)
		}
	}
	return stageName
}
