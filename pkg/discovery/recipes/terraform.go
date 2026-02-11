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

// TerraformSource discovers recipes from Terraform Registry.
type TerraformSource struct {
	name       string
	baseURL    string
	namespace  string
	httpClient *http.Client
}

// NewTerraformSource creates a new Terraform Registry recipe source.
func NewTerraformSource(config SourceConfig) (*TerraformSource, error) {
	baseURL := config.URL
	if baseURL == "" {
		baseURL = "https://registry.terraform.io/v1/modules"
	}

	namespace := "hashicorp"
	if ns, ok := config.Options["namespace"]; ok {
		namespace = ns
	}

	return &TerraformSource{
		name:      config.Name,
		baseURL:   baseURL,
		namespace: namespace,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Name returns the source name.
func (s *TerraformSource) Name() string {
	return s.name
}

// Type returns the source type.
func (s *TerraformSource) Type() string {
	return "terraform"
}

// Search searches for recipes matching the resource type.
func (s *TerraformSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	// Map Radius resource types to Terraform module search terms
	searchTerm := s.mapResourceTypeToTerraform(resourceType)
	if searchTerm == "" {
		return nil, nil
	}

	modules, err := s.searchModules(ctx, searchTerm)
	if err != nil {
		return nil, err
	}

	// Filter and convert to recipes
	var recipes []Recipe
	for _, module := range modules {
		recipe := Recipe{
			Name:         module.Name,
			Description:  module.Description,
			ResourceType: resourceType,
			Source:       s.name,
			SourceType:   "terraform",
			Version:      module.Version,
			TemplatePath: fmt.Sprintf("%s/%s/%s", module.Namespace, module.Name, module.Provider),
			Tags:         []string{"terraform", module.Provider},
		}
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

// List lists all available recipes.
func (s *TerraformSource) List(ctx context.Context) ([]Recipe, error) {
	// Return known Terraform modules that work well with Radius
	return s.getKnownTerraformModules(), nil
}

func (s *TerraformSource) mapResourceTypeToTerraform(resourceType string) string {
	mappings := map[string]string{
		"Applications.Datastores/sqlDatabases":     "postgresql,mysql,sql",
		"Applications.Datastores/mongoDatabases":   "mongodb,documentdb",
		"Applications.Datastores/redisCaches":      "redis,elasticache",
		"Applications.Messaging/rabbitMQQueues":    "rabbitmq,sqs,servicebus",
		"Applications.Messaging/kafkaTopics":       "kafka,msk",
		"Applications.Dapr/pubSubBrokers":          "pubsub,sns,eventgrid",
		"Applications.Dapr/stateStores":            "storage,s3,blob",
		"Applications.Dapr/secretStores":           "secrets,vault,keyvault",
	}

	return mappings[resourceType]
}

// TerraformModuleSearchResponse represents the Terraform Registry search response.
type TerraformModuleSearchResponse struct {
	Modules []TerraformModule `json:"modules"`
	Meta    struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	} `json:"meta"`
}

// TerraformModule represents a Terraform module.
type TerraformModule struct {
	ID          string `json:"id"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Downloads   int    `json:"downloads"`
	Verified    bool   `json:"verified"`
}

func (s *TerraformSource) searchModules(ctx context.Context, query string) ([]TerraformModule, error) {
	url := fmt.Sprintf("%s?q=%s&limit=10", s.baseURL, query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching modules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fall back to known modules
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var searchResp TerraformModuleSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return searchResp.Modules, nil
}

func (s *TerraformSource) getKnownTerraformModules() []Recipe {
	return []Recipe{
		{
			Name:         "terraform-aws-rds-postgresql",
			Description:  "AWS RDS PostgreSQL database",
			ResourceType: "Applications.Datastores/sqlDatabases",
			Source:       s.name,
			SourceType:   "terraform",
			Version:      "6.3.0",
			TemplatePath: "terraform-aws-modules/rds/aws",
			Parameters: []RecipeParameter{
				{Name: "identifier", Type: "string", Description: "RDS instance identifier", Required: true},
				{Name: "engine_version", Type: "string", Description: "PostgreSQL version", Required: false, Default: "14"},
				{Name: "instance_class", Type: "string", Description: "Instance class", Required: false, Default: "db.t3.micro"},
			},
			Tags: []string{"terraform", "aws", "rds", "postgresql"},
		},
		{
			Name:         "terraform-azure-postgresql",
			Description:  "Azure Database for PostgreSQL",
			ResourceType: "Applications.Datastores/sqlDatabases",
			Source:       s.name,
			SourceType:   "terraform",
			Version:      "3.0.0",
			TemplatePath: "Azure/postgresql/azurerm",
			Parameters: []RecipeParameter{
				{Name: "server_name", Type: "string", Description: "PostgreSQL server name", Required: true},
				{Name: "sku_name", Type: "string", Description: "SKU name", Required: false, Default: "B_Gen5_1"},
			},
			Tags: []string{"terraform", "azure", "postgresql"},
		},
		{
			Name:         "terraform-aws-elasticache-redis",
			Description:  "AWS ElastiCache Redis cluster",
			ResourceType: "Applications.Datastores/redisCaches",
			Source:       s.name,
			SourceType:   "terraform",
			Version:      "3.5.0",
			TemplatePath: "cloudposse/elasticache-redis/aws",
			Parameters: []RecipeParameter{
				{Name: "name", Type: "string", Description: "Cluster name", Required: true},
				{Name: "instance_type", Type: "string", Description: "Instance type", Required: false, Default: "cache.t3.micro"},
			},
			Tags: []string{"terraform", "aws", "elasticache", "redis"},
		},
		{
			Name:         "terraform-aws-documentdb",
			Description:  "AWS DocumentDB (MongoDB compatible)",
			ResourceType: "Applications.Datastores/mongoDatabases",
			Source:       s.name,
			SourceType:   "terraform",
			Version:      "0.15.0",
			TemplatePath: "cloudposse/documentdb-cluster/aws",
			Parameters: []RecipeParameter{
				{Name: "cluster_name", Type: "string", Description: "Cluster name", Required: true},
				{Name: "instance_class", Type: "string", Description: "Instance class", Required: false, Default: "db.t3.medium"},
			},
			Tags: []string{"terraform", "aws", "documentdb", "mongodb"},
		},
		{
			Name:         "terraform-aws-msk",
			Description:  "AWS MSK (Managed Kafka)",
			ResourceType: "Applications.Messaging/kafkaTopics",
			Source:       s.name,
			SourceType:   "terraform",
			Version:      "2.3.0",
			TemplatePath: "terraform-aws-modules/msk-kafka-cluster/aws",
			Parameters: []RecipeParameter{
				{Name: "cluster_name", Type: "string", Description: "MSK cluster name", Required: true},
				{Name: "kafka_version", Type: "string", Description: "Kafka version", Required: false, Default: "3.4.0"},
			},
			Tags: []string{"terraform", "aws", "msk", "kafka"},
		},
		{
			Name:         "terraform-aws-rabbitmq",
			Description:  "AWS MQ for RabbitMQ",
			ResourceType: "Applications.Messaging/rabbitMQQueues",
			Source:       s.name,
			SourceType:   "terraform",
			Version:      "1.0.0",
			TemplatePath: "cloudposse/mq-broker/aws",
			Parameters: []RecipeParameter{
				{Name: "broker_name", Type: "string", Description: "Broker name", Required: true},
				{Name: "engine_type", Type: "string", Description: "Engine type", Required: false, Default: "RabbitMQ"},
			},
			Tags: []string{"terraform", "aws", "mq", "rabbitmq"},
		},
	}
}

func filterByQuery(modules []TerraformModule, query string) []TerraformModule {
	terms := strings.Split(query, ",")
	var filtered []TerraformModule

	for _, module := range modules {
		for _, term := range terms {
			term = strings.TrimSpace(term)
			if strings.Contains(strings.ToLower(module.Name), term) ||
				strings.Contains(strings.ToLower(module.Description), term) {
				filtered = append(filtered, module)
				break
			}
		}
	}

	return filtered
}
