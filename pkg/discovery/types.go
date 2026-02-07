// Package discovery provides automatic application discovery for Radius.
// It analyzes codebases to detect infrastructure dependencies, deployable services,
// and team practices—then generates Resource Types and matches Recipes to produce
// deployable app.bicep files.
package discovery

import (
	"github.com/radius-project/radius/pkg/discovery/dtypes"
)

// Re-export all types from dtypes package for convenience.
// This allows consumers to import just "discovery" instead of "discovery/dtypes".

// DiscoveryResult is the top-level result from analyzing a codebase.
type DiscoveryResult = dtypes.DiscoveryResult

// Service represents a deployable unit detected in the codebase.
type Service = dtypes.Service

// EntryPoint describes how a service is started.
type EntryPoint = dtypes.EntryPoint

// EntryPointType indicates the type of entry point.
type EntryPointType = dtypes.EntryPointType

// Language represents a programming language.
type Language = dtypes.Language

// DetectedDependency represents an infrastructure dependency found in source code.
type DetectedDependency = dtypes.DetectedDependency

// DependencyType categorizes infrastructure dependencies.
type DependencyType = dtypes.DependencyType

// Evidence provides proof of a detection.
type Evidence = dtypes.Evidence

// EvidenceType categorizes evidence sources.
type EvidenceType = dtypes.EvidenceType

// TeamPractices contains conventions extracted from existing IaC and configuration.
type TeamPractices = dtypes.TeamPractices

// NamingPattern describes a detected naming convention.
type NamingPattern = dtypes.NamingPattern

// PracticeSource indicates where a practice was extracted from.
type PracticeSource = dtypes.PracticeSource

// PracticeSourceType categorizes practice sources.
type PracticeSourceType = dtypes.PracticeSourceType

// ResourceTypeMapping maps a detected dependency to a Radius Resource Type.
type ResourceTypeMapping = dtypes.ResourceTypeMapping

// ResourceType represents a Radius Resource Type definition.
type ResourceType = dtypes.ResourceType

// MatchSource indicates how a Resource Type was matched.
type MatchSource = dtypes.MatchSource

// RecipeMatch represents a recipe that can provision a detected dependency.
type RecipeMatch = dtypes.RecipeMatch

// Recipe represents an IaC implementation for provisioning infrastructure.
type Recipe = dtypes.Recipe

// RecipeSourceType categorizes recipe sources.
type RecipeSourceType = dtypes.RecipeSourceType

// RecipeParameter describes a parameter for a recipe.
type RecipeParameter = dtypes.RecipeParameter

// DiscoveryWarning represents a warning generated during analysis.
type DiscoveryWarning = dtypes.DiscoveryWarning

// WarningLevel categorizes warning severity.
type WarningLevel = dtypes.WarningLevel

// DiscoveryOptions configures the discovery process.
type DiscoveryOptions = dtypes.DiscoveryOptions

// GenerateOptions configures the app.bicep generation process.
type GenerateOptions = dtypes.GenerateOptions

// Re-export constants.
const (
	// Entry point types
	EntryPointDockerfile = dtypes.EntryPointDockerfile
	EntryPointMain       = dtypes.EntryPointMain
	EntryPointScript     = dtypes.EntryPointScript

	// Languages
	LanguagePython     = dtypes.LanguagePython
	LanguageJavaScript = dtypes.LanguageJavaScript
	LanguageTypeScript = dtypes.LanguageTypeScript
	LanguageGo         = dtypes.LanguageGo
	LanguageJava       = dtypes.LanguageJava
	LanguageCSharp     = dtypes.LanguageCSharp

	// Dependency types
	DependencyPostgreSQL = dtypes.DependencyPostgreSQL
	DependencyMySQL      = dtypes.DependencyMySQL
	DependencyMongoDB    = dtypes.DependencyMongoDB
	DependencyRedis      = dtypes.DependencyRedis
	DependencyRabbitMQ   = dtypes.DependencyRabbitMQ
	DependencyKafka      = dtypes.DependencyKafka
	DependencyAzureBlob  = dtypes.DependencyAzureBlob
	DependencyS3         = dtypes.DependencyS3
	DependencyUnknown    = dtypes.DependencyUnknown

	// Evidence types
	EvidencePackageManifest  = dtypes.EvidencePackageManifest
	EvidenceImport           = dtypes.EvidenceImport
	EvidenceConnectionString = dtypes.EvidenceConnectionString
	EvidenceEnvVariable      = dtypes.EvidenceEnvVariable

	// Practice source types
	SourceTerraform  = dtypes.SourceTerraform
	SourceBicep      = dtypes.SourceBicep
	SourceARM        = dtypes.SourceARM
	SourceKubernetes = dtypes.SourceKubernetes
	SourceEnvFile    = dtypes.SourceEnvFile

	// Match sources
	MatchCatalog     = dtypes.MatchCatalog
	MatchInferred    = dtypes.MatchInferred
	MatchUserDefined = dtypes.MatchUserDefined

	// Recipe source types
	RecipeSourceAVM       = dtypes.RecipeSourceAVM
	RecipeSourceTerraform = dtypes.RecipeSourceTerraform
	RecipeSourceGit       = dtypes.RecipeSourceGit
	RecipeSourceLocal     = dtypes.RecipeSourceLocal

	// Warning levels
	WarningInfo    = dtypes.WarningInfo
	WarningWarning = dtypes.WarningWarning
	WarningError   = dtypes.WarningError
)
