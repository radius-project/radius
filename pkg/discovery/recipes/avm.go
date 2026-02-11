// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AVMSource discovers recipes from Azure Verified Modules.
type AVMSource struct {
	name       string
	baseURL    string
	httpClient *http.Client
}

// NewAVMSource creates a new AVM recipe source.
func NewAVMSource(config SourceConfig) (*AVMSource, error) {
	baseURL := config.URL
	if baseURL == "" {
		baseURL = "https://aka.ms/avm/modules"
	}

	return &AVMSource{
		name:    config.Name,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Name returns the source name.
func (s *AVMSource) Name() string {
	return s.name
}

// Type returns the source type.
func (s *AVMSource) Type() string {
	return "avm"
}

// Search searches for recipes matching the resource type.
func (s *AVMSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	// Map Radius resource types to AVM module patterns
	modulePattern := s.mapResourceTypeToAVM(resourceType)
	if modulePattern == "" {
		return nil, nil
	}

	// Get AVM modules matching the pattern
	modules, err := s.fetchModules(ctx, modulePattern)
	if err != nil {
		return nil, err
	}

	return modules, nil
}

// List lists all available recipes.
func (s *AVMSource) List(ctx context.Context) ([]Recipe, error) {
	return s.fetchModules(ctx, "")
}

func (s *AVMSource) mapResourceTypeToAVM(resourceType string) string {
	// Map Radius resource types (from resource-types-contrib) to AVM module patterns
	// See: https://github.com/radius-project/resource-types-contrib
	mappings := map[string]string{
		// New Radius.* namespace from resource-types-contrib
		"Radius.Data/mySqlDatabases":      "avm/res/sql",
		"Radius.Data/postgreSqlDatabases": "avm/res/db-for-postgre-sql",
		"Radius.Data/mongoDatabases":      "avm/res/document-db/database-account",
		"Radius.Data/redisCaches":         "avm/res/cache/redis",
		"Radius.Messaging/rabbitMQQueues": "avm/res/service-bus",
		"Radius.Security/secrets":         "avm/res/key-vault/vault",
		"Radius.Compute/containers":       "avm/res/container-instances",
		// Legacy Applications.* namespace (for backward compatibility)
		"Applications.Datastores/sqlDatabases":   "avm/res/sql",
		"Applications.Datastores/mongoDatabases": "avm/res/document-db/database-account",
		"Applications.Datastores/redisCaches":    "avm/res/cache/redis",
		"Applications.Messaging/rabbitMQQueues":  "avm/res/service-bus",
		"Applications.Dapr/pubSubBrokers":        "avm/res/service-bus",
		"Applications.Dapr/stateStores":          "avm/res/storage/storage-account",
		"Applications.Dapr/secretStores":         "avm/res/key-vault/vault",
	}

	return mappings[resourceType]
}

func (s *AVMSource) fetchModules(ctx context.Context, pattern string) ([]Recipe, error) {
	// AVM modules are published to the Bicep public module registry
	// For now, return known AVM modules that work with Radius
	knownModules := s.getKnownAVMModules()

	if pattern == "" {
		return knownModules, nil
	}

	var filtered []Recipe
	for _, module := range knownModules {
		if strings.Contains(module.TemplatePath, pattern) {
			filtered = append(filtered, module)
		}
	}

	return filtered, nil
}

func (s *AVMSource) getKnownAVMModules() []Recipe {
	// These map to resource-types-contrib resource types and their recipes
	// See: https://github.com/radius-project/resource-types-contrib
	return []Recipe{
		// MySQL Database - maps to Radius.Data/mySqlDatabases
		{
			Name:         "kubernetes-mysql",
			Description:  "MySQL database on Kubernetes from resource-types-contrib",
			ResourceType: "Radius.Data/mySqlDatabases",
			Source:       s.name,
			SourceType:   "contrib",
			Version:      "2025-08-01-preview",
			TemplatePath: "ghcr.io/radius-project/recipes/data/mysql:kubernetes",
			Parameters: []RecipeParameter{
				{Name: "database", Type: "string", Description: "Database name", Required: false},
				{Name: "username", Type: "string", Description: "MySQL username", Required: false},
				{Name: "version", Type: "string", Description: "MySQL version (5.7, 8.0, 8.4)", Required: false, Default: "8.4"},
			},
			Tags: []string{"kubernetes", "mysql", "database", "contrib"},
		},
		// PostgreSQL Database - maps to Radius.Data/postgreSqlDatabases
		{
			Name:         "kubernetes-postgresql",
			Description:  "PostgreSQL database on Kubernetes from resource-types-contrib",
			ResourceType: "Radius.Data/postgreSqlDatabases",
			Source:       s.name,
			SourceType:   "contrib",
			Version:      "2025-08-01-preview",
			TemplatePath: "ghcr.io/radius-project/recipes/data/postgresql:kubernetes",
			Parameters: []RecipeParameter{
				{Name: "database", Type: "string", Description: "Database name", Required: false},
				{Name: "user", Type: "string", Description: "PostgreSQL username", Required: false, Default: "postgres"},
				{Name: "size", Type: "string", Description: "Size (S, M, L)", Required: false, Default: "S"},
			},
			Tags: []string{"kubernetes", "postgresql", "database", "contrib"},
		},
		// Redis Cache - maps to Radius.Data/redisCaches
		{
			Name:         "kubernetes-redis",
			Description:  "Redis cache on Kubernetes from resource-types-contrib",
			ResourceType: "Radius.Data/redisCaches",
			Source:       s.name,
			SourceType:   "contrib",
			Version:      "2025-08-01-preview",
			TemplatePath: "ghcr.io/radius-project/recipes/data/redis:kubernetes",
			Parameters: []RecipeParameter{
				{Name: "capacity", Type: "string", Description: "Capacity (S, M, L, XL)", Required: false, Default: "M"},
			},
			Tags: []string{"kubernetes", "redis", "cache", "contrib"},
		},
		// Azure SQL Database (AVM)
		{
			Name:         "azure-sql",
			Description:  "Azure SQL Database using AVM",
			ResourceType: "Radius.Data/mySqlDatabases",
			Source:       s.name,
			SourceType:   "avm",
			Version:      "0.4.0",
			TemplatePath: "br/public:avm/res/sql/server:0.4.0",
			Parameters: []RecipeParameter{
				{Name: "serverName", Type: "string", Description: "SQL Server name", Required: true},
				{Name: "databaseName", Type: "string", Description: "Database name", Required: true},
				{Name: "sku", Type: "string", Description: "SKU name", Required: false, Default: "Basic"},
			},
			Tags: []string{"azure", "sql", "database", "avm"},
		},
		// Azure Cache for Redis (AVM)
		{
			Name:         "azure-redis",
			Description:  "Azure Cache for Redis using AVM",
			ResourceType: "Radius.Data/redisCaches",
			Source:       s.name,
			SourceType:   "avm",
			Version:      "0.3.0",
			TemplatePath: "br/public:avm/res/cache/redis:0.3.0",
			Parameters: []RecipeParameter{
				{Name: "name", Type: "string", Description: "Redis cache name", Required: true},
				{Name: "capacity", Type: "int", Description: "Cache capacity", Required: false, Default: 1},
			},
			Tags: []string{"azure", "redis", "cache", "avm"},
		},
		// Azure Cosmos DB MongoDB (AVM)
		{
			Name:         "azure-cosmosdb-mongodb",
			Description:  "Azure Cosmos DB with MongoDB API using AVM",
			ResourceType: "Radius.Data/mongoDatabases",
			Source:       s.name,
			SourceType:   "avm",
			Version:      "0.5.0",
			TemplatePath: "br/public:avm/res/document-db/database-account:0.5.0",
			Parameters: []RecipeParameter{
				{Name: "name", Type: "string", Description: "Cosmos DB account name", Required: true},
				{Name: "kind", Type: "string", Description: "Database kind", Required: false, Default: "MongoDB"},
			},
			Tags: []string{"azure", "cosmosdb", "mongodb", "avm"},
		},
		// Azure Service Bus (AVM)
		{
			Name:         "azure-servicebus",
			Description:  "Azure Service Bus using AVM",
			ResourceType: "Radius.Messaging/rabbitMQQueues",
			Source:       s.name,
			SourceType:   "avm",
			Version:      "0.4.0",
			TemplatePath: "br/public:avm/res/service-bus/namespace:0.4.0",
			Parameters: []RecipeParameter{
				{Name: "name", Type: "string", Description: "Service Bus namespace name", Required: true},
				{Name: "sku", Type: "string", Description: "SKU name", Required: false, Default: "Standard"},
			},
			Tags: []string{"azure", "servicebus", "messaging", "avm"},
		},
		// Azure Key Vault (AVM)
		{
			Name:         "azure-keyvault",
			Description:  "Azure Key Vault using AVM",
			ResourceType: "Radius.Security/secrets",
			Source:       s.name,
			SourceType:   "avm",
			Version:      "0.6.0",
			TemplatePath: "br/public:avm/res/key-vault/vault:0.6.0",
			Parameters: []RecipeParameter{
				{Name: "name", Type: "string", Description: "Key Vault name", Required: true},
				{Name: "sku", Type: "string", Description: "SKU name", Required: false, Default: "standard"},
			},
			Tags: []string{"azure", "keyvault", "secrets", "avm"},
		},
	}
}

// AVMModuleResponse represents the response from AVM module listing.
type AVMModuleResponse struct {
	Modules []AVMModule `json:"modules"`
}

// AVMModule represents an AVM module.
type AVMModule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Path        string `json:"path"`
}

func (s *AVMSource) fetchFromRegistry(ctx context.Context, pattern string) ([]Recipe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching modules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var moduleResp AVMModuleResponse
	if err := json.Unmarshal(body, &moduleResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	var recipes []Recipe
	for _, module := range moduleResp.Modules {
		if pattern != "" && !strings.Contains(module.Path, pattern) {
			continue
		}

		recipes = append(recipes, Recipe{
			Name:         module.Name,
			Description:  module.Description,
			Source:       s.name,
			SourceType:   "avm",
			Version:      module.Version,
			TemplatePath: module.Path,
			Tags:         []string{"azure", "avm"},
		})
	}

	return recipes, nil
}
