# Data Model: Automatic Application Discovery

**Feature**: 001-auto-app-discovery  
**Date**: January 29, 2026

## Core Entities

### 1. DiscoveryResult

The top-level result from analyzing a codebase.

```go
type DiscoveryResult struct {
    // Metadata
    ProjectPath   string    `json:"projectPath"`
    AnalyzedAt    time.Time `json:"analyzedAt"`
    AnalyzerVersion string  `json:"analyzerVersion"`
    
    // Discovered elements
    Services      []Service          `json:"services"`
    Dependencies  []DetectedDependency `json:"dependencies"`
    Practices     TeamPractices      `json:"practices"`
    
    // Generation candidates
    ResourceTypes []ResourceTypeMapping `json:"resourceTypes"`
    Recipes       []RecipeMatch         `json:"recipes"`
    
    // Validation
    Warnings      []DiscoveryWarning `json:"warnings"`
    Confidence    float64            `json:"confidence"` // 0.0-1.0
}
```

### 2. Service

A deployable unit detected in the codebase.

```go
type Service struct {
    Name          string            `json:"name"`
    Path          string            `json:"path"`
    Language      Language          `json:"language"`
    Framework     string            `json:"framework,omitempty"`
    
    // Container info
    Dockerfile    string            `json:"dockerfile,omitempty"`
    ExposedPorts  []int             `json:"exposedPorts,omitempty"`
    
    // Entry point
    EntryPoint    EntryPoint        `json:"entryPoint"`
    
    // Dependencies this service uses
    DependencyIDs []string          `json:"dependencyIds"`
    
    // Detection confidence
    Confidence    float64           `json:"confidence"`
    Evidence      []Evidence        `json:"evidence"`
}

type EntryPoint struct {
    Type    EntryPointType `json:"type"` // dockerfile, main, script
    File    string         `json:"file"`
    Command string         `json:"command,omitempty"`
}

type EntryPointType string
const (
    EntryPointDockerfile EntryPointType = "dockerfile"
    EntryPointMain       EntryPointType = "main"
    EntryPointScript     EntryPointType = "script"
)

type Language string
const (
    LanguagePython     Language = "python"
    LanguageJavaScript Language = "javascript"
    LanguageTypeScript Language = "typescript"
    LanguageGo         Language = "go"
    LanguageJava       Language = "java"
    LanguageCSharp     Language = "csharp"
)
```

### 3. DetectedDependency

An infrastructure dependency found in source code.

```go
type DetectedDependency struct {
    ID            string           `json:"id"`
    Type          DependencyType   `json:"type"`
    Name          string           `json:"name"`
    
    // Detection details
    Library       string           `json:"library"`
    Version       string           `json:"version,omitempty"`
    Confidence    float64          `json:"confidence"`
    Evidence      []Evidence       `json:"evidence"`
    
    // Connection info extracted
    ConnectionEnv string           `json:"connectionEnv,omitempty"`
    DefaultPort   int              `json:"defaultPort,omitempty"`
    
    // Services that use this dependency
    UsedBy        []string         `json:"usedBy"`
}

type DependencyType string
const (
    DependencyPostgreSQL DependencyType = "postgresql"
    DependencyMySQL      DependencyType = "mysql"
    DependencyMongoDB    DependencyType = "mongodb"
    DependencyRedis      DependencyType = "redis"
    DependencyRabbitMQ   DependencyType = "rabbitmq"
    DependencyKafka      DependencyType = "kafka"
    DependencyAzureBlob  DependencyType = "azure-blob"
    DependencyS3         DependencyType = "s3"
    DependencyUnknown    DependencyType = "unknown"
)

type Evidence struct {
    Type     EvidenceType `json:"type"`
    File     string       `json:"file"`
    Line     int          `json:"line"`
    Snippet  string       `json:"snippet"`
}

type EvidenceType string
const (
    EvidencePackageManifest EvidenceType = "package-manifest"
    EvidenceImport          EvidenceType = "import"
    EvidenceConnectionString EvidenceType = "connection-string"
    EvidenceEnvVariable     EvidenceType = "env-variable"
)
```

### 4. TeamPractices

Conventions extracted from existing IaC and configuration.

```go
type TeamPractices struct {
    NamingConvention  NamingPattern        `json:"namingConvention,omitempty"`
    Tags              map[string]string    `json:"tags,omitempty"`
    Environment       string               `json:"environment,omitempty"`
    Region            string               `json:"region,omitempty"`
    
    // Security preferences
    EncryptionEnabled bool                 `json:"encryptionEnabled"`
    PrivateNetworking bool                 `json:"privateNetworking"`
    
    // Sizing hints
    DefaultTier       string               `json:"defaultTier,omitempty"`
    
    // Sources
    ExtractedFrom     []PracticeSource     `json:"extractedFrom"`
}

type NamingPattern struct {
    Pattern     string   `json:"pattern"`  // e.g., "{env}-{app}-{resource}"
    Examples    []string `json:"examples"`
    Confidence  float64  `json:"confidence"`
}

type PracticeSource struct {
    Type     PracticeSourceType `json:"type"`
    FilePath string             `json:"filePath"`
}

type PracticeSourceType string
const (
    SourceTerraform   PracticeSourceType = "terraform"
    SourceBicep       PracticeSourceType = "bicep"
    SourceARM         PracticeSourceType = "arm"
    SourceKubernetes  PracticeSourceType = "kubernetes"
    SourceEnvFile     PracticeSourceType = "env-file"
)
```

### 5. ResourceTypeMapping

Maps detected dependency to Radius Resource Type.

```go
type ResourceTypeMapping struct {
    DependencyID   string         `json:"dependencyId"`
    ResourceType   ResourceType   `json:"resourceType"`
    MatchSource    MatchSource    `json:"matchSource"`
    Confidence     float64        `json:"confidence"`
}

type ResourceType struct {
    Name       string                 `json:"name"`       // e.g., "Applications.Datastores/postgreSqlDatabases"
    APIVersion string                 `json:"apiVersion"` // e.g., "2023-10-01-preview"
    Properties map[string]interface{} `json:"properties"` // Default values
    Schema     string                 `json:"schema,omitempty"` // JSON Schema URL
}

type MatchSource string
const (
    MatchCatalog    MatchSource = "catalog"    // Matched from built-in catalog
    MatchInferred   MatchSource = "inferred"   // Inferred from dependency type
    MatchUserDefined MatchSource = "user-defined" // From user configuration
)
```

### 6. RecipeMatch

A recipe that can provision a detected dependency.

```go
type RecipeMatch struct {
    DependencyID  string        `json:"dependencyId"`
    Recipe        Recipe        `json:"recipe"`
    Score         float64       `json:"score"` // 0.0-1.0 match quality
    MatchReasons  []string      `json:"matchReasons"`
}

type Recipe struct {
    Name           string            `json:"name"`
    SourceType     RecipeSourceType  `json:"sourceType"`
    SourceLocation string            `json:"sourceLocation"` // Registry path or git URL
    Version        string            `json:"version,omitempty"`
    
    // Metadata
    Description    string            `json:"description,omitempty"`
    Provider       string            `json:"provider,omitempty"` // azure, aws, gcp
    
    // Parameters
    Parameters     []RecipeParameter `json:"parameters"`
}

type RecipeSourceType string
const (
    RecipeSourceAVM        RecipeSourceType = "avm"
    RecipeSourceTerraform  RecipeSourceType = "terraform-registry"
    RecipeSourceGit        RecipeSourceType = "git"
    RecipeSourceLocal      RecipeSourceType = "local"
)

type RecipeParameter struct {
    Name         string      `json:"name"`
    Type         string      `json:"type"`
    Description  string      `json:"description,omitempty"`
    Required     bool        `json:"required"`
    Default      interface{} `json:"default,omitempty"`
}
```

### 7. DiscoveryWarning

Warnings generated during analysis.

```go
type DiscoveryWarning struct {
    Level    WarningLevel `json:"level"`
    Code     string       `json:"code"`
    Message  string       `json:"message"`
    File     string       `json:"file,omitempty"`
    Line     int          `json:"line,omitempty"`
}

type WarningLevel string
const (
    WarningInfo    WarningLevel = "info"
    WarningWarning WarningLevel = "warning"
    WarningError   WarningLevel = "error"
)
```

---

## Relationships

```
┌─────────────────┐
│ DiscoveryResult │
└────────┬────────┘
         │
    ┌────┴────┬────────────┬─────────────┐
    │         │            │             │
    ▼         ▼            ▼             ▼
┌───────┐ ┌──────────────┐ ┌─────────────┐ ┌───────────────┐
│Service│ │Detected      │ │TeamPractices│ │ResourceType   │
│       │ │Dependency    │ │             │ │Mapping        │
└───┬───┘ └──────┬───────┘ └─────────────┘ └───────────────┘
    │            │                               │
    │   usedBy   │                               │
    └────────────┘                               │
                                                 ▼
                                         ┌─────────────┐
                                         │RecipeMatch  │
                                         └─────────────┘
```

---

## State Transitions

### Dependency Lifecycle

```
[Detected] → [Mapped to Resource Type] → [Recipe Matched] → [Included in App Definition]
    │               │                          │                      │
    └── Low confidence ──► [Needs User Input] ──► [Confirmed] ────────┘
```

### Discovery Session Lifecycle

```
[Created] → [Analyzing] → [Complete] → [Exported]
    │                         │
    └── Error ──────────► [Failed]
```

---

## Validation Rules

| Entity | Field | Rule |
|--------|-------|------|
| Service | Name | Non-empty, valid identifier |
| Service | Confidence | 0.0 ≤ value ≤ 1.0 |
| DetectedDependency | ID | Unique within result |
| DetectedDependency | Type | Valid DependencyType enum |
| ResourceType | Name | Matches `Applications.{Provider}/{resourceType}` pattern |
| Recipe | SourceLocation | Valid URL or file path |
| Evidence | Line | ≥ 1 |
