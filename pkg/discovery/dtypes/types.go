// Package dtypes provides core types for the discovery framework.
// This package is separate from the main discovery package to avoid import cycles
// between discovery and its sub-packages (analyzers, catalog, etc.).
package dtypes

import (
	"time"
)

// DiscoveryResult is the top-level result from analyzing a codebase.
type DiscoveryResult struct {
	// Metadata
	ProjectPath     string    `json:"projectPath"`
	AnalyzedAt      time.Time `json:"analyzedAt"`
	AnalyzerVersion string    `json:"analyzerVersion"`

	// Discovered elements
	Services     []Service            `json:"services"`
	Dependencies []DetectedDependency `json:"dependencies"`
	Practices    TeamPractices        `json:"practices"`

	// Generation candidates
	ResourceTypes []ResourceTypeMapping `json:"resourceTypes"`
	Recipes       []RecipeMatch         `json:"recipes"`

	// Validation
	Warnings   []DiscoveryWarning `json:"warnings"`
	Confidence float64            `json:"confidence"` // 0.0-1.0
}

// Service represents a deployable unit detected in the codebase.
type Service struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Language  Language `json:"language"`
	Framework string   `json:"framework,omitempty"`

	// Container info
	Dockerfile   string `json:"dockerfile,omitempty"`
	ExposedPorts []int  `json:"exposedPorts,omitempty"`

	// Entry point
	EntryPoint EntryPoint `json:"entryPoint"`

	// Dependencies this service uses
	DependencyIDs []string `json:"dependencyIds"`

	// Bundled service detection
	// IsBundledInto indicates this service is bundled into another service in production
	// (e.g., frontend static files served by backend)
	IsBundledInto string `json:"isBundledInto,omitempty"`

	// BundlesServices lists services that are bundled into this one
	BundlesServices []string `json:"bundlesServices,omitempty"`

	// Detection confidence
	Confidence float64    `json:"confidence"`
	Evidence   []Evidence `json:"evidence"`
}

// EntryPoint describes how a service is started.
type EntryPoint struct {
	Type    EntryPointType `json:"type"` // dockerfile, main, script
	File    string         `json:"file"`
	Command string         `json:"command,omitempty"`
}

// EntryPointType indicates the type of entry point.
type EntryPointType string

const (
	EntryPointDockerfile EntryPointType = "dockerfile"
	EntryPointMain       EntryPointType = "main"
	EntryPointScript     EntryPointType = "script"
)

// Language represents a programming language.
type Language string

const (
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguageGo         Language = "go"
	LanguageJava       Language = "java"
	LanguageCSharp     Language = "csharp"
)

// DetectedDependency represents an infrastructure dependency found in source code.
type DetectedDependency struct {
	ID   string         `json:"id"`
	Type DependencyType `json:"type"`
	Name string         `json:"name"`

	// Detection details
	Library    string     `json:"library"`
	Version    string     `json:"version,omitempty"`
	Confidence float64    `json:"confidence"`
	Evidence   []Evidence `json:"evidence"`

	// Connection info extracted
	ConnectionEnv string `json:"connectionEnv,omitempty"`
	DefaultPort   int    `json:"defaultPort,omitempty"`

	// Services that use this dependency
	UsedBy []string `json:"usedBy"`
}

// DependencyType categorizes infrastructure dependencies.
type DependencyType string

const (
	DependencyPostgreSQL    DependencyType = "postgresql"
	DependencyMySQL         DependencyType = "mysql"
	DependencyMongoDB       DependencyType = "mongodb"
	DependencyRedis         DependencyType = "redis"
	DependencyRabbitMQ      DependencyType = "rabbitmq"
	DependencyKafka         DependencyType = "kafka"
	DependencyAzureBlob     DependencyType = "azure-blob"
	DependencyCosmosDB      DependencyType = "cosmosdb"
	DependencyAzureKeyVault DependencyType = "azure-keyvault"
	DependencyS3            DependencyType = "s3"
	DependencyUnknown       DependencyType = "unknown"
)

// Evidence provides proof of a detection.
type Evidence struct {
	Type    EvidenceType `json:"type"`
	File    string       `json:"file"`
	Line    int          `json:"line"`
	Snippet string       `json:"snippet"`
}

// EvidenceType categorizes evidence sources.
type EvidenceType string

const (
	EvidencePackageManifest  EvidenceType = "package-manifest"
	EvidenceImport           EvidenceType = "import"
	EvidenceConnectionString EvidenceType = "connection-string"
	EvidenceEnvVariable      EvidenceType = "env-variable"
)

// TeamPractices contains conventions extracted from existing IaC and configuration.
type TeamPractices struct {
	NamingConvention NamingPattern     `json:"namingConvention,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	Environment      string            `json:"environment,omitempty"`
	Region           string            `json:"region,omitempty"`

	// Security preferences
	EncryptionEnabled bool `json:"encryptionEnabled"`
	PrivateNetworking bool `json:"privateNetworking"`

	// Sizing hints
	DefaultTier string `json:"defaultTier,omitempty"`

	// Sources
	ExtractedFrom []PracticeSource `json:"extractedFrom"`
}

// NamingPattern describes a detected naming convention.
type NamingPattern struct {
	Pattern    string   `json:"pattern"` // e.g., "{env}-{app}-{resource}"
	Examples   []string `json:"examples"`
	Confidence float64  `json:"confidence"`
}

// PracticeSource indicates where a practice was extracted from.
type PracticeSource struct {
	Type      PracticeSourceType `json:"type"`
	FilePath  string             `json:"filePath"`
	Resources []IaCResource      `json:"resources,omitempty"` // Resources defined in the file
	Providers []string           `json:"providers,omitempty"` // Providers used (terraform)
	Summary   string             `json:"summary,omitempty"`   // Brief summary of the file
}

// IaCResource represents a resource defined in an IaC file.
type IaCResource struct {
	Type string `json:"type"` // e.g., "azurerm_resource_group", "Microsoft.Storage/storageAccounts"
	Name string `json:"name"` // Logical name in the IaC
}

// PracticeSourceType categorizes practice sources.
type PracticeSourceType string

const (
	SourceTerraform  PracticeSourceType = "terraform"
	SourceBicep      PracticeSourceType = "bicep"
	SourceARM        PracticeSourceType = "arm"
	SourceKubernetes PracticeSourceType = "kubernetes"
	SourceEnvFile    PracticeSourceType = "env-file"
)

// ResourceTypeMapping maps a detected dependency to a Radius Resource Type.
type ResourceTypeMapping struct {
	DependencyID string       `json:"dependencyId"`
	ResourceType ResourceType `json:"resourceType"`
	MatchSource  MatchSource  `json:"matchSource"`
	Confidence   float64      `json:"confidence"`
}

// ResourceType represents a Radius Resource Type definition.
type ResourceType struct {
	Name       string                 `json:"name"`                 // e.g., "Applications.Datastores/postgreSqlDatabases"
	APIVersion string                 `json:"apiVersion"`           // e.g., "2023-10-01-preview"
	Properties map[string]interface{} `json:"properties,omitempty"` // Default values
	Schema     string                 `json:"schema,omitempty"`     // JSON Schema URL
}

// MatchSource indicates how a Resource Type was matched.
type MatchSource string

const (
	MatchCatalog     MatchSource = "catalog"      // Matched from built-in catalog
	MatchContrib     MatchSource = "contrib"      // Matched from resource-types-contrib
	MatchInferred    MatchSource = "inferred"     // Inferred from dependency type
	MatchUserDefined MatchSource = "user-defined" // From user configuration
)

// RecipeMatch represents a recipe that can provision a detected dependency.
type RecipeMatch struct {
	DependencyID string   `json:"dependencyId"`
	Recipe       Recipe   `json:"recipe"`
	Score        float64  `json:"score"` // 0.0-1.0 match quality
	MatchReasons []string `json:"matchReasons"`
}

// Recipe represents an IaC implementation for provisioning infrastructure.
type Recipe struct {
	Name           string            `json:"name"`
	SourceType     RecipeSourceType  `json:"sourceType"`
	SourceLocation string            `json:"sourceLocation"` // Registry path or git URL
	Version        string            `json:"version,omitempty"`
	Description    string            `json:"description,omitempty"`
	Provider       string            `json:"provider,omitempty"` // azure, aws, gcp
	Parameters     []RecipeParameter `json:"parameters"`
}

// RecipeSourceType categorizes recipe sources.
type RecipeSourceType string

const (
	RecipeSourceAVM       RecipeSourceType = "avm"
	RecipeSourceTerraform RecipeSourceType = "terraform-registry"
	RecipeSourceGit       RecipeSourceType = "git"
	RecipeSourceLocal     RecipeSourceType = "local"
)

// RecipeParameter describes a parameter for a recipe.
type RecipeParameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

// DiscoveryWarning represents a warning generated during analysis.
type DiscoveryWarning struct {
	Level   WarningLevel `json:"level"`
	Code    string       `json:"code"`
	Message string       `json:"message"`
	File    string       `json:"file,omitempty"`
	Line    int          `json:"line,omitempty"`
}

// WarningLevel categorizes warning severity.
type WarningLevel string

const (
	WarningInfo    WarningLevel = "info"
	WarningWarning WarningLevel = "warning"
	WarningError   WarningLevel = "error"
)

// DiscoveryOptions configures the discovery process.
type DiscoveryOptions struct {
	ProjectPath   string     `json:"projectPath"`
	Languages     []Language `json:"languages,omitempty"`     // Auto-detect if empty
	MinConfidence float64    `json:"minConfidence,omitempty"` // Default 0.5
	IncludeTests  bool       `json:"includeTests,omitempty"`
	Verbose       bool       `json:"verbose,omitempty"`
}

// GenerateOptions configures the app.bicep generation process.
type GenerateOptions struct {
	DiscoveryResult *DiscoveryResult `json:"discoveryResult"`
	OutputPath      string           `json:"outputPath,omitempty"` // Default ./radius/app.bicep
	Environment     string           `json:"environment,omitempty"`
	IncludeRecipes  bool             `json:"includeRecipes"`
	IncludeComments bool             `json:"includeComments"`
	DryRun          bool             `json:"dryRun"`
	Force           bool             `json:"force"`
}
