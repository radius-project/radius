// Package templates provides template-related utilities for output generation.
package templates

// DependencyType represents an available infrastructure dependency type.
type DependencyType struct {
	// Name is the dependency type name.
	Name string

	// Description is a human-readable description.
	Description string

	// ResourceType is the Radius resource type.
	// Core types use Applications.* namespace (built into Radius).
	// Contrib types use Radius.* namespace (from radius-project/resource-types-contrib).
	ResourceType string

	// APIVersion is the API version for the resource type.
	APIVersion string

	// Source indicates where this resource type is defined.
	// "core" = built-in to Radius, "contrib" = from resource-types-contrib
	Source string
}

// GetAvailableDependencyTypes returns all available dependency types for scaffolding.
// This includes both core Radius types and types from resource-types-contrib.
func GetAvailableDependencyTypes() []DependencyType {
	return []DependencyType{
		// Core Radius types (Applications.* namespace)
		{
			Name:         "postgres",
			Description:  "PostgreSQL database",
			ResourceType: "Applications.Datastores/sqlDatabases",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "mysql",
			Description:  "MySQL database",
			ResourceType: "Applications.Datastores/sqlDatabases",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "redis",
			Description:  "Redis cache",
			ResourceType: "Applications.Datastores/redisCaches",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "mongodb",
			Description:  "MongoDB database",
			ResourceType: "Applications.Datastores/mongoDatabases",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "rabbitmq",
			Description:  "RabbitMQ message queue",
			ResourceType: "Applications.Messaging/rabbitMQQueues",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "kafka",
			Description:  "Apache Kafka",
			ResourceType: "Applications.Messaging/kafkaQueues",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "statestore",
			Description:  "Dapr state store",
			ResourceType: "Applications.Dapr/stateStores",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "pubsub",
			Description:  "Dapr pub/sub",
			ResourceType: "Applications.Dapr/pubSubBrokers",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		{
			Name:         "secretstore",
			Description:  "Dapr secret store",
			ResourceType: "Applications.Dapr/secretStores",
			APIVersion:   "2023-10-01-preview",
			Source:       "core",
		},
		// Contrib types (Radius.* namespace from resource-types-contrib)
		// These are community-contributed resource types with recipes
		{
			Name:         "mysql-contrib",
			Description:  "MySQL database (contrib)",
			ResourceType: "Radius.Data/mySqlDatabases",
			APIVersion:   "2025-08-01-preview",
			Source:       "contrib",
		},
		{
			Name:         "postgres-contrib",
			Description:  "PostgreSQL database (contrib)",
			ResourceType: "Radius.Data/postgreSqlDatabases",
			APIVersion:   "2025-08-01-preview",
			Source:       "contrib",
		},
		{
			Name:         "redis-contrib",
			Description:  "Redis cache (contrib)",
			ResourceType: "Radius.Data/redisCaches",
			APIVersion:   "2025-08-01-preview",
			Source:       "contrib",
		},
	}
}

// GetDependencyByName returns a dependency type by name.
func GetDependencyByName(name string) *DependencyType {
	for _, dep := range GetAvailableDependencyTypes() {
		if dep.Name == name {
			return &dep
		}
	}
	return nil
}

// ApplicationTemplate represents an application scaffolding template.
type ApplicationTemplate struct {
	// Name is the template name.
	Name string

	// Description is a human-readable description.
	Description string

	// DefaultDependencies are dependencies typically needed by this template.
	DefaultDependencies []string

	// Files is the list of files this template generates.
	Files []string
}

// GetAvailableTemplates returns all available application templates.
func GetAvailableTemplates() []ApplicationTemplate {
	return []ApplicationTemplate{
		{
			Name:                "web-api",
			Description:         "Web API backend service",
			DefaultDependencies: []string{"postgres", "redis"},
			Files:               []string{"Dockerfile", "app.bicep"},
		},
		{
			Name:                "worker",
			Description:         "Background worker service",
			DefaultDependencies: []string{"rabbitmq", "redis"},
			Files:               []string{"Dockerfile", "app.bicep"},
		},
		{
			Name:                "frontend",
			Description:         "Frontend web application",
			DefaultDependencies: nil,
			Files:               []string{"Dockerfile", "app.bicep"},
		},
		{
			Name:                "microservice",
			Description:         "Microservice with Dapr integration",
			DefaultDependencies: []string{"statestore", "pubsub"},
			Files:               []string{"Dockerfile", "app.bicep"},
		},
		{
			Name:                "minimal",
			Description:         "Minimal application with no dependencies",
			DefaultDependencies: nil,
			Files:               []string{"app.bicep"},
		},
	}
}

// GetTemplateByName returns an application template by name.
func GetTemplateByName(name string) *ApplicationTemplate {
	for _, tmpl := range GetAvailableTemplates() {
		if tmpl.Name == name {
			return &tmpl
		}
	}
	return nil
}
