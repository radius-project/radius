---
name: arch-documenter
description: 'Document application architectures with Mermaid diagrams. Use for: generating architecture overviews, component diagrams, sequence diagrams from code, explaining complex Go codebases, answering architecture questions, suggesting architectural improvements, producing entity-relationship diagrams, and distilling code into human-readable descriptions.'
argument-hint: 'Describe what part of the architecture to document or ask an architecture question'
---

# Architecture Documenter

Expert skill for analyzing codebases, documenting application architectures, and generating accurate Mermaid diagrams grounded in actual source code.

## When to Use

- Generate a high-level architecture overview of the system or a subsystem
- Produce component diagrams showing entity relationships
- Create sequence diagrams that are true-to-code (reflect actual call chains)
- Explain how a complex subsystem works in plain language
- Answer questions about the existing architecture
- Suggest architectural improvements that would simplify the code
- Onboard new contributors by explaining system structure

## Core Principles

1. **Code-grounded**: Every diagram and explanation must be derived from actual source code, not assumptions. Read the code before documenting it.
2. **Progressive depth**: Start with high-level overviews, then drill into details only when asked.
3. **Accuracy over aesthetics**: A correct simple diagram beats an elaborate wrong one.
4. **Human-readable output**: Distill complex code concepts into clear, jargon-minimal prose. Use diagrams to complement text, not replace it.

## Procedure

### Step 1: Scope the Request

Determine what the user wants documented:

| Request Type | Output |
|---|---|
| "How does X work?" | Prose explanation + optional diagram |
| "Show me the architecture of X" | Component diagram + brief description |
| "Show me the flow when X happens" | Sequence diagram + step-by-step narrative |
| "What are the relationships between X, Y, Z?" | Entity-relationship / component diagram |
| "How could X be improved?" | Current-state diagram + improvement suggestions |
| "Give me an overview" | High-level system diagram + component summary |

### Step 2: Gather Context from Code

This is the most critical step. **Do not generate diagrams from memory or assumptions.**

1. **Identify entry points**: Find `main()` functions, server setup, route registration, or handler initialization relevant to the scope.
2. **Trace the call chain**: Follow function calls from entry points through layers (frontend → backend → data). Read interfaces and their implementations.
3. **Map package structure**: Understand how packages relate to each other. Pay attention to `doc.go` files for package-level documentation.
4. **Identify key types**: Find the core structs, interfaces, and their methods that define the architecture.
5. **Note patterns**: Identify design patterns in use (controller pattern, resource provider pattern, middleware chains, async operations, etc.).

#### Go-Specific Investigation Techniques

- **Find interface implementations**: Search for methods matching interface signatures. Use `grep` for receiver types.
- **Trace dependency injection**: Look at constructor functions (`New...()`) and `setup` packages to understand how components are wired together.
- **Follow the handler chain**: For HTTP services, start at route registration and follow middleware → handler → controller → backend flow.
- **Check for code generation**: Look for generated files (`zz_generated_*.go`, files with generation comments) to understand what is hand-written vs. generated.
- **Read test files**: Tests often reveal the expected behavior and interaction patterns between components.

### Step 3: Generate the Diagram

Choose the appropriate Mermaid diagram type based on the request. See [Mermaid Diagram Reference](./references/mermaid-patterns.md) for templates.

| Situation | Diagram Type |
|---|---|
| System / subsystem overview | `graph TD` (top-down flowchart) |
| Request/response flow | `sequenceDiagram` |
| Entity relationships | `classDiagram` or `erDiagram` |
| State transitions | `stateDiagram-v2` |
| Component dependencies | `graph LR` (left-right flowchart) |
| Deployment topology | `graph TD` with subgraphs |

#### Diagram Quality Checklist

- [ ] Every node in the diagram corresponds to a real package, type, or component in the code
- [ ] Relationships reflect actual code dependencies (imports, function calls, interface implementations)
- [ ] Labels use the actual names from the codebase (type names, package names, function names)
- [ ] The diagram is not overcrowded — split into multiple diagrams if >15 nodes
- [ ] Subgraphs are used to group related components
- [ ] Arrow labels describe the nature of the relationship (e.g., "implements", "calls", "sends")

### Step 4: Write the Explanation

Pair every diagram with a prose explanation that:

1. **Summarizes** what the diagram shows in 1-2 sentences
2. **Walks through** the key components and their responsibilities
3. **Highlights** important architectural decisions or patterns
4. **Notes** any non-obvious aspects (error handling paths, async behavior, retries)

#### Writing Style

- Use short paragraphs (3-4 sentences max)
- Lead with the "what" and "why" before the "how"
- Use bullet lists for component responsibilities
- Bold key terms on first use
- Reference specific file paths so readers can find the code

### Step 5: Suggest Improvements (When Asked)

When the user asks for architectural improvements:

1. **Identify pain points**: Look for code smells — excessive coupling, god packages, circular dependencies, duplicated patterns, inconsistent abstractions.
2. **Propose specific changes**: Name the packages/types involved and describe the refactoring.
3. **Show before/after**: Use a current-state diagram and a proposed-state diagram to illustrate the improvement.
4. **Assess trade-offs**: Every change has a cost. Note migration effort, risk, and what gets simpler vs. more complex.

## Radius Project Context

This skill is tailored for the Radius project. Key architectural knowledge:

### High-Level Components

| Component | Location | Purpose |
|---|---|---|
| UCP (Universal Control Plane) | `pkg/ucp/`, `cmd/ucpd/` | Core control plane, resource routing, proxy |
| Applications RP (Applications.Core) | `pkg/corerp/`, `cmd/applications-rp/` | Resource provider for core Radius resources (environments, applications, containers, gateways) |
| Dynamic RP | `pkg/dynamicrp/`, `cmd/dynamic-rp/` | Resource provider for user-defined resource types that have no dedicated RP implementation |
| Dapr RP | `pkg/daprrp/` | Resource provider for Dapr portable resources (state stores, pub/sub, secret stores) |
| Datastores RP | `pkg/datastoresrp/` | Resource provider for datastore portable resources (MongoDB, Redis, SQL) |
| Messaging RP | `pkg/messagingrp/` | Resource provider for messaging portable resources (RabbitMQ) |
| Portable Resources (shared) | `pkg/portableresources/` | Shared backend, handlers, processors, and renderers used by Dapr/Datastores/Messaging RPs |
| Controller | `pkg/controller/`, `cmd/controller/` | Kubernetes controller for deployment reconciliation |
| CLI (rad) | `pkg/cli/`, `cmd/rad/` | Command-line interface |
| ARM RPC Framework | `pkg/armrpc/` | Shared framework for building ARM-compatible resource providers |
| Recipes Engine | `pkg/recipes/` | Recipe execution engine for provisioning infrastructure via Terraform and Bicep |
| SDK | `pkg/sdk/` | Client SDK for connecting to and interacting with the Radius control plane |
| Shared Components | `pkg/components/` | Shared infrastructure: database, message queue, secrets, metrics, tracing |
| RP Commons | `pkg/rp/` | Shared packages used by corerp and the portable resource providers |

### Common Patterns

- **Frontend/Backend split**: Resource providers have `frontend/` (HTTP handlers, API validation) and `backend/` (async operations, deployment) packages.
- **Data models**: Each RP defines data models in `datamodel/` with versioned API types in `api/`.
- **ARM RPC controllers**: HTTP handlers implement the `armrpc` controller interfaces.
- **Async operations**: Long-running operations use the async operation framework in `pkg/armrpc/asyncoperation/`.
- **Recipes**: Infrastructure provisioning via Terraform/Bicep recipes in `pkg/recipes/`.

### Design Notes

Architecture design documents are available in the `design-notes/architecture/` directory of the `radius-project/design-notes` repository. Reference these for historical context on architectural decisions.

## Output Format

Always structure output as:

````markdown
## [Title — what is being documented]

[1-2 sentence summary]

```mermaid
[diagram]
```

### Key Components

[Bulleted list of components and responsibilities]

### How It Works

[Prose walkthrough of the flow/architecture]

### Notable Details

[Any non-obvious aspects worth calling out]
````
