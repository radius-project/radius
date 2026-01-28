# Feature Specification: Repo Radius

**Feature Branch**: `001-repo-radius`
**Created**: 2026-01-28
**Status**: Draft
**Input**: Decentralized mode of Radius that runs as a CLI tool and stores state in a Git repository

## Overview

Repo Radius is a lightweight, Git-centric mode of Radius designed to run without a centralized control plane. It treats a Git repository as the system of record and is optimized for CI/CD workflows, particularly GitHub Actions. This specification focuses exclusively on Repo Radius (not Control Plane Radius).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Repo Radius (Priority: P1)

As a developer, I want to initialize my Git repository for Repo Radius so that I can begin defining my infrastructure and application model in a Git-centric workflow.

**Why this priority**: Initialization is the entry point for all Repo Radius functionality. Without this, no other features can be used.

**Independent Test**: Can be fully tested by running `rad init` in a Git repository and verifying the directory structure is created with appropriate defaults.

**Acceptance Scenarios**:

1. **Given** a Git repository exists in the current working directory, **When** I run `rad init`, **Then** the system creates the `rad/` directory structure with `config/`, `model/`, `plan/`, and `deploy/` subdirectories
2. **Given** I am not in a Git repository, **When** I run `rad init`, **Then** the system displays an error message indicating a Git repository is required
3. **Given** a `rad/` directory already exists, **When** I run `rad init`, **Then** the system either preserves existing configuration or prompts for confirmation before overwriting

---

### User Story 2 - Generate Deployment Scripts (Priority: P1)

As a developer, I want to generate deployment scripts from my application model so that I can preview and optionally customize the infrastructure changes before deployment.

**Why this priority**: Script generation is the core value proposition of Repo Radius—translating an application model into executable deployment artifacts.

**Independent Test**: Can be fully tested by running `rad plan` with a valid application model and verifying deployment scripts are generated in `rad/plan/`.

**Acceptance Scenarios**:

1. **Given** a valid application model exists in `rad/model/`, **When** I run `rad plan`, **Then** the system generates deployment scripts in `rad/plan/` based on the Recipes specified in the selected Environment
2. **Given** the Environment specifies Terraform as the deployment engine, **When** I run `rad plan`, **Then** the system generates Terraform plan/apply scripts
3. **Given** the Environment specifies Bicep as the deployment engine, **When** I run `rad plan`, **Then** the system generates Bicep deployment scripts
4. **Given** no application model exists, **When** I run `rad plan`, **Then** the system displays an error indicating the model is missing

---

### User Story 3 - Deploy from Git Commit (Priority: P1)

As a developer, I want to deploy infrastructure from a specific Git commit or tag so that I have an auditable, reproducible deployment process.

**Why this priority**: Deployment is the ultimate goal of the workflow. Requiring Git commits ensures auditability and prevents accidental deployment of uncommitted changes.

**Independent Test**: Can be fully tested by committing changes, running `rad deploy`, and verifying resources are deployed and details captured in `rad/deploy/`.

**Acceptance Scenarios**:

1. **Given** deployment scripts exist in `rad/plan/` and are committed to Git, **When** I run `rad deploy` specifying a commit hash or tag, **Then** the system executes the deployment scripts from that commit
2. **Given** I have uncommitted changes in `rad/plan/`, **When** I run `rad deploy`, **Then** the system refuses to deploy and displays an error requiring committed changes
3. **Given** a successful deployment completes, **When** the deployment finishes, **Then** the system captures and stores resource details in `rad/deploy/` including Environment used, cloud resource IDs, and full resource properties
4. **Given** a deployment fails, **When** the failure occurs, **Then** the system provides clear error output indicating what failed and why

---

### User Story 4 - Configure Environments (Priority: P2)

As a developer, I want to configure multiple deployment environments so that I can deploy the same application model to different cloud accounts, regions, or clusters.

**Why this priority**: Multi-environment support enables the dev/staging/production workflow that enterprise teams require.

**Independent Test**: Can be fully tested by creating multiple `.env` files in `rad/config/` and verifying `rad plan` and `rad deploy` respect the selected environment.

**Acceptance Scenarios**:

1. **Given** I need to deploy to AWS, **When** I create an environment configuration, **Then** I can specify AWS account and region in a `.env` file without including credentials
2. **Given** I need to deploy to Azure, **When** I create an environment configuration, **Then** I can specify Azure subscription and resource group
3. **Given** I need to deploy to Kubernetes, **When** I create an environment configuration, **Then** I can specify the cluster API endpoint and namespace
4. **Given** I have multiple environments configured, **When** I run `rad plan` or `rad deploy`, **Then** I can specify which environment to target

---

### User Story 5 - Install via Package Manager (Priority: P2)

As a developer, I want to install Repo Radius using my operating system's native package manager so that installation is simple and follows platform conventions.

**Why this priority**: Easy installation reduces friction for adoption and enables consistent tooling across development teams.

**Independent Test**: Can be fully tested by installing via the appropriate package manager and verifying `rad` commands are available.

**Acceptance Scenarios**:

1. **Given** I am on Windows, **When** I run `winget install radius`, **Then** the Repo Radius CLI is installed and available in my PATH
2. **Given** I am on macOS, **When** I run `brew install radius`, **Then** the Repo Radius CLI is installed and available in my PATH
3. **Given** I am on Linux (Debian/Ubuntu), **When** I run `apt install radius`, **Then** the Repo Radius CLI is installed
4. **Given** I am on Linux (Fedora/RHEL), **When** I run `dnf install radius`, **Then** the Repo Radius CLI is installed

---

### User Story 6 - Run in GitHub Actions (Priority: P2)

As a CI/CD engineer, I want to run Repo Radius in a GitHub Actions workflow so that I can automate infrastructure deployment as part of my CI/CD pipeline.

**Why this priority**: CI/CD integration is explicitly called out as the primary optimization target for Repo Radius.

**Independent Test**: Can be fully tested by creating a GitHub Actions workflow that runs `rad plan` and `rad deploy` and verifying successful execution.

**Acceptance Scenarios**:

1. **Given** a GitHub Actions workflow, **When** I invoke `rad plan`, **Then** the command completes successfully in non-interactive mode
2. **Given** a GitHub Actions workflow, **When** I invoke `rad deploy` with a commit reference, **Then** the deployment executes without requiring interactive input
3. **Given** deployment scripts were generated in a previous workflow step, **When** I run `rad deploy` in a subsequent step, **Then** the deployment uses the committed scripts

---

### User Story 7 - Run Deployment Scripts Independently (Priority: P3)

As a developer, I want the option to run generated deployment scripts directly (outside of Radius) so that I have flexibility in how I execute deployments.

**Why this priority**: Provides escape hatch for advanced users who want more control or need to integrate with existing deployment tooling.

**Independent Test**: Can be fully tested by generating scripts with `rad plan`, then executing them directly using Terraform CLI or Azure CLI.

**Acceptance Scenarios**:

1. **Given** Terraform scripts were generated by `rad plan`, **When** I run `terraform plan` and `terraform apply` directly, **Then** the infrastructure is deployed successfully
2. **Given** Bicep scripts were generated by `rad plan`, **When** I run `az deployment` directly, **Then** the infrastructure is deployed successfully

---

### Edge Cases

- What happens when `rad init` is run in a non-Git directory?
  - System MUST display a clear error message and refuse to initialize
- What happens when `rad deploy` is run with uncommitted changes in `rad/plan/`?
  - System MUST refuse deployment and require changes to be committed
- What happens when the specified Environment does not exist?
  - System MUST display an error listing available environments
- What happens when deployment scripts reference a Recipe Pack that doesn't exist?
  - System MUST fail with a clear error during `rad plan`
- What happens when cloud credentials are missing or invalid at deployment time?
  - System MUST fail with a clear error without exposing credential details

## Requirements *(mandatory)*

### Functional Requirements

#### Execution Model

- **FR-001**: System MUST run as a single executable on Windows, Linux, and macOS
- **FR-002**: System MUST be installable via WinGet (Windows), Homebrew (macOS), apt (Debian/Ubuntu), and dnf (Fedora/RHEL)
- **FR-003**: System MUST NOT require Kubernetes for operation (though it MAY use Kind or k3d as a short-lived job behind the scenes in the short term)
- **FR-004**: System MUST be optimized for non-interactive execution in GitHub Actions
- **FR-005**: System MUST expose a command surface similar to the existing `rad` CLI

#### `rad init` Command

- **FR-010**: System MUST verify the current working directory is a Git repository before initialization
- **FR-011**: System MUST create the directory structure: `rad/config/`, `rad/model/`, `rad/plan/`, `rad/deploy/`
- **FR-012**: System MUST create friendly default configuration in `rad/config/`

#### `rad plan` Command

- **FR-020**: System MUST generate ready-to-execute deployment scripts (bash or PowerShell)
- **FR-021**: System MUST derive deployment scripts from Recipes specified by the Recipe Pack selected in the Environment
- **FR-022**: System MUST support Terraform (plan/apply) as a deployment engine
- **FR-023**: System MUST support Bicep as a deployment engine
- **FR-024**: Generated scripts MUST be executable independently by the user outside of Radius
- **FR-025**: Generated scripts MUST be stored in `rad/plan/`

#### `rad deploy` Command

- **FR-030**: System MUST execute deployment scripts only from a Git commit hash or tag
- **FR-031**: System MUST NOT deploy directly from uncommitted local files
- **FR-032**: System MUST capture structured details about deployed resources after deployment
- **FR-033**: System MUST record the Environment used for each deployment
- **FR-034**: System MUST record cloud platform resource IDs for each deployed resource
- **FR-035**: System MUST record the full set of properties for each deployed resource as returned by the cloud platform
- **FR-036**: System MUST store deployment details in `rad/deploy/`

#### Configuration Model

- **FR-040**: All configuration MUST be stored in the Git repository under `rad/config/`
- **FR-041**: Resource Types MUST be stored as YAML or TypeSpec files
- **FR-042**: Environment configuration MUST be stored as `.env` files
- **FR-043**: Environment files MUST support AWS account and region configuration
- **FR-044**: Environment files MUST support Azure subscription and resource group configuration
- **FR-045**: Environment files MUST support Kubernetes cluster API endpoint and namespace configuration
- **FR-046**: Environment files MUST support Recipe Pack selection
- **FR-047**: Environment files MUST support Terraform CLI configuration (`terraformrc`)
- **FR-048**: Environment files MUST NOT contain credentials
- **FR-049**: Recipe Packs MUST be stored in the `rad/config/` directory

### Explicit Non-Goals

- **NG-001**: Repo Radius does NOT have a concept of Radius Resource Groups
- **NG-002**: Repo Radius does NOT introduce new commands related to Resource Groups

### Key Entities

- **Environment**: A deployment target configuration specifying cloud provider details (account, region, subscription, resource group, cluster endpoint, namespace), selected Recipe Pack, and deployment engine settings. Stored as `.env` files. Does not contain credentials.
- **Recipe Pack**: A collection of Recipes that define how abstract resource types are deployed to specific cloud platforms. Stored in `rad/config/`.
- **Application Model**: The user-defined model describing the application and its resource dependencies. Stored in `rad/model/`. Produced by a separate project (out of scope).
- **Deployment Script**: A ready-to-execute script (bash or PowerShell) generated by `rad plan` that deploys infrastructure using a supported deployment engine. Stored in `rad/plan/`.
- **Deployment Record**: Structured details captured after `rad deploy` completes, including Environment, resource IDs, and full resource properties. Stored in `rad/deploy/`.
- **Resource Type**: A definition of an abstract resource type stored as YAML or TypeSpec. Stored in `rad/config/`.

### Assumptions

- The application model in `rad/model/` is produced by a separate project and is available at `rad plan` time
- Cloud provider credentials are provided externally (via environment variables, credential files, or CI/CD secrets) and are not managed by Repo Radius
- Users have Git installed and understand basic Git operations
- Generated deployment artifacts are expected to become part of the Radius application graph in the future (future enhancement)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a repository for Repo Radius in under 30 seconds
- **SC-002**: Users can generate deployment scripts with `rad plan` in under 2 minutes for a typical application model
- **SC-003**: 90% of GitHub Actions workflows using Repo Radius complete without manual intervention
- **SC-004**: Users can deploy infrastructure with `rad deploy` and have full resource details captured automatically
- **SC-005**: Users can install Repo Radius via their native package manager without additional manual steps
- **SC-006**: Generated deployment scripts can be executed independently (outside Radius) without modification

## Open Questions

The following items require clarification before implementation:

### Question 1: Recipe Pack File Format

**Context**: Recipe Packs are stored in `rad/config/` but the file format is not specified.

**What we need to know**: What file format should Recipe Packs use?

**Suggested Answers**:

| Option | Answer                       | Implications                                                                 |
| ------ | ---------------------------- | ---------------------------------------------------------------------------- |
| A      | Bicep files                  | Aligns with existing Radius Recipes; familiar to Azure users                 |
| B      | YAML files                   | Human-readable; easy to edit; portable across tools                          |
| C      | Key-value (`.env` style)     | Simple but limited expressiveness; may not capture complex recipe structures |
| D      | TypeSpec files               | Strongly typed; aligns with Resource Types; requires TypeSpec tooling        |
| Custom | Provide your own answer      | Specify the format and rationale                                             |

**Your choice**: _[Awaiting user response]_

---

### Question 2: GitHub Actions Optimization

**Context**: Repo Radius is described as "optimized for non-interactive execution in a GitHub Action."

**What we need to know**: What specific features or behaviors would make Repo Radius particularly well-suited for GitHub Actions?

**Suggested Answers**:

| Option | Answer                                          | Implications                                                             |
| ------ | ----------------------------------------------- | ------------------------------------------------------------------------ |
| A      | Exit codes and structured output (JSON/YAML)    | Enables workflow conditionals and artifact parsing                       |
| B      | GitHub Actions marketplace action               | One-line integration; managed versioning                                 |
| C      | OIDC authentication support for cloud providers | No long-lived secrets; aligns with GitHub security best practices        |
| D      | All of the above                                | Maximum integration but larger implementation scope                      |
| Custom | Provide your own answer                         | Specify features and rationale                                           |

**Your choice**: _[Awaiting user response]_

---

### Question 3: Deployment Record File Format

**Context**: `rad deploy` captures "structured details about the deployed resources" in `rad/deploy/`.

**What we need to know**: What file format should be used to store deployed resource details?

**Suggested Answers**:

| Option | Answer                     | Implications                                                         |
| ------ | -------------------------- | -------------------------------------------------------------------- |
| A      | YAML                       | Human-readable; easy to diff in Git; widely supported                |
| B      | JSON                       | Machine-readable; easy to parse; may be verbose for large outputs    |
| C      | Terraform state file       | Native if using Terraform; but creates coupling to deployment engine |
| D      | Custom structured format   | Optimized for Radius but requires documentation and tooling          |
| Custom | Provide your own answer    | Specify the format and rationale                                     |

**Your choice**: _[Awaiting user response]_
