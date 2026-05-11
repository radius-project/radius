# Data Model: Git App Graph Preview

**Feature Branch**: `001-git-app-graph-preview`
**Date**: February 4, 2026

## Core Entities

### AppGraph

The root container for all graph data. This is the primary structure serialized to JSON.

```go
// AppGraph represents the complete application topology extracted from Bicep files.
// This is the primary output of the `rad app graph <file.bicep>` command.
type AppGraph struct {
    // Metadata contains generation context and provenance information
    Metadata AppGraphMetadata `json:"metadata"`
    
    // Resources contains all resource nodes in the application
    Resources []AppGraphResource `json:"resources"`
    
    // Connections contains all edges between resources
    Connections []AppGraphConnection `json:"connections"`
}
```

### AppGraphMetadata

Provenance and staleness detection information.

```go
// AppGraphMetadata contains generation context for the app graph.
type AppGraphMetadata struct {
    // GeneratedAt is the UTC timestamp when this graph was generated
    GeneratedAt time.Time `json:"generatedAt"`
    
    // RadiusCliVersion is the version of rad CLI used to generate this graph
    RadiusCliVersion string `json:"radiusCliVersion"`
    
    // SourceFiles lists all Bicep files that contributed to this graph
    SourceFiles []string `json:"sourceFiles"`
    
    // SourceHash is a SHA256 hash of all source files for staleness detection
    SourceHash string `json:"sourceHash"`
    
    // GitCommit is the current git commit SHA (if in a git repository)
    GitCommit string `json:"gitCommit,omitempty"`
}
```

### AppGraphResource

A single resource node in the application graph.

```go
// AppGraphResource represents a single resource in the application topology.
type AppGraphResource struct {
    // ID is the fully qualified Radius resource ID
    // Format: /planes/radius/local/resourceGroups/{rg}/providers/{type}/{name}
    ID string `json:"id"`
    
    // Name is the human-readable resource name
    Name string `json:"name"`
    
    // Type is the resource type (e.g., Applications.Core/containers)
    Type string `json:"type"`
    
    // SourceLocation indicates where this resource is defined
    SourceLocation SourceLocation `json:"sourceLocation"`
    
    // GitInfo contains git metadata for this resource (optional)
    GitInfo *GitInfo `json:"gitInfo,omitempty"`
    
    // Properties contains type-specific resource configuration
    // Stored as map for flexibility across resource types
    Properties map[string]any `json:"properties,omitempty"`
}
```

### SourceLocation

Source file tracking for each resource.

```go
// SourceLocation indicates the Bicep source file and line where a resource is defined.
type SourceLocation struct {
    // File is the path to the Bicep file (relative to repo root)
    File string `json:"file"`
    
    // Line is the 1-based line number where the resource begins
    Line int `json:"line"`
    
    // Module is the module path if this resource is from an imported module
    Module string `json:"module,omitempty"`
}
```

### GitInfo

Git metadata for a resource, populated via `git blame`.

```go
// GitInfo contains git commit information for a resource.
type GitInfo struct {
    // CommitSHA is the full commit hash that last modified this resource
    CommitSHA string `json:"commitSha"`
    
    // CommitShort is the abbreviated commit hash for display
    CommitShort string `json:"commitShort"`
    
    // Author is the commit author email
    Author string `json:"author"`
    
    // Date is the commit timestamp in RFC3339 format
    Date time.Time `json:"date"`
    
    // Message is the commit message (first line only)
    Message string `json:"message"`
    
    // Uncommitted indicates this resource has uncommitted changes
    Uncommitted bool `json:"uncommitted,omitempty"`
}
```

### AppGraphConnection

An edge between two resources representing a dependency or data flow.

```go
// AppGraphConnection represents a directed edge between two resources.
type AppGraphConnection struct {
    // SourceID is the resource ID where the connection originates
    SourceID string `json:"sourceId"`
    
    // TargetID is the resource ID where the connection terminates
    TargetID string `json:"targetId"`
    
    // Type indicates the kind of connection
    Type ConnectionType `json:"type"`
}

// ConnectionType enumerates the kinds of resource connections.
type ConnectionType string

const (
    // ConnectionTypeConnection represents a direct connection (e.g., container to database)
    ConnectionTypeConnection ConnectionType = "connection"
    
    // ConnectionTypeRoute represents a gateway route to a destination
    ConnectionTypeRoute ConnectionType = "route"
    
    // ConnectionTypeDependsOn represents an explicit dependsOn relationship
    ConnectionTypeDependsOn ConnectionType = "dependsOn"
)
```

## Diff Entities

### GraphDiff

The result of comparing two app graphs.

```go
// GraphDiff represents the differences between two app graphs.
type GraphDiff struct {
    // BaseCommit is the commit SHA of the base graph (optional)
    BaseCommit string `json:"baseCommit,omitempty"`
    
    // HeadCommit is the commit SHA of the head graph (optional)
    HeadCommit string `json:"headCommit,omitempty"`
    
    // AddedResources are resources present in head but not in base
    AddedResources []AppGraphResource `json:"addedResources"`
    
    // RemovedResources are resources present in base but not in head
    RemovedResources []AppGraphResource `json:"removedResources"`
    
    // ModifiedResources are resources with changed properties
    ModifiedResources []ResourceDiff `json:"modifiedResources"`
    
    // AddedConnections are connections present in head but not in base
    AddedConnections []AppGraphConnection `json:"addedConnections"`
    
    // RemovedConnections are connections present in base but not in head
    RemovedConnections []AppGraphConnection `json:"removedConnections"`
    
    // Summary provides a human-readable overview
    Summary DiffSummary `json:"summary"`
}

// ResourceDiff captures changes to a single resource.
type ResourceDiff struct {
    // ID is the resource ID (same in both base and head)
    ID string `json:"id"`
    
    // Name is the resource name
    Name string `json:"name"`
    
    // Type is the resource type
    Type string `json:"type"`
    
    // ChangedProperties lists the property paths that changed
    ChangedProperties []PropertyChange `json:"changedProperties"`
}

// PropertyChange describes a single property modification.
type PropertyChange struct {
    // Path is the JSON path to the changed property (e.g., "properties.container.image")
    Path string `json:"path"`
    
    // OldValue is the value in the base graph
    OldValue any `json:"oldValue,omitempty"`
    
    // NewValue is the value in the head graph
    NewValue any `json:"newValue,omitempty"`
}

// DiffSummary provides aggregate statistics for the diff.
type DiffSummary struct {
    TotalChanges       int `json:"totalChanges"`
    ResourcesAdded     int `json:"resourcesAdded"`
    ResourcesRemoved   int `json:"resourcesRemoved"`
    ResourcesModified  int `json:"resourcesModified"`
    ConnectionsAdded   int `json:"connectionsAdded"`
    ConnectionsRemoved int `json:"connectionsRemoved"`
}
```

## Entity Relationships

```
┌─────────────────┐
│    AppGraph     │
├─────────────────┤
│ Metadata        │──────┐
│ Resources[]     │──┐   │
│ Connections[]   │  │   │
└─────────────────┘  │   │
                     │   │
     ┌───────────────┘   │
     ▼                   ▼
┌─────────────────┐  ┌──────────────────┐
│ AppGraphResource│  │ AppGraphMetadata │
├─────────────────┤  ├──────────────────┤
│ ID              │  │ GeneratedAt      │
│ Name            │  │ RadiusCliVersion │
│ Type            │  │ SourceFiles[]    │
│ SourceLocation  │──┐ SourceHash       │
│ GitInfo?        │  │ GitCommit?       │
│ Properties      │  └──────────────────┘
└─────────────────┘
         │
    ┌────┴────┐
    ▼         ▼
┌──────────┐ ┌─────────┐
│SourceLoc │ │ GitInfo │
├──────────┤ ├─────────┤
│ File     │ │CommitSHA│
│ Line     │ │Author   │
│ Module?  │ │Date     │
└──────────┘ │Message  │
             └─────────┘

┌────────────────────┐
│ AppGraphConnection │
├────────────────────┤
│ SourceID           │───────► AppGraphResource.ID
│ TargetID           │───────► AppGraphResource.ID
│ Type               │
└────────────────────┘
```

## Validation Rules

### AppGraph
- `Metadata.GeneratedAt` MUST be a valid UTC timestamp
- `Metadata.SourceFiles` MUST contain at least one file
- `Metadata.SourceHash` MUST be a valid SHA256 hash (64 hex chars)
- `Resources` MAY be empty for an empty Bicep file

### AppGraphResource
- `ID` MUST be a valid Radius resource ID format
- `Name` MUST be non-empty
- `Type` MUST be a recognized resource type or follow ARM type pattern
- `SourceLocation.File` MUST be a valid relative file path
- `SourceLocation.Line` MUST be >= 1

### AppGraphConnection
- `SourceID` MUST reference an existing resource ID
- `TargetID` MUST reference an existing resource ID
- `Type` MUST be one of the defined ConnectionType values

### GraphDiff
- All resource references in Added/Removed/Modified MUST be valid
- `Summary` counts MUST match the actual array lengths

## State Transitions

Resources can be in the following states relative to git:

```
┌──────────────┐
│  Uncommitted │ ───(git add)───► ┌──────────┐
└──────────────┘                  │  Staged  │
                                  └──────────┘
                                       │
                                 (git commit)
                                       │
                                       ▼
┌──────────────┐                 ┌──────────┐
│   Modified   │ ◄──(edit file)──│Committed │
└──────────────┘                 └──────────┘
       │
  (git add + commit)
       │
       ▼
  ┌──────────┐
  │Committed │ (new SHA)
  └──────────┘
```

Graph states:
- **Current**: Generated from current working directory files
- **Committed**: Exists in `.radius/app-graph.json` in git history
- **Stale**: Committed graph doesn't match current Bicep files (detected via sourceHash)

## JSON Schema Example

```json
{
  "metadata": {
    "generatedAt": "2026-01-30T10:15:00Z",
    "radiusCliVersion": "0.35.0",
    "sourceFiles": [
      "app.bicep",
      "modules/database.bicep"
    ],
    "sourceHash": "sha256:7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730",
    "gitCommit": "abc123def456"
  },
  "resources": [
    {
      "id": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/frontend",
      "name": "frontend",
      "type": "Applications.Core/containers",
      "sourceLocation": {
        "file": "app.bicep",
        "line": 12
      },
      "gitInfo": {
        "commitSha": "abc123def456789012345678901234567890abcd",
        "commitShort": "abc123d",
        "author": "dev@example.com",
        "date": "2026-01-29T14:30:00Z",
        "message": "Add frontend container"
      },
      "properties": {
        "container": {
          "image": "myapp/frontend:v1.2.3"
        }
      }
    },
    {
      "id": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/backend",
      "name": "backend",
      "type": "Applications.Core/containers",
      "sourceLocation": {
        "file": "app.bicep",
        "line": 28
      },
      "gitInfo": {
        "commitSha": "def456abc789012345678901234567890abcdef01",
        "commitShort": "def456a",
        "author": "dev@example.com",
        "date": "2026-01-28T09:15:00Z",
        "message": "Add backend service"
      }
    },
    {
      "id": "/planes/radius/local/resourceGroups/default/providers/Applications.Datastores/redisCaches/cache",
      "name": "cache",
      "type": "Applications.Datastores/redisCaches",
      "sourceLocation": {
        "file": "modules/database.bicep",
        "line": 5,
        "module": "modules/database.bicep"
      },
      "gitInfo": {
        "commitSha": "789abc012def345678901234567890abcdef0123",
        "commitShort": "789abc0",
        "author": "ops@example.com",
        "date": "2026-01-27T16:45:00Z",
        "message": "Add Redis cache for session storage"
      }
    }
  ],
  "connections": [
    {
      "sourceId": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/frontend",
      "targetId": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/backend",
      "type": "connection"
    },
    {
      "sourceId": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/backend",
      "targetId": "/planes/radius/local/resourceGroups/default/providers/Applications.Datastores/redisCaches/cache",
      "type": "connection"
    }
  ]
}
```
