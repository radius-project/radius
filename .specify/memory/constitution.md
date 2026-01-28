<!--
Sync Impact Report
- Version change: N/A -> 1.0.0
- Modified principles: none (new document)
- Added sections:
  - Principle I (API-First Design)
  - Principle II (Idiomatic Code Standards)
  - Principle III (Multi-Cloud Neutrality)
  - Principle IV (Testing Pyramid Discipline)
  - Principle V (Collaboration-Centric Design)
  - Principle VI (Open Source and Community-First)
  - Principle VII (Simplicity Over Cleverness)
  - Principle VIII (Separation of Concerns and Modularity)
  - Principle IX (Incremental Adoption & Backward Compatibility)
  - Principle X (TypeScript and React Standards)
  - Principle XI (Frontend Testing Discipline)
  - Principle XII (Resource Type Schema Quality)
  - Principle XIII (Recipe Development Standards)
  - Principle XIV (Documentation Structure and Quality)
  - Principle XV (Documentation Contribution Standards)
  - Principle XVI (Repository-Specific Standards)
  - Principle XVII (Polyglot Project Coherence)
- Removed sections: none
- Templates requiring updates:
  ✅ .specify/templates/plan-template.md (Constitution Check remains generic, no change required)
  ✅ .specify/templates/spec-template.md (Incremental adoption supported via independent user stories pattern)
  ✅ .specify/templates/tasks-template.md (Incremental delivery language already present; aligns with new principles)
- Follow-up TODOs:
  TODO(REPO_ADDENDUMS): Create constitution addendums for dashboard, resource-types-contrib, and docs repositories
  TODO(CROSS_REPO_REVIEW): Establish cross-repository design review process for coordinated changes
-->

# Radius Design Notes Constitution

## Core Principles

### I. API-First Design

All features MUST be designed with well-defined APIs before implementation begins. API definitions MUST use TypeSpec for generating OpenAPI specifications, following established patterns in the `typespec/` directory. APIs MUST be versioned according to semantic versioning with backward compatibility maintained within major versions. Resource Provider APIs MUST follow ARM-RPC patterns for consistency across the Radius control plane.

**Rationale**: Radius is fundamentally an API-driven platform enabling multi-cloud deployments and integration with various tools (Bicep, Terraform, Kubernetes). Clear API contracts ensure developers and platform engineers can collaborate effectively with well-understood interfaces.

### II. Idiomatic Code Standards

All code MUST follow language-specific conventions and best practices appropriate to its ecosystem. Each language has community standards that improve readability, maintainability, and developer productivity.

**Directives**:

- **Go**: Follow *Effective Go* patterns; format with `gofmt`; provide godoc comments for all exported items; minimize exported surface area; leverage Go's simplicity over complex abstractions; handle errors explicitly without suppression
- **TypeScript**: Follow TypeScript handbook and Backstage guidelines; use ESLint and Prettier; enable strict mode; provide explicit types for public APIs; prefer functional patterns where appropriate
- **Bicep**: Follow official best practices; use kebab-case for resources, camelCase for parameters; modularize with modules; add parameter descriptions; prefer secure defaults. Note: Radius uses its own Bicep extension that provides type definitions for Radius resource types; ensure compatibility with this extension
- **Terraform**: Follow HashiCorp style guide; format with `terraform fmt`; use modules for reusability; provide variable descriptions and validation; specify explicit dependencies
- **Python**: Follow PEP 8; use type hints for function signatures; prefer comprehensions where readable; use virtual environments
- **Markdown**: Follow CommonMark; maintain consistent heading hierarchy; use reference-style links in long documents

**Rationale**: Idiomatic code reduces cognitive load and makes each repository approachable to contributors familiar with that language's ecosystem. Consistency within each language community improves collaboration across the multi-language Radius project.

### III. Multi-Cloud Neutrality

All designs MUST account for multi-cloud deployment scenarios (Kubernetes, Azure, AWS, on-premises). Cloud-specific implementations MUST be abstracted behind provider interfaces defined in `pkg/aws/`, `pkg/azure/`, and `pkg/kubernetes/`. Portable resources MUST work across all supported platforms through the Recipe system. Features MUST NOT assume availability of cloud-specific services unless explicitly designed as cloud-specific extensions.

**Rationale**: Radius enables organizations to avoid cloud lock-in and deploy applications consistently across environments. This principle ensures the platform remains true to its core value proposition of cloud neutrality.

### IV. Testing Pyramid Discipline (NON-NEGOTIABLE)

Every feature MUST include comprehensive testing across appropriate layers for its repository type:

**For Go code (radius repo)**:

- **Unit tests**: Test individual functions and types in `pkg/` directories, runnable with basic prerequisites only (no external dependencies). Use `make test` to run all unit tests. Note: Running the full test suite may take significant time without test caching tools; consider using targeted test runs during development.
- **Integration tests**: Test features with dependencies (databases, external services, cloud providers) in appropriate `test/` subdirectories.
- **Functional tests**: End-to-end scenarios using the `magpiego` test framework in `test/functional/`, exercising realistic user workflows.

**For TypeScript/React code (dashboard repo)**:

- **Unit tests**: Test individual components, hooks, and utilities with Jest and React Testing Library; ensure proper mocking of external dependencies.
- **Integration tests**: Test plugin interactions, API integrations, and complex component interactions.
- **E2E tests**: Use Playwright to test full user workflows across the dashboard UI.

**For IaC code (resource-types-contrib repo)**:

- **Schema validation**: Test YAML schemas validate correctly against expected inputs.
- **Recipe deployment tests**: Test Bicep/Terraform recipes deploy successfully in test environments.
- **Integration tests**: Verify recipes integrate correctly with Radius control plane.

**For documentation (docs repo)**:

- **Build verification**: Hugo builds complete without errors.
- **Link validation**: All internal and external links resolve correctly.
- **Spelling validation**: Use `pyspelling` to catch typos and maintain consistency.
- **Example validation**: All code examples build and run successfully.

Tests MUST be written during feature implementation, not as an afterthought. Code coverage reports are reviewed in PRs to ensure adequate test coverage. New features MUST NOT be merged without corresponding tests at appropriate pyramid levels. Tests MUST fail before implementation (Red-Green-Refactor cycle).

**Rationale**: Quality and reliability are paramount for a platform managing critical infrastructure across multiple clouds. Repository-specific testing ensures bugs are caught early while respecting the unique validation needs of Go services, UI components, infrastructure recipes, and documentation.

### V. Collaboration-Centric Design

Design specifications MUST explicitly address how the feature enables collaboration between developers and platform engineers. Features MUST consider both perspectives:

- **Developer experience**: How does this simplify application authoring and deployment? Does it reduce cognitive load?
- **Platform engineer experience**: How does this enable governance, compliance, and operational best practices?

Environment and Recipe abstractions MUST be designed to allow platform engineers to define infrastructure patterns while giving developers the flexibility they need. APIs and tooling MUST support both audiences without forcing one perspective onto the other.

**Rationale**: Radius exists to bridge the gap between development and operations teams. Every feature should reinforce this collaboration rather than creating new silos or imposing unnecessary constraints.

### VI. Open Source and Community-First

Design specifications MUST be authored in markdown and stored in the public `design-notes` repository before implementation begins. Significant features MUST follow the issue-first workflow at github.com/radius-project/radius, with community discussion before work begins. Design decisions MUST be documented with clear rationale. Breaking changes MUST be called out explicitly with migration guidance. All commits MUST include a `Signed-off-by` line (Developer Certificate of Origin).

**Rationale**: As a CNCF sandbox project, transparency and community involvement are essential. Documented design decisions ensure better outcomes, build trust with the community, and align with open source governance practices.

### VII. Simplicity Over Cleverness

Start simple and add complexity only when proven necessary through actual requirements. Question every abstraction layer—each one adds cognitive overhead. Optimize for correctness first, testability second, and simplicity third. Reject over-engineering and "future-proofing" in favor of solving immediate, well-understood requirements. Apply YAGNI (You Aren't Gonna Need It) principles rigorously.

**Rationale**: Premature complexity is the enemy of maintainability. Simple, direct solutions are easier to understand, test, debug, and evolve. Complexity should be justified by concrete needs, not hypothetical future scenarios.

### VIII. Separation of Concerns and Modularity

Components MUST have single, well-defined responsibilities with clear boundaries. Modules MUST be designed for reuse where appropriate without introducing unnecessary coupling. Dependencies between components MUST flow in one direction (avoid circular dependencies). Domain logic MUST be separated from infrastructure concerns (e.g., HTTP handlers, database access). Cross-cutting concerns (logging, authentication, telemetry) MUST be implemented through consistent patterns rather than scattered throughout the codebase.

**Rationale**: Clean separation enables independent testing, easier refactoring, and parallel development. Modular design allows components to evolve independently and promotes code reuse across the project.

### IX. Incremental Adoption & Backward Compatibility

Features, abstractions, and workflow changes MUST support gradual opt-in rather than forcing a disruptive migration. New abstractions MUST start behind feature flags or clearly labeled "experimental" status until validated by real usage. Documentation MUST call out required user actions for any change that affects existing workflows.

**Breaking Changes Policy**: Radius has not yet reached version 1.0.0, so breaking changes are acceptable when necessary to improve the platform. We strive to minimize breaking changes and provide migration guidance when they occur, but we do not guarantee backward compatibility until the 1.0.0 release. After 1.0.0, backward compatibility will be maintained within major versions with proper deprecation periods.

**Rationale**: Radius integrates with diverse existing toolchains (Bicep, Terraform, Kubernetes). While we aim to minimize disruption, the pre-1.0 phase allows us to make necessary improvements based on community feedback. Iterative evolution with clear communication reduces risk and builds trust with early adopters.

### X. TypeScript and React Standards (Dashboard)

All TypeScript code in the dashboard repository MUST follow Backstage's architectural patterns and community best practices. Components MUST be developed in isolation using Storybook with stories demonstrating all interactive states. React components MUST follow functional component patterns with hooks. Type safety MUST be enforced through TypeScript strict mode. API clients MUST use typed interfaces generated from OpenAPI specifications. State management MUST follow Backstage plugin conventions using context and hooks appropriately.

**Rationale**: The dashboard provides the primary UI for Radius users. Consistent TypeScript and React practices ensure the UI remains maintainable, accessible, and performant as the feature set grows.

### XI. Frontend Testing Discipline (Dashboard)

All UI components MUST include unit tests using Jest and React Testing Library, testing component behavior rather than implementation details. Interactive components MUST have Storybook stories demonstrating all states (loading, error, success, empty). Critical user workflows MUST have Playwright E2E tests covering realistic scenarios. Visual regression MUST be monitored through Storybook's visual testing tools. Accessibility MUST be validated using automated tools (axe-core) and manual keyboard navigation testing.

**Rationale**: Frontend bugs directly impact user experience and are often harder to detect through manual testing. Comprehensive frontend testing ensures the dashboard remains reliable and accessible across browsers and user contexts.

### XII. Resource Type Schema Quality (Contrib)

All resource type schemas MUST be valid YAML files with complete property definitions, descriptions, and examples. Schemas MUST include comprehensive inline documentation explaining each field's purpose and valid values. Required vs. optional fields MUST be clearly specified. Schemas MUST follow consistent naming conventions (camelCase for properties, kebab-case for resource names). Examples MUST be runnable and demonstrate realistic usage patterns. Maturity level (Alpha, Beta, Stable) MUST be clearly documented with stability guarantees.

**Rationale**: Resource type schemas define the contract for community-contributed resources. High-quality schemas ensure developers can author Radius applications confidently without ambiguity or trial-and-error.

### XIII. Recipe Development Standards (Contrib)

All Recipes MUST be implemented in either Terraform or Bicep with clear module structure. Terraform is the preferred language for Recipe authoring due to its broader ecosystem and community familiarity. Recipes MUST include comprehensive README documentation explaining purpose, prerequisites, parameters, and outputs. Parameters MUST have descriptions and sensible defaults where applicable. Recipes MUST follow secure-by-default principles (e.g., disable public access, enable encryption). Recipes MUST be tested in representative environments before contribution. Recipes are inherently platform-specific (targeting Azure, AWS, or other infrastructure providers); a set of Recipes with the same resource type can provide cloud-agnostic behavior to applications by offering equivalent functionality across platforms.

**Rationale**: Recipes enable platform engineers to define reusable infrastructure patterns. Well-structured recipes reduce duplication, improve security posture, and accelerate Radius adoption by providing production-ready infrastructure building blocks.

### XIV. Documentation Structure and Quality (Docs)

All documentation MUST follow the [Diátaxis](https://diataxis.fr/) framework organizing content into Tutorials, How-To Guides, Reference, and Explanation. Documentation MUST be written in Markdown following the Docsy theme conventions. Code examples MUST be tested and runnable. Screenshots MUST be up-to-date with current UI state. Internal links MUST use Hugo shortcodes for maintainability. Navigation structure MUST support progressive disclosure from beginner to advanced topics. See the Docs repo [contributing guide](https://github.com/radius-project/docs/blob/8c5c60a743dcd4392a6795359e701362ad3da9b0/docs/content/contributing/docs/contributing-docs/index.md) for details.

**Rationale**: Radius serves diverse audiences from platform engineers to application developers. Structured documentation helps users find answers quickly whether they're learning, solving problems, or seeking reference material.

### XV. Documentation Contribution Standards (Docs)

All documentation contributions MUST build successfully with Hugo (`hugo serve`). Markdown MUST pass markdownlint validation. Spelling MUST be validated with pyspelling. Links MUST be validated before merging. CLI documentation MUST be auto-generated from Cobra command definitions in the radius repo. API documentation MUST be auto-generated from OpenAPI specs. Manually-written docs MUST be kept in sync with code through CI validation.

**Rationale**: Documentation quality directly impacts user success and satisfaction. Automated validation catches errors early while auto-generation from code ensures documentation stays accurate as the platform evolves.

### XVI. Repository-Specific Standards

Each repository MAY define additional standards and conventions in a `CONTRIBUTING.md` file that complement (but do not contradict) this constitution. Repository-specific standards SHOULD address:

- **Build and test workflows**: How to run local builds, tests, and validation
- **Code organization**: Directory structure conventions specific to that repo
- **Review process**: Repository-specific review checklists and approval requirements
- **Release process**: How changes are released and versioned for that repo
- **Tool configuration**: Linters, formatters, and other tooling specific to that stack

**Rationale**: Different repositories serve different purposes with different technology stacks. Repository-specific standards provide flexibility while maintaining coherence through shared core principles.

### XVII. Polyglot Project Coherence

Cross-cutting concerns (authentication, API patterns, error handling, observability) MUST have consistent design patterns across repositories despite different implementation languages. Shared concepts MUST use consistent terminology in documentation and code. Repository boundaries MUST be respected—avoid tight coupling between repos; prefer API contracts over shared code. Design decisions affecting multiple repositories MUST be documented in the design-notes repo with cross-repo impact clearly stated.

**Rationale**: Radius is fundamentally a polyglot project spanning Go services, TypeScript UI, IaC templates, and documentation. Coherent patterns across repositories reduce cognitive load when contributors work across boundaries and ensure the platform feels unified to users despite its technical diversity.

## Technology Stack & Standards

### Supported Languages and Tools

- **Go**: Primary implementation language for control plane services (`pkg/corerp/`, `pkg/ucp/`, etc.), CLI (`pkg/cli/`), and controllers (`pkg/controllers/`) in the radius repo. Version specified in `go.mod`.
- **TypeScript/Node.js**: Dashboard UI (Backstage-based), TypeSpec API definitions in `typespec/` directory, Bicep tooling in `bicep-tools/`, build scripts.
- **React**: Frontend framework for dashboard UI components, using functional components with hooks.
- **Python**: Code generation scripts in `hack/` and tooling automation.
- **Bicep**: Infrastructure as Code language for Radius resource definitions and application authoring.
- **Terraform**: Primary IaC language for Recipe authoring in resource-types-contrib repo due to broader ecosystem familiarity.
- **Bicep (Recipes)**: Alternative IaC language for Recipes, particularly useful for Azure-native scenarios.
- **TypeSpec**: API definition language for generating OpenAPI specifications in `swagger/`.
- **Hugo**: Static site generator for documentation using Docsy theme.
- **Markdown**: Documentation format across all repositories.

### Development Environment Requirements

All contributors MUST be able to develop using appropriate tooling for their repository:

**For radius repo**:

- **VS Code with Dev Containers** (recommended): Pre-configured environment with all tools in `.devcontainer/devcontainer.json`
- **Local installation**: Following prerequisites documented in `docs/contributing/contributing-code/contributing-code-prerequisites/`

The radius dev container includes: Git, GitHub CLI, Go, Node.js, Python, gotestsum, kubectl, Helm, Docker, jq, k3d, kind, stern, Dapr CLI, and VS Code extensions (Go, Python, Bicep, Kubernetes, TypeSpec, YAML, shellcheck, Makefile Tools).

For local Kubernetes testing in radius repo, prefer **k3d** as the primary tool. Secondarily consider **kind** for compatibility testing.

**For dashboard repo**:

- Node.js and yarn package manager
- VS Code with recommended extensions: ESLint, Prettier, TypeScript
- Local Radius installation for testing dashboard against real APIs

**For resource-types-contrib repo**:

- Bicep CLI for Bicep recipe development
- Terraform CLI for Terraform recipe development
- Text editor with YAML support for schema editing
- Local Radius installation for testing recipes

**For docs repo**:

- Hugo extended version for local documentation builds
- Python with pyspelling for spell checking
- Text editor with Markdown support

### Code Quality Standards

**For Go code (radius repo)**:

- **Formatting**: All Go code MUST be formatted with `gofmt` (enforced by `make format-check`)
- **Linting**: All code MUST pass `golangci-lint` checks (run via `make lint`)
- **Documentation**: All exported Go packages, types, variables, constants, and functions MUST have godoc comments

**For TypeScript code (dashboard repo)**:

- **Formatting**: All TypeScript code MUST be formatted with Prettier
- **Linting**: All code MUST pass ESLint checks with Backstage configuration
- **Type checking**: All code MUST pass TypeScript strict mode checks
- **Documentation**: All exported components, hooks, and utilities MUST have TSDoc comments

**For Bicep code (resource-types-contrib repo)**:

- **Formatting**: All Bicep code SHOULD be formatted with Bicep CLI formatter
- **Linting**: All Bicep code MUST pass Bicep linter validation
- **Documentation**: All modules MUST have parameter descriptions and examples

**For Terraform code (resource-types-contrib repo)**:

- **Formatting**: All Terraform code MUST be formatted with `terraform fmt`
- **Linting**: All Terraform code SHOULD pass `terraform validate` checks
- **Documentation**: All modules MUST have variable descriptions and examples

**For Markdown (all repos)**:

- **Formatting**: All Markdown SHOULD follow consistent style conventions
- **Linting**: All Markdown MUST pass markdownlint validation (in docs repo)
- **Spelling**: All documentation MUST pass pyspelling validation (in docs repo)
- **Security**: CodeQL security analysis findings MUST be addressed or explicitly justified with rationale
- **Generated Code**: All generated code (OpenAPI specs, Go types from specs, mocks via mockgen, Kubernetes API types via controller-gen) MUST be checked into source control and kept up-to-date via `make generate`
- **Dependencies**: Submodules (e.g., `bicep-types`) MUST be updated with `git submodule update --init --recursive` before building

## Development Workflow & Review

### Issue-First Development

Contributors MUST start by selecting an existing issue or creating a new issue on github.com/radius-project/radius before beginning work. For significant changes, maintainers MUST confirm the approach is in scope and aligns with project direction before implementation begins. Trivial changes (typos, minor documentation improvements) may proceed directly to PR without prior issue discussion.

### Commit and Pull Request Requirements

- All commits MUST include `Signed-off-by` line certifying Developer Certificate of Origin (use `git commit -s`)
- PRs MUST reference the issue they address in the description
- PR descriptions MUST explain what changed, why it changed, and any trade-offs considered
- Breaking changes MUST be clearly documented in PR descriptions with migration guidance
- PRs MUST pass all CI checks: `make build`, `make test`, `make lint`, `make format-check`
- Generated code MUST be up-to-date (run `make generate` and commit results if schemas or mocks changed)

### Design Specification Process for Major Features

Features requiring design specifications (new resource types, architectural changes, breaking changes) MUST follow the Spec Kit workflow in the `design-notes` repository:

1. **Constitution** (this document): Establish and validate project principles
2. **Specify** (`.specify/features/*/spec.md`): Define user scenarios, requirements, and success criteria (technology-agnostic)
3. **Plan** (`.specify/features/*/plan.md`): Create technical implementation plan with architecture, file structure, and constitution compliance check
4. **Tasks** (`.specify/features/*/tasks.md`): Break down plan into actionable, testable tasks organized by user story priority
5. **Implement**: Execute tasks according to plan with iterative validation and testing

Each phase MUST be reviewed and approved before proceeding to the next. The design note PR process in `design-notes` repository precedes implementation work in the `radius` repository.

### Code Review Standards

Reviewers MUST verify:

**For all repositories**:

- **Principle Alignment**: Design and implementation align with constitution principles
- **Testing**: Appropriate tests are present across the testing pyramid; tests were written before or during implementation
- **Documentation**: Changes are documented appropriately (inline comments, README updates, or docs changes)
- **Commit Hygiene**: Conventional commit messages; Signed-off-by present; no merge commits
- **Complexity**: Any violations of simplicity principles (e.g., new abstraction layers) are justified with concrete requirements
- **Incremental Adoption**: Changes altering existing workflows include migration guidance, optionality (flags or config), and do not silently break existing paths

**For radius repo**:

- **API Contracts**: APIs are properly versioned using TypeSpec; OpenAPI specs are generated and checked in
- **Generated Code**: `make generate` has been run and all generated files are current
- **Error Handling**: Errors are not suppressed without justification; specific error types are handled appropriately
- **Resource cleanup**: Resources are properly cleaned up (no leaks)
- **Bicep types**: Generated Bicep types are synchronized with TypeSpec changes

**For dashboard repo**:

- **Component stories**: Storybook stories demonstrate all interactive states
- **Accessibility**: Components are keyboard-navigable and screen-reader friendly
- **Type safety**: No `any` types without justification; prefer explicit typing
- **Performance**: No unnecessary re-renders or expensive operations in render paths

**For resource-types-contrib repo**:

- **Schema completeness**: All properties are documented with descriptions
- **Recipe security**: Recipes follow secure-by-default principles
- **Examples validity**: All examples are tested and runnable
- **Maturity labeling**: Alpha/Beta/Stable status is accurately assigned

**For docs repo**:

- **Build success**: Documentation builds without errors or warnings
- **Link validity**: All links resolve correctly (internal and external)
- **Code accuracy**: All code examples are tested and current
- **Framework alignment**: Content follows Diátaxis framework organization

## Governance

This constitution supersedes all other development practices for the Radius design notes repository. All design specifications, implementation plans, and pull requests MUST demonstrate compliance with these principles.

### Amendment Process

Amendments to this constitution require:

1. Proposal via GitHub issue in the `design-notes` repository with clear rationale for the change
2. Discussion and consensus among Radius maintainers and community stakeholders
3. Version bump according to semantic versioning:
   - **MAJOR**: Backward incompatible governance changes, principle removals, or fundamental redefinitions
   - **MINOR**: New principles added, sections materially expanded, or significant new guidance
   - **PATCH**: Clarifications, wording improvements, typo fixes, non-semantic refinements
4. Update to this document with Sync Impact Report documenting changes and affected artifacts
5. Approval from at least two maintainers before merging

### Compliance and Enforcement

All design specifications, plans, and implementation PRs MUST demonstrate compliance with this constitution. Maintainers MAY request changes to bring work into compliance with stated principles. Complexity that violates principles (especially Multi-Cloud Neutrality, Simplicity Over Cleverness, or Incremental Adoption & Backward Compatibility) MUST be justified with explicit trade-off analysis documented in the PR or design note.

### Periodic Review

This constitution MUST undergo a scheduled review at least quarterly (January, April, July, October) to assess relevance of principles, identify emerging gaps (e.g., security, observability evolution), and plan any prospective MINOR or MAJOR amendments transparently.

For day-to-day development guidance beyond this constitution, refer to:

- [CONTRIBUTING.md](https://github.com/radius-project/radius/blob/main/CONTRIBUTING.md) for contribution workflow
- [Developer guides](https://github.com/radius-project/radius/tree/main/docs/contributing) for detailed technical instructions
- [Code organization guide](https://github.com/radius-project/radius/blob/main/docs/contributing/contributing-code/contributing-code-organization/README.md) for repository structure

**Version**: 1.0.0 | **Ratified**: 2025-11-06 | **Last Amended**: 2025-11-07
