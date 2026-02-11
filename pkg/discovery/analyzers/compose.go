// Package analyzers provides language-specific analyzers for detecting
// infrastructure dependencies in codebases.
package analyzers

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeInfo contains information extracted from a Docker Compose file.
type ComposeInfo struct {
	// Path is the path to the compose file.
	Path string `json:"path"`

	// Services defined in the compose file.
	Services []ComposeService `json:"services"`

	// Volumes defined in the compose file.
	Volumes []string `json:"volumes,omitempty"`

	// Networks defined in the compose file.
	Networks []string `json:"networks,omitempty"`
}

// ComposeService represents a service defined in Docker Compose.
type ComposeService struct {
	Name string `json:"name"`

	// Image is the container image (if not using build).
	Image string `json:"image,omitempty"`

	// Build context if building from Dockerfile.
	Build *ComposeBuild `json:"build,omitempty"`

	// Ports exposed by the service.
	Ports []int `json:"ports,omitempty"`

	// Environment variables.
	Environment map[string]string `json:"environment,omitempty"`

	// Dependencies (depends_on).
	DependsOn []string `json:"dependsOn,omitempty"`

	// Command override.
	Command string `json:"command,omitempty"`

	// Volumes mounted.
	Volumes []string `json:"volumes,omitempty"`

	// IsInfrastructure indicates this is a database/cache/queue service.
	IsInfrastructure bool `json:"isInfrastructure"`

	// InfrastructureType is the type if IsInfrastructure is true.
	InfrastructureType string `json:"infrastructureType,omitempty"`
}

// ComposeBuild represents build configuration in Docker Compose.
type ComposeBuild struct {
	Context    string `json:"context,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty"`
	Target     string `json:"target,omitempty"`
}

// ComposeParser parses Docker Compose files to extract deployment information.
type ComposeParser struct{}

// NewComposeParser creates a new ComposeParser.
func NewComposeParser() *ComposeParser {
	return &ComposeParser{}
}

// FindComposeFiles searches for Docker Compose files in the given path.
func (p *ComposeParser) FindComposeFiles(ctx context.Context, projectPath string) ([]string, error) {
	var composeFiles []string

	composeNames := []string{
		"compose.yaml",
		"compose.yml",
		"docker-compose.yaml",
		"docker-compose.yml",
		"docker-compose.dev.yaml",
		"docker-compose.dev.yml",
		"docker-compose.prod.yaml",
		"docker-compose.prod.yml",
		"docker-compose.override.yaml",
		"docker-compose.override.yml",
	}

	for _, name := range composeNames {
		path := filepath.Join(projectPath, name)
		if _, err := os.Stat(path); err == nil {
			composeFiles = append(composeFiles, path)
		}
	}

	return composeFiles, nil
}

// rawComposeFile represents the raw structure of a docker-compose file.
type rawComposeFile struct {
	Services map[string]rawComposeService `yaml:"services"`
	Volumes  map[string]interface{}       `yaml:"volumes"`
	Networks map[string]interface{}       `yaml:"networks"`
}

type rawComposeService struct {
	Image       string                 `yaml:"image"`
	Build       interface{}            `yaml:"build"` // Can be string or object
	Ports       []interface{}          `yaml:"ports"`
	Environment interface{}            `yaml:"environment"` // Can be list or map
	DependsOn   interface{}            `yaml:"depends_on"`  // Can be list or map
	Command     interface{}            `yaml:"command"`     // Can be string or list
	Volumes     []string               `yaml:"volumes"`
	Labels      map[string]string      `yaml:"labels"`
	Healthcheck map[string]interface{} `yaml:"healthcheck"`
}

// Parse extracts information from a Docker Compose file.
func (p *ComposeParser) Parse(ctx context.Context, composePath string) (*ComposeInfo, error) {
	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, err
	}

	var raw rawComposeFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	info := &ComposeInfo{
		Path: composePath,
	}

	// Parse services
	for name, svc := range raw.Services {
		service := ComposeService{
			Name:        name,
			Image:       svc.Image,
			Environment: make(map[string]string),
		}

		// Parse build config
		if svc.Build != nil {
			service.Build = parseBuildConfig(svc.Build)
		}

		// Parse ports
		service.Ports = parsePorts(svc.Ports)

		// Parse environment
		service.Environment = parseEnvironment(svc.Environment)

		// Parse depends_on
		service.DependsOn = parseDependsOn(svc.DependsOn)

		// Parse command
		service.Command = parseCommand(svc.Command)

		// Parse volumes
		service.Volumes = svc.Volumes

		// Detect if this is infrastructure (database, cache, etc.)
		service.IsInfrastructure, service.InfrastructureType = detectInfrastructureService(service)

		info.Services = append(info.Services, service)
	}

	// Parse volumes
	for name := range raw.Volumes {
		info.Volumes = append(info.Volumes, name)
	}

	// Parse networks
	for name := range raw.Networks {
		info.Networks = append(info.Networks, name)
	}

	return info, nil
}

func parseBuildConfig(build interface{}) *ComposeBuild {
	switch b := build.(type) {
	case string:
		return &ComposeBuild{Context: b}
	case map[string]interface{}:
		cb := &ComposeBuild{}
		if v, ok := b["context"].(string); ok {
			cb.Context = v
		}
		if v, ok := b["dockerfile"].(string); ok {
			cb.Dockerfile = v
		}
		if v, ok := b["target"].(string); ok {
			cb.Target = v
		}
		return cb
	}
	return nil
}

func parsePorts(ports []interface{}) []int {
	var result []int
	for _, p := range ports {
		switch v := p.(type) {
		case int:
			result = append(result, v)
		case string:
			// Handle "8080:8080" or "8080" format
			parts := strings.Split(v, ":")
			var portStr string
			if len(parts) >= 2 {
				portStr = parts[0] // Host port
			} else {
				portStr = parts[0]
			}
			// Remove any protocol suffix
			portStr = strings.Split(portStr, "/")[0]
			if port, err := strconv.Atoi(portStr); err == nil {
				result = append(result, port)
			}
		}
	}
	return result
}

func parseEnvironment(env interface{}) map[string]string {
	result := make(map[string]string)
	switch e := env.(type) {
	case []interface{}:
		// List format: ["KEY=value", "KEY2=value2"]
		for _, item := range e {
			if s, ok := item.(string); ok {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					result[parts[0]] = parts[1]
				}
			}
		}
	case map[string]interface{}:
		// Map format: {KEY: value}
		for k, v := range e {
			if s, ok := v.(string); ok {
				result[k] = s
			} else if v != nil {
				result[k] = ""
			}
		}
	}
	return result
}

func parseDependsOn(deps interface{}) []string {
	var result []string
	switch d := deps.(type) {
	case []interface{}:
		// Simple list format
		for _, item := range d {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	case map[string]interface{}:
		// Extended format with conditions
		for name := range d {
			result = append(result, name)
		}
	}
	return result
}

func parseCommand(cmd interface{}) string {
	switch c := cmd.(type) {
	case string:
		return c
	case []interface{}:
		var parts []string
		for _, item := range c {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// detectInfrastructureService determines if a service is infrastructure (database, cache, etc.).
func detectInfrastructureService(svc ComposeService) (bool, string) {
	image := strings.ToLower(svc.Image)
	name := strings.ToLower(svc.Name)

	// Database detection
	dbPatterns := map[string]string{
		"postgres":    "postgresql",
		"mysql":       "mysql",
		"mariadb":     "mysql",
		"mongo":       "mongodb",
		"redis":       "redis",
		"memcached":   "memcached",
		"elasticsearch": "elasticsearch",
		"cassandra":   "cassandra",
		"couchdb":     "couchdb",
		"dynamodb":    "dynamodb",
		"mssql":       "mssql",
		"sqlserver":   "mssql",
		"cockroach":   "cockroachdb",
		"neo4j":       "neo4j",
		"influxdb":    "influxdb",
	}

	// Check image name
	for pattern, infraType := range dbPatterns {
		if strings.Contains(image, pattern) || strings.Contains(name, pattern) {
			return true, infraType
		}
	}

	// Message queue detection
	mqPatterns := map[string]string{
		"rabbitmq": "rabbitmq",
		"kafka":    "kafka",
		"nats":     "nats",
		"activemq": "activemq",
		"zeromq":   "zeromq",
		"pulsar":   "pulsar",
	}

	for pattern, infraType := range mqPatterns {
		if strings.Contains(image, pattern) || strings.Contains(name, pattern) {
			return true, infraType
		}
	}

	// Other infrastructure
	otherPatterns := map[string]string{
		"nginx":    "loadbalancer",
		"traefik":  "loadbalancer",
		"haproxy":  "loadbalancer",
		"vault":    "vault",
		"consul":   "consul",
		"etcd":     "etcd",
		"zookeeper": "zookeeper",
	}

	for pattern, infraType := range otherPatterns {
		if strings.Contains(image, pattern) || strings.Contains(name, pattern) {
			return true, infraType
		}
	}

	return false, ""
}

// GetApplicationServices returns only application services (not infrastructure).
func (info *ComposeInfo) GetApplicationServices() []ComposeService {
	var result []ComposeService
	for _, svc := range info.Services {
		if !svc.IsInfrastructure {
			result = append(result, svc)
		}
	}
	return result
}

// GetInfrastructureServices returns only infrastructure services.
func (info *ComposeInfo) GetInfrastructureServices() []ComposeService {
	var result []ComposeService
	for _, svc := range info.Services {
		if svc.IsInfrastructure {
			result = append(result, svc)
		}
	}
	return result
}

// GetServiceDependencyGraph returns a map of service -> dependencies.
func (info *ComposeInfo) GetServiceDependencyGraph() map[string][]string {
	graph := make(map[string][]string)
	for _, svc := range info.Services {
		graph[svc.Name] = svc.DependsOn
	}
	return graph
}
