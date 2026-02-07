// Package resourcetypes provides Resource Type catalog management.
package resourcetypes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
)

// ContribSource provides dynamic resource type lookup from resource-types-contrib.
// Repository: https://github.com/radius-project/resource-types-contrib
type ContribSource struct {
	baseURL    string
	httpClient *http.Client
	cache      map[string]*ContribResourceType
	cacheMu    sync.RWMutex
}

// ContribResourceType represents a resource type definition from resource-types-contrib.
type ContribResourceType struct {
	// Namespace is the resource type namespace (e.g., "Radius.Data")
	Namespace string `json:"namespace"`

	// Name is the resource type name (e.g., "mySqlDatabases")
	Name string `json:"name"`

	// FullName is the complete resource type identifier (e.g., "Radius.Data/mySqlDatabases")
	FullName string `json:"fullName"`

	// APIVersion is the API version (e.g., "2025-08-01-preview")
	APIVersion string `json:"apiVersion"`

	// Description describes the resource type
	Description string `json:"description,omitempty"`

	// Category is the resource category (e.g., "data", "messaging", "compute")
	Category string `json:"category,omitempty"`

	// Keywords for matching dependency types
	Keywords []string `json:"keywords,omitempty"`
}

// NewContribSource creates a new resource-types-contrib source.
func NewContribSource() *ContribSource {
	return &ContribSource{
		baseURL: "https://raw.githubusercontent.com/radius-project/resource-types-contrib/main",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]*ContribResourceType),
	}
}

// knownContribTypes returns the known resource types from resource-types-contrib.
// This serves as a fallback when the remote lookup fails.
func (s *ContribSource) knownContribTypes() []ContribResourceType {
	return []ContribResourceType{
		// Data namespace
		{
			Namespace:   "Radius.Data",
			Name:        "mySqlDatabases",
			FullName:    "Radius.Data/mySqlDatabases",
			APIVersion:  "2025-08-01-preview",
			Description: "MySQL database resource from resource-types-contrib",
			Category:    "data",
			Keywords:    []string{"mysql", "mariadb", "sql", "database", "db"},
		},
		{
			Namespace:   "Radius.Data",
			Name:        "postgreSqlDatabases",
			FullName:    "Radius.Data/postgreSqlDatabases",
			APIVersion:  "2025-08-01-preview",
			Description: "PostgreSQL database resource from resource-types-contrib",
			Category:    "data",
			Keywords:    []string{"postgresql", "postgres", "pg", "sql", "database", "db"},
		},
		{
			Namespace:   "Radius.Data",
			Name:        "mongoDatabases",
			FullName:    "Radius.Data/mongoDatabases",
			APIVersion:  "2025-08-01-preview",
			Description: "MongoDB database resource from resource-types-contrib",
			Category:    "data",
			Keywords:    []string{"mongodb", "mongo", "nosql", "document", "database", "db"},
		},
		{
			Namespace:   "Radius.Data",
			Name:        "redisCaches",
			FullName:    "Radius.Data/redisCaches",
			APIVersion:  "2025-08-01-preview",
			Description: "Redis cache resource from resource-types-contrib",
			Category:    "data",
			Keywords:    []string{"redis", "cache", "keyvalue", "kv"},
		},
		// Messaging namespace
		{
			Namespace:   "Radius.Messaging",
			Name:        "rabbitMQQueues",
			FullName:    "Radius.Messaging/rabbitMQQueues",
			APIVersion:  "2025-08-01-preview",
			Description: "RabbitMQ queue resource from resource-types-contrib",
			Category:    "messaging",
			Keywords:    []string{"rabbitmq", "rabbit", "amqp", "queue", "messaging", "mq"},
		},
		{
			Namespace:   "Radius.Messaging",
			Name:        "kafkaQueues",
			FullName:    "Radius.Messaging/kafkaQueues",
			APIVersion:  "2025-08-01-preview",
			Description: "Kafka queue resource from resource-types-contrib",
			Category:    "messaging",
			Keywords:    []string{"kafka", "streaming", "queue", "messaging", "event"},
		},
		// Compute namespace
		{
			Namespace:   "Radius.Compute",
			Name:        "containers",
			FullName:    "Radius.Compute/containers",
			APIVersion:  "2025-08-01-preview",
			Description: "Container resource from resource-types-contrib",
			Category:    "compute",
			Keywords:    []string{"container", "docker", "pod", "workload"},
		},
		// Network namespace
		{
			Namespace:   "Radius.Network",
			Name:        "loadBalancers",
			FullName:    "Radius.Network/loadBalancers",
			APIVersion:  "2025-08-01-preview",
			Description: "Load balancer resource from resource-types-contrib",
			Category:    "network",
			Keywords:    []string{"loadbalancer", "lb", "traefik", "nginx", "haproxy", "ingress", "gateway"},
		},
		{
			Namespace:   "Radius.Network",
			Name:        "gateways",
			FullName:    "Radius.Network/gateways",
			APIVersion:  "2025-08-01-preview",
			Description: "API Gateway resource from resource-types-contrib",
			Category:    "network",
			Keywords:    []string{"gateway", "api", "ingress", "route"},
		},
		// Security namespace
		{
			Namespace:   "Radius.Security",
			Name:        "secrets",
			FullName:    "Radius.Security/secrets",
			APIVersion:  "2025-08-01-preview",
			Description: "Secret store resource from resource-types-contrib",
			Category:    "security",
			Keywords:    []string{"secret", "vault", "keyvault", "credential"},
		},
		// Storage namespace
		{
			Namespace:   "Radius.Storage",
			Name:        "blobContainers",
			FullName:    "Radius.Storage/blobContainers",
			APIVersion:  "2025-08-01-preview",
			Description: "Blob storage container resource from resource-types-contrib",
			Category:    "storage",
			Keywords:    []string{"blob", "storage", "s3", "bucket", "object"},
		},
	}
}

// LookupByDependencyType finds a resource type that matches a dependency type.
// It uses keyword matching to find the best match from resource-types-contrib.
func (s *ContribSource) LookupByDependencyType(ctx context.Context, depType dtypes.DependencyType) (*ContribResourceType, error) {
	depTypeStr := strings.ToLower(string(depType))

	// Check cache first
	s.cacheMu.RLock()
	if cached, ok := s.cache[depTypeStr]; ok {
		s.cacheMu.RUnlock()
		return cached, nil
	}
	s.cacheMu.RUnlock()

	// Search known types by keyword matching
	knownTypes := s.knownContribTypes()

	var bestMatch *ContribResourceType
	bestScore := 0

	for i := range knownTypes {
		rt := &knownTypes[i]
		score := s.matchScore(depTypeStr, rt)
		if score > bestScore {
			bestScore = score
			bestMatch = rt
		}
	}

	// Require a minimum match score
	if bestScore >= 2 {
		// Cache the result
		s.cacheMu.Lock()
		s.cache[depTypeStr] = bestMatch
		s.cacheMu.Unlock()
		return bestMatch, nil
	}

	return nil, fmt.Errorf("no matching resource type found for dependency: %s", depType)
}

// matchScore calculates how well a dependency type matches a resource type.
func (s *ContribSource) matchScore(depType string, rt *ContribResourceType) int {
	score := 0

	// Direct name match (highest priority)
	if strings.Contains(strings.ToLower(rt.Name), depType) {
		score += 5
	}

	// Keyword matching
	for _, keyword := range rt.Keywords {
		if strings.Contains(depType, keyword) || strings.Contains(keyword, depType) {
			score += 3
		}
		// Partial match
		if len(keyword) > 3 && len(depType) > 3 {
			if strings.Contains(keyword, depType[:3]) || strings.Contains(depType, keyword[:3]) {
				score += 1
			}
		}
	}

	// Category matching
	if strings.Contains(depType, rt.Category) {
		score += 2
	}

	return score
}

// ToResourceTypeEntry converts a ContribResourceType to a ResourceTypeEntry.
func (rt *ContribResourceType) ToResourceTypeEntry(depType dtypes.DependencyType) ResourceTypeEntry {
	return ResourceTypeEntry{
		DependencyType:   depType,
		ResourceTypeName: rt.FullName,
		APIVersion:       rt.APIVersion,
		Description:      rt.Description,
		Source:           "contrib", // Mark as from resource-types-contrib
	}
}

// fetchRemoteTypes attempts to fetch the resource types index from GitHub.
// This is for future use when resource-types-contrib publishes an index file.
func (s *ContribSource) fetchRemoteTypes(ctx context.Context) ([]ContribResourceType, error) {
	url := s.baseURL + "/resources/index.json"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch resource types: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var types []ContribResourceType
	if err := json.Unmarshal(body, &types); err != nil {
		return nil, err
	}

	return types, nil
}

// DefaultContribSource is the global contrib source instance.
var DefaultContribSource = NewContribSource()

// LookupFromContrib looks up a resource type from resource-types-contrib.
func LookupFromContrib(ctx context.Context, depType dtypes.DependencyType) (*ContribResourceType, error) {
	return DefaultContribSource.LookupByDependencyType(ctx, depType)
}
