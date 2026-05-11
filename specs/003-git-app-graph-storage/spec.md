# Feature Specification: Git App Graph Preview

**Feature Branch**: `001-git-app-graph-preview`   
**Created**: January 30, 2026   
**Status**: Draft   
**Input**: User description: "Radius currently stores the state of application deployments as an app graph within its data store. Today, the app graph does not get generated until the application is deployed. Help me build an app graph representation for applications that are defined (e.g. in an app.bicep file) but not yet deployed. Additionally, enrich the app graph representation with git changelog info (i.e. git commit data) so that I may use this data to visualize how the app graph changes over time (i.e. across commits). The ultimate goal is to be able to visualize the app graph and do diffs of the app graph in GitHub on PRs, commit comparisons, etc."   

## Clarifications

### Session 2026-02-04

- Q: GitHub App vs Action for PR integration? → A: GitHub Action (fork-friendly, no installation approval required, aligns with existing Radius workflow patterns)
- Q: Diff visualization format in PR comments? → A: Table + Mermaid diagrams (change table for details, before/after diagrams for visual topology)
- Q: How to handle Bicep parameters without params file? → A: Require params file (fail with error if Bicep has required parameters but no `--parameters` provided)
- Q: GitHub Action trigger events? → A: `pull_request` + `push` (PR for review comments, push to main for baseline tracking)
- Q: Monorepo support with multiple app graphs? → A: Auto-detect all `**/.radius/app-graph.json` files; each diffed independently

## Problem Statement

Radius currently generates application graphs only after deployment, which means:
1. Developers cannot preview the app graph structure before deploying
2. There's no way to track how the application architecture evolves over time
3. PR reviewers cannot see the impact of Bicep changes on the overall application topology
4. No mechanism exists to compare app graphs across commits or branches

This feature introduces **static app graph generation** from Bicep files and **git-aware graph versioning** to enable visualization and diffing in GitHub workflows.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Generate App Graph from Bicep Files (Priority: P1)

As a **developer**, I want to generate an app graph from my `app.bicep` file without deploying, so I can preview the application topology and validate my changes locally.

As a **platform engineer**, I want to review app graph changes in PRs, so I can ensure architectural changes align with organizational standards before deployment.

**Why this priority**: This is the foundational capability. Without static graph generation, no other features can function. It delivers immediate value by enabling local validation.

**Independent Test**: Can be fully tested by running a CLI command against a Bicep file and verifying the graph output matches expected structure.

**Acceptance Scenarios**:

1. **Given** a valid `app.bicep` file with container, gateway, and database resources, **When** I run `rad app graph app.bicep`, **Then** I receive a JSON graph representation showing all resources and their connections.

2. **Given** a Bicep file with syntax errors, **When** I run `rad app graph app.bicep`, **Then** I receive a clear error message indicating the parsing failure with line/column information.

3. **Given** a Bicep file referencing external modules, **When** I run `rad app graph app.bicep`, **Then** the graph includes resources from all referenced modules with proper dependency tracking.

4. **Given** a Bicep file with parameterized values, **When** I run `rad app graph app.bicep --parameters params.json`, **Then** the graph reflects the resolved parameter values.

5. **Given** a Bicep file with required parameters (no defaults) and no `--parameters` flag, **When** I run `rad app graph app.bicep`, **Then** I receive a clear error listing the missing required parameters.

6. **Given** a Bicep file using the Radius Bicep extension types, **When** I run `rad app graph app.bicep`, **Then** the graph correctly identifies Radius-specific resource types and their relationships.

---

### User Story 2 - Export Graph as Diff-Friendly Format (Priority: P1)

As a developer, I want the app graph exported in a deterministic, diff-friendly format, so I can commit it to version control and see meaningful diffs when it changes.

**Why this priority**: Critical for enabling GitHub integration. Without a stable, diffable format, PR visualization is impossible.

**Independent Test**: Generate graph twice from identical Bicep, verify outputs are byte-identical. Modify Bicep, regenerate, verify diff shows only the changed elements.

**Output Model**: JSON is the canonical data format, always generated. Markdown is a rendered preview of the JSON data, generated additively when requested.

**Acceptance Scenarios**:

1. **Given** an app graph, **When** I export it, **Then** the JSON output is deterministically sorted (alphabetical by resource ID) producing identical output for identical inputs.

2. **Given** an app graph, **When** I run `rad app graph app.bicep`, **Then** JSON is written to `.radius/app-graph.json` (default location) serving as the single source of truth for all automation and diff operations.

3. **Given** an app graph, **When** I run `rad app graph app.bicep --stdout`, **Then** JSON is written to stdout instead of a file.

4. **Given** an app graph, **When** I run `rad app graph app.bicep --format markdown`, **Then** I receive **both** `.radius/app-graph.json` and `.radius/app-graph.md` containing:
   - A resource table with name, type, source file, and git metadata
   - An embedded Mermaid diagram showing the topology that GitHub renders automatically

5. **Given** a graph exported to markdown, **When** viewed in GitHub, **Then** the Mermaid diagram renders as a visual flowchart with distinct shapes for resource types (containers as rectangles, gateways as diamonds, databases as cylinders).

6. **Given** two app graphs from different commits, **When** I diff them, **Then** the diff is computed from JSON (not Markdown), and added/removed/modified resources are clearly identified.

---

### User Story 3 - Git Metadata Enrichment (Priority: P2)

As a developer, I want the app graph to automatically include git commit information, so I can track when and why each resource was added or modified.

**Why this priority**: Builds on P1 capabilities to enable historical tracking. Valuable but not blocking core functionality.

**Independent Test**: Generate graph from a Bicep file in a git repository, verify each resource includes commit SHA, author, and timestamp of last modification by default.

**Acceptance Scenarios**:

1. **Given** a Bicep file in a git repository, **When** I run `rad app graph app.bicep`, **Then** each resource automatically includes the commit SHA, author, date, and message of its last modification.

2. **Given** a resource defined across multiple Bicep files, **When** I generate the graph, **Then** the resource shows the most recent commit that affected any of its defining files.

3. **Given** a newly added resource not yet committed, **When** I generate the graph, **Then** the resource is marked as "uncommitted" with the current working directory state.

4. **Given** a graph with git metadata, **When** I export to markdown, **Then** each resource row includes a linked commit SHA (e.g., `[abc123](../../commit/abc123)`).

5. **Given** a Bicep file in a git repository, **When** I run `rad app graph app.bicep --no-git`, **Then** the graph is generated without git metadata for faster execution.

6. **Given** a Bicep file outside a git repository, **When** I run `rad app graph app.bicep`, **Then** the graph is generated successfully with git fields marked as "not available".

---

### User Story 4 - GitHub Action for PR Graph Diff (Priority: P2)

As a PR reviewer, I want to see a visual diff of the app graph in PR comments, so I can understand the architectural impact of code changes without deploying.

**Why this priority**: High-value GitHub integration, but depends on P1 capabilities being stable.

**Operational Model**: The GitHub Action reads committed `.radius/app-graph.json` files from git history — it does NOT generate graphs on-demand. This keeps the Action lightweight (no Bicep/Radius tooling required) and fast.

**Trigger Events**: The Action supports two trigger modes:
- **`pull_request`**: Posts diff comments on PRs when `.radius/app-graph.json` changes
- **`push` to main/default branch**: Updates baseline tracking for historical comparison

**Monorepo Support**: The Action auto-detects all `**/.radius/app-graph.json` files in the repository. Each graph is diffed independently, with separate PR comment sections per application.

**Independent Test**: Create a PR with Bicep changes and updated graph JSON, verify the action posts a comment showing before/after graph comparison.

**Acceptance Scenarios**:

1. **Given** a PR that includes changes to `.radius/app-graph.json`, **When** the GitHub Action runs, **Then** it reads the JSON from base and head commits and posts a comment showing the graph diff with added/removed/modified resources highlighted.

2. **Given** a PR with no changes to `.radius/app-graph.json`, **When** the GitHub Action runs, **Then** it posts a comment indicating "No app graph changes detected."

3. **Given** a PR that adds a new connection between resources, **When** the GitHub Action runs, **Then** the diff clearly shows the new connection with source and target resources.

4. **Given** a PR comment already exists from a previous run, **When** the PR is updated and the action runs again, **Then** the existing comment is updated rather than creating a duplicate.

5. **Given** a PR where Bicep files changed but `.radius/app-graph.json` was not updated, **When** the CI validation job runs, **Then** it fails with a message instructing the developer to run `rad app graph app.bicep` and commit the updated graph.

6. **Given** a monorepo with multiple Radius applications (e.g., `apps/frontend/.radius/app-graph.json` and `apps/backend/.radius/app-graph.json`), **When** the GitHub Action runs on a PR, **Then** it detects all graph files and posts a unified comment with separate diff sections per application.

---

### User Story 5 - Historical Graph Timeline (Priority: P3)

As a developer, I want to view how my app graph evolved across commits, so I can understand architectural decisions and identify when changes were introduced.

**Why this priority**: Advanced feature for historical analysis. Valuable for debugging and auditing but not essential for core workflow.

**Independent Test**: Generate timeline for last 10 commits, verify each entry shows the graph state and changes from previous commit.

**Acceptance Scenarios**:

1. **Given** a git repository with multiple commits affecting Bicep files, **When** I run `rad app graph history app.bicep --commits 10`, **Then** I receive a timeline showing graph snapshots at each commit with change summaries.

2. **Given** a specific commit SHA, **When** I run `rad app graph app.bicep --at abc123`, **Then** I receive the app graph as it existed at that commit.

3. **Given** two commit SHAs, **When** I run `rad app graph diff app.bicep --from abc123 --to def456`, **Then** I receive a detailed diff showing all graph changes between those commits.

---

### User Story 6 - Environment-Resolved Graph (Priority: P3)

As a platform engineer, I want to see how abstract Radius types resolve to concrete infrastructure in a specific environment, so I can understand the actual resources that will be deployed.

**Why this priority**: Advanced feature for environment-specific analysis. The static graph (showing portable types) serves most PR review needs; resolved views are valuable for deployment planning and troubleshooting.

**Background**: Radius portable types like `Radius.Data/store` resolve differently depending on the environment's recipe configuration:
- Environment → RecipePack → Recipe → Concrete Resource
- The same `Radius.Data/store` might become PostgreSQL in `dev` and CosmosDB in `prod`

**Independent Test**: Generate resolved graph for an environment with known recipe bindings, verify concrete resource types appear instead of abstract Radius types.

**Acceptance Scenarios**:

1. **Given** a Bicep file with `Radius.Data/store` and a connected Radius environment with PostgreSQL recipes, **When** I run `rad app graph app.bicep --environment prod`, **Then** the graph shows the resolved `Azure.DBforPostgreSQL/flexibleServers` (or equivalent) instead of the abstract `Radius.Data/store`.

2. **Given** a Bicep file with portable types, **When** I run `rad app graph app.bicep --environment dev` and `rad app graph app.bicep --environment prod`, **Then** I can compare how the same application resolves to different infrastructure across environments.

3. **Given** an environment where a recipe is not configured for a portable type, **When** I run `rad app graph app.bicep --environment prod`, **Then** the graph shows the abstract type with an annotation indicating "no recipe bound".

4. **Given** a Bicep file, **When** I run `rad app graph app.bicep` (no `--environment` flag), **Then** the graph shows the abstract portable types (default behavior unchanged).

---

### Edge Cases

- What happens when Bicep file references resources outside the current file/module that cannot be resolved?
  - Generate partial graph with unresolved references marked as "external" placeholders
- How does the system handle circular dependencies in Bicep?
  - Detect and report cycles with clear error messaging; still generate graph with cycle annotation
- What happens when git history is shallow (e.g., `--depth 1` clone)?
  - Gracefully degrade: use available history, mark resources as "history unavailable" when git blame fails
- How does the system handle large graphs (100+ resources)?
  - Paginate CLI output, provide `--filter` options, optimize JSON/Markdown output for size
- What happens when Bicep uses runtime expressions that can't be statically resolved?
  - Mark affected values as "dynamic" in the graph, use placeholder notation
- What happens when Bicep files use cloud-specific resources (Azure, AWS)?
  - Graph generation MUST work regardless of cloud provider; cloud-specific resources are represented with their provider prefix (e.g., `Microsoft.Storage/storageAccounts`, `AWS::S3::Bucket`)
- What happens when the committed graph is stale (Bicep changed but graph not regenerated)?
  - CI validation job compares committed graph to freshly generated graph; fails PR if they differ
  - Graph JSON includes `sourceHash` to detect staleness without full regeneration
  - Clear error message instructs developer to run `rad app graph app.bicep`
- What does the graph show for portable Radius types like `Radius.Data/store` that resolve differently per environment?
  - **Static graph shows abstract types**: The declared `Radius.Data/store` is shown, not the resolved infrastructure (PostgreSQL, CosmosDB, etc.)
  - This is intentional—the static graph represents the **portable application architecture** independent of environment-specific recipe resolution
  - For environment-resolved views, see User Story 6 (P3)

---

## CLI Design

This feature extends the existing `rad app graph` command with file-based input for static graph generation. The command intelligently distinguishes between deployed apps and Bicep files based on the argument:

| Command | Input Type | Output |
|---------|------------|--------|
| `rad app graph myapp` | App name | Deployed app graph (existing behavior) |
| `rad app graph myapp -e prod` | App name + environment | Deployed graph in specific environment |
| `rad app graph app.bicep` | Bicep file (`.bicep` extension) | JSON to `.radius/app-graph.json` (default) |
| `rad app graph app.bicep --stdout` | Bicep file + stdout flag | JSON to stdout (no file written) |
| `rad app graph app.bicep -o custom.json` | Bicep file + custom output | JSON to specified file |
| `rad app graph app.bicep --format markdown` | Bicep file + markdown | JSON + Markdown to `.radius/` |
| `rad app graph app.bicep --no-git` | Bicep file + no-git | JSON without git metadata (faster) |
| `rad app graph app.bicep --at abc123` | Bicep file + commit | JSON at specific commit |
| `rad app graph diff app.bicep --from abc123 --to def456` | Bicep file + commits | Diff computed from JSON, output as JSON or Markdown |
| `rad app graph history app.bicep --commits 10` | Bicep file + count | Historical timeline |
| `rad app graph app.bicep --environment prod` | Bicep file + environment | JSON with resolved recipe types |

**Output Model**:
- **JSON is canonical**: Always generated, serves as the single source of truth for all automation and diff operations
- **Markdown is additive**: When `--format markdown` is specified, Markdown is generated *in addition to* JSON as a human-readable preview
- **GitHub Action uses JSON**: Diff computation is always JSON-to-JSON; Markdown is purely a rendering/presentation layer for PR comments

**Design Rationale**: Unifying under `rad app graph` provides:
- Conceptual consistency: both are "app graphs" (prospective vs. deployed)
- Discoverability: all graph functionality in one place
- Intuitive disambiguation: `.bicep` extension clearly indicates file input
- Alignment with existing `rad app graph <appname>` pattern

---

## Committed Artifact Model

The app graph JSON is designed to be **committed to version control** as a tracked artifact. This enables lightweight GitHub integration without requiring the Action to have Bicep/Radius tooling.

### Default Output Location

By default, `rad app graph app.bicep` writes to `.radius/app-graph.json` relative to the Bicep file's directory:

```
myapp/
├── app.bicep
├── modules/
│   └── database.bicep
└── .radius/
    ├── app-graph.json      # Canonical graph data (committed)
    └── app-graph.md        # Optional preview (if --format markdown)
```

### Developer Workflow

```bash
# 1. Make changes to Bicep files
vim app.bicep

# 2. Regenerate the graph (writes to .radius/app-graph.json by default)
rad app graph app.bicep

# 3. Commit both the Bicep changes and updated graph
git add app.bicep .radius/app-graph.json
git commit -m "Add redis cache to application"

# 4. Push and create PR — GitHub Action reads committed JSON to render diff
git push
```

### Why Committed Artifacts?

| Benefit | Explanation |
|---------|-------------|
| **Simple GitHub Action** | Action is a lightweight viewer that reads JSON from git history — no Bicep CLI, no Radius environment needed |
| **Fast CI** | No graph generation in CI; diff is just JSON comparison |
| **Reproducible** | Graph captured at commit time, not regenerated with potentially different tooling |
| **Auditable** | Graph evolution visible in git history alongside code changes |
| **Fork-friendly** | Works in forks without special tooling or secrets |

### Staleness Detection

To prevent committed graphs from drifting out of sync with Bicep files:

1. **CI Validation Job** (recommended): Regenerate graph in CI, compare to committed version, fail if different
2. **Pre-commit Hook** (optional): Validate graph matches Bicep before allowing commit
3. **Graph Metadata**: JSON includes `sourceHash` field — hash of input Bicep file(s) for staleness detection

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

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST parse Bicep files and extract resource definitions without requiring deployment
- **FR-002**: System MUST resolve module references and build a complete graph across multiple Bicep files
- **FR-003**: System MUST extract connection relationships from resource properties (connections, routes, ports)
- **FR-004**: System MUST produce deterministic output (same input = byte-identical output)
- **FR-005**: System MUST always generate JSON output as the canonical data format with stable key ordering for deterministic diffs
- **FR-006**: System MUST support Markdown output as an **additive** preview format (generated alongside JSON when `--format markdown` is specified), containing a resource table and embedded Mermaid diagram
- **FR-007**: System MUST perform all diff computations using JSON data, with Markdown used only as a rendering layer for human consumption
- **FR-008**: System MUST enrich graph nodes with git metadata (commit SHA, author, date, message) by default when in a git repository, with `--no-git` flag to disable
- **FR-009**: System MUST track which Bicep file(s) define each resource for git blame integration
- **FR-010**: System MUST write graph output to `.radius/app-graph.json` by default (relative to Bicep file location), with `--stdout` flag for stdout output and `-o` flag for custom path
- **FR-011**: System MUST include `sourceHash` metadata in JSON output to enable staleness detection
- **FR-012**: System MUST provide a GitHub Action that reads committed graph JSON from git history and posts graph diffs on PRs (no graph generation in CI)
- **FR-013**: System MUST update existing PR comments rather than creating duplicates
- **FR-014**: System MUST support generating graphs at specific git commits/refs
- **FR-015**: System MUST handle Bicep parameter files to resolve parameterized values
- **FR-016**: System MUST report clear errors for invalid Bicep syntax with file/line/column information
- **FR-017**: System MUST handle unresolvable references gracefully with placeholder annotations
- **FR-018**: System MUST work with Bicep files targeting any cloud provider (multi-cloud neutrality per Constitution Principle III)
- **FR-019**: System MUST be compatible with the Radius Bicep extension type definitions

### Non-Functional Requirements

- **NFR-001**: All Go code MUST follow Effective Go patterns and pass `golangci-lint` (Constitution Principle II)
- **NFR-002**: All exported Go packages, types, and functions MUST have godoc comments (Constitution Code Quality Standards)
- **NFR-003**: Feature MUST NOT require changes to existing deployment workflows (Constitution Principle IX - Incremental Adoption)
- **NFR-004**: CLI commands MUST follow existing `rad` CLI patterns and conventions
- **NFR-005**: Error messages MUST be actionable with clear guidance for resolution (Constitution Principle VI)

### Key Entities

- **AppGraph**: Root container holding all resources, connections, and metadata for a single application
  - Resources: Collection of AppGraphResource nodes
  - Metadata: Git commit info, generation timestamp, source files
  
- **AppGraphResource**: Single resource node in the graph
  - ID: Unique resource identifier (matches Radius resource ID format)
  - Name: Human-readable resource name
  - Type: Resource type (e.g., `Applications.Core/containers`)
  - SourceFile: Bicep file path where resource is defined
  - SourceLine: Line number in source file
  - Connections: Outbound connections to other resources
  - GitInfo: Last commit SHA, author, date, message affecting this resource
  
- **AppGraphConnection**: Edge between two resources
  - SourceID: Origin resource
  - TargetID: Destination resource
  - Direction: Outbound/Inbound
  - Type: Connection type (connection, route, port binding)
  
- **GraphDiff**: Comparison result between two graphs
  - AddedResources: Resources present in new but not old
  - RemovedResources: Resources present in old but not new
  - ModifiedResources: Resources with changed properties/connections
  - AddedConnections: New edges
  - RemovedConnections: Removed edges

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Graph generation completes in < 5 seconds for applications with up to 50 resources
- **SC-002**: Generated graphs are 100% deterministic (identical input produces byte-identical output)
- **SC-003**: Graph diff correctly identifies all added, removed, and modified resources with zero false positives
- **SC-004**: GitHub Action posts PR comments within 60 seconds of workflow trigger
- **SC-005**: Markdown output (including embedded Mermaid diagram) renders correctly in GitHub without manual formatting
- **SC-006**: Git enrichment adds < 2 seconds overhead for repositories with up to 1000 commits
- **SC-007**: System handles Bicep files up to 5000 lines without performance degradation
- **SC-008**: Error messages include actionable guidance in 100% of failure cases

---

## Testing Requirements *(per Constitution Principle IV)*

This feature MUST include comprehensive testing across the testing pyramid:

### Unit Tests
- Test individual graph parsing functions in isolation
- Test git metadata extraction logic
- Test output formatters (JSON, Markdown with embedded Mermaid) with known inputs
- Test error handling for malformed Bicep files
- All unit tests runnable with `make test` without external dependencies

### Integration Tests
- Test Bicep CLI integration for file parsing
- Test git operations (blame, log) with real git repositories
- Test module resolution across multiple Bicep files

### Functional Tests
- End-to-end test: Bicep file → graph generation → output validation
- Test GitHub Action in a real PR workflow
- Test graph diff accuracy with known before/after states

---

## Open Questions

1. **Bicep Compiler Integration**: Should we use the official Bicep CLI for parsing, or implement a lightweight parser? Trade-off: accuracy vs. dependency management. **Recommendation**: Use official Bicep CLI per Constitution Principle VII (Simplicity Over Cleverness).

2. ~~**Graph Storage**: Should generated graphs be committed to the repo (e.g., `app-graph.json`)? Trade-off: visibility vs. repo noise.~~ **RESOLVED (Initial Implementation)**: Graphs are committed to `.radius/app-graph.json`. This enables lightweight GitHub Action (reads from git history, no tooling required) and provides auditable graph evolution. **Future Evolution**: External storage backends (e.g., SQLite, cloud databases) could be supported for scenarios requiring graph queries across repositories, historical analytics, or enterprise-scale graph management.

3. ~~**GitHub App vs Action**: Should the PR integration be a GitHub Action (user-managed) or a GitHub App (centrally managed)? Trade-off: flexibility vs. ease of setup.~~ **RESOLVED**: GitHub Action. Fork-friendly, no installation approval required, aligns with existing Radius workflow patterns, supports incremental adoption.

4. ~~**Diff Visualization**: What's the preferred format for showing diffs in PR comments—table-based, Mermaid side-by-side, or unified text diff?~~ **RESOLVED**: Table + Mermaid diagrams. Change table shows added/removed/modified resources with details; before/after Mermaid diagrams provide visual topology comparison. Both render natively in GitHub.

5. ~~**Parameter Handling**: How should we handle Bicep parameters without a params file—use defaults, require params, or mark as "unknown"?~~ **RESOLVED**: Require params file. If Bicep has required parameters (no defaults) but no `--parameters` flag is provided, fail with a clear error message listing the missing parameters.

---

## Cross-Repository Impact *(per Constitution Principle XVII)*

This feature may affect multiple Radius repositories:

| Repository | Impact |
|------------|--------|
| `radius` | CLI implementation (`pkg/cli/cmd/app/graph/`), core graph logic |
| `docs` | User documentation for new CLI commands, GitHub Action setup guide |
| `design-notes` | This specification and implementation plan |

Coordinate changes across repositories per Constitution guidance on polyglot project coherence.