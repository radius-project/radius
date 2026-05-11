# Research: Git App Graph Preview

**Feature Branch**: `001-git-app-graph-preview`
**Date**: February 4, 2026

## Research Tasks

### 1. Radius CLI Architecture

**Decision**: Extend existing `rad app graph` command in `pkg/cli/cmd/app/graph/`

**Rationale**: 
- The existing `rad app graph <appname>` command already follows established CLI patterns
- Command structure uses Cobra framework with Runner pattern (`NewCommand()` + `Runner.Run()`)
- Output formatting is handled through `pkg/cli/output/` utilities
- Adding file-based input preserves conceptual consistency ("both are app graphs")

**Alternatives Considered**:
- New `rad graph` top-level command: Rejected (breaks CLI hierarchy, less discoverable)
- New `rad bicep graph` command: Rejected (conceptually these are both "app graphs", just from different sources)

### 2. Existing App Graph Data Structures

**Decision**: Extend `ApplicationGraphResponse` and related types in `pkg/corerp/api/`

**Rationale**:
- Existing types (`ApplicationGraphResource`, `ApplicationGraphConnection`) capture most needed fields
- Adding git metadata fields is additive and backward-compatible
- Deterministic output requires adding `sourceHash`, `sourceFile`, `sourceLine` metadata
- Types are already JSON-serializable via auto-generated marshalling code

**Existing Structure** (from `zz_generated_models.go`):
```go
type ApplicationGraphResponse struct {
    Resources []*ApplicationGraphResource
}

type ApplicationGraphResource struct {
    ID                *string
    Name              *string
    Type              *string
    ProvisioningState *string
    Connections       []*ApplicationGraphConnection
    OutputResources   []*ApplicationGraphOutputResource
}
```

**Additions Needed**:
- `GitInfo` struct: commit SHA, author, date, message
- `SourceFile`, `SourceLine` for Bicep source tracking
- `Metadata` struct: generatedAt, sourceFiles, sourceHash, radiusCliVersion

### 3. Bicep Parsing Approach

**Decision**: Use official Bicep CLI for compilation, then parse ARM JSON

**Rationale** (per Constitution Principle VII - Simplicity Over Cleverness):
- Bicep CLI provides `bicep build --stdout` to compile to ARM JSON
- Radius already has Bicep CLI integration in `pkg/cli/bicep/`
- ARM JSON is stable and well-documented; parsing it avoids Bicep grammar complexity
- External modules are resolved by Bicep CLI, not our code

**Alternatives Considered**:
- Custom Bicep parser: Rejected (complex, maintenance burden, grammar changes)
- ANTLR-based parser: Rejected (over-engineering for this use case)
- Use Bicep language server: Rejected (heavyweight, overkill for static analysis)

**Implementation Pattern**:
```go
// Existing pattern in pkg/cli/bicep/types.go
func (impl *Impl) PrepareTemplate(filePath string) (map[string]any, error) {
    args := []string{"build", "--stdout", filePath}
    // Execute bicep CLI and parse JSON output
}
```

### 4. Graph Extraction from ARM JSON

**Decision**: Extract resources and connections from compiled ARM JSON template

**Rationale**:
- ARM JSON has stable schema with `resources` array
- Radius resources use `connections` and `routes` properties for relationships
- Existing graph logic in `pkg/corerp/frontend/controller/applications/graph_util.go` shows patterns

**Key Extraction Points**:
1. `resources[].type` - Resource type identification
2. `resources[].name` - Resource name (may contain expressions)
3. `resources[].properties.connections` - Direct connections
4. `resources[].properties.routes` - Gateway routes
5. `resources[].dependsOn` - Explicit dependencies

### 5. Git Integration

**Decision**: Use `git log` and `git blame` via exec, not library

**Rationale**:
- Git is universally available in development environments
- Shell commands are simpler than CGo bindings to libgit2
- Radius already uses exec patterns for Bicep CLI
- Graceful degradation when not in git repo or shallow clone

**Commands Needed**:
- `git blame -l -e <file>` - Get commit SHA per line
- `git log -1 --format='%H|%ae|%aI|%s' <sha>` - Get commit metadata
- `git rev-parse --show-toplevel` - Detect git repo root

### 6. GitHub Action Architecture

**Decision**: Lightweight Action that reads committed JSON from git history; no graph generation

**Rationale** (per spec Committed Artifact Model):
- Action only needs git and jq, not Bicep/Radius tooling
- Works in forks without special secrets
- Fast execution (JSON comparison vs. full compilation)
- Reproducible (graph captured at commit time)

**Implementation**:
1. Checkout base and head commits
2. Read `.radius/app-graph.json` from each
3. Compute diff using JSON comparison
4. Render diff as Markdown with Mermaid diagrams
5. Post/update PR comment using `peter-evans/create-or-update-comment`

**Alternatives Considered**:
- GitHub App: Rejected for MVP (more complex setup, centralized management)
- Generate graph in CI: Rejected (requires Bicep tooling, slower, less reproducible)

### 7. Diff Computation Strategy

**Decision**: JSON-based diffing with semantic resource comparison

**Rationale**:
- JSON is deterministic when keys are sorted
- Resource ID provides stable identity across commits
- Diff should show added/removed/modified at resource level, not line level

**Diff Algorithm**:
1. Parse base and head JSON
2. Create resource map keyed by ID
3. Compare:
   - Added: ID in head but not base
   - Removed: ID in base but not head
   - Modified: ID in both but properties differ
4. For connections: compare by (sourceID, targetID) tuples

### 8. Output Formats

**Decision**: JSON canonical, Markdown additive with embedded Mermaid

**Rationale** (per spec):
- JSON is machine-readable, deterministic, diffable
- Markdown renders in GitHub UI without additional tooling
- Mermaid diagrams supported natively by GitHub
- Separation allows different consumers (CI vs. humans)

**JSON Schema**:
```json
{
  "metadata": {
    "generatedAt": "2026-01-30T10:15:00Z",
    "sourceFiles": ["app.bicep", "modules/database.bicep"],
    "sourceHash": "sha256:abc123...",
    "radiusCliVersion": "0.35.0"
  },
  "resources": [...],
  "connections": [...]
}
```

**Mermaid Shapes** (per spec):
- Containers: rectangles (`[name]`)
- Gateways: diamonds (`{name}`)
- Databases: cylinders (`[(name)]`)

### 9. Platform Abstraction for Future Integrations

**Decision**: Separate diff computation from rendering; platform-specific rendering layer

**Rationale** (per user input: "design choices today won't limit us in the future"):
- Core diff logic (JSON comparison) is platform-agnostic
- GitHub-specific code isolated to:
  - GitHub Action (workflow YAML)
  - Markdown/Mermaid rendering
  - PR comment posting
- Future GitLab integration would only need new rendering layer

**Architecture**:
```
┌─────────────────────────────────────────────────────────────────┐
│                        Platform-Agnostic                        │
├─────────────────────────────────────────────────────────────────┤
│  CLI (rad app graph)  │  JSON Schema  │  Diff Computation       │
│  Git Integration      │  Data Model   │  Core Rendering (MD)    │
└─────────────────────────────────────────────────────────────────┘
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
           ┌───────────────┐   ┌───────────────┐
           │ GitHub Action │   │ GitLab CI     │
           │ PR Comments   │   │ MR Notes      │
           │ Mermaid       │   │ (Future)      │
           └───────────────┘   └───────────────┘
```

### 10. Radius Bicep Extension Compatibility

**Decision**: Support Radius Bicep extension type definitions

**Rationale**:
- Radius extends Bicep with custom types (`Applications.Core/containers`, etc.)
- These types must be recognized in ARM JSON output
- Type registry already exists in `pkg/corerp/api/`

**Implementation**:
- Resource type detection checks for `Applications.*` prefix
- Portable types (`Radius.Data/store`) shown as-is in static graph
- Environment-resolved graph (P3) requires live Radius environment connection

## Technology Decisions Summary

| Area | Decision | Rationale |
|------|----------|-----------|
| CLI Framework | Cobra (existing) | Consistency with `rad` CLI |
| Bicep Parsing | Bicep CLI → ARM JSON | Simplicity, official support |
| Git Operations | Shell exec | Universal, no CGo |
| Data Structures | Extend existing types | Backward compatible |
| GitHub Integration | Action, not App | Simpler setup, fork-friendly |
| Diff Algorithm | JSON semantic diff | Deterministic, meaningful |
| Output | JSON + optional Markdown | Machine + human readable |
| Platform Abstraction | Rendering layer separation | Future GitLab support |

## Open Questions Resolution

1. **Bicep Compiler Integration**: ✅ Use official Bicep CLI (per Constitution Principle VII)
2. **Graph Storage**: ✅ Committed to `.radius/app-graph.json` (per spec)
3. **GitHub App vs Action**: ✅ GitHub Action for MVP (simpler, fork-friendly)
4. **Diff Visualization**: Table + Mermaid in PR comments
5. **Parameter Handling**: ✅ Require params file; fail with clear error listing missing required parameters if Bicep has required parameters (no defaults) but no `--parameters` flag provided
