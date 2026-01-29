# Feature Specification: Repo Radius

**Feature Branch**: `001-repo-radius`
**Created**: 2026-01-28
**Updated**: 2026-01-29
**Status**: Draft
**Input**: Decentralized mode of Radius that runs as a CLI tool and stores state in a Git repository

## Overview

Repo Radius is a lightweight, Git-centric mode of Radius designed to run without a centralized control plane. It treats a Git repository as the system of record and is optimized for CI/CD workflows, particularly GitHub Actions. This specification focuses exclusively on Repo Radius (not Control Plane Radius).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize Repository for Repo Radius (Priority: P1)

As a developer, I want to initialize my Git repository for Repo Radius so that I can begin defining my infrastructure and application model in a Git-centric workflow.

**Why this priority**: Initialization is the entry point for all Repo Radius functionality. Without this, no other features can be used.

**Independent Test**: Can be fully tested by running `rad init` in a Git repository and verifying the directory structure is created, Resource Types are populated, and environment configuration is established.

**Acceptance Scenarios**:

1. **Given** a Git repository exists in the current working directory, **When** I run `rad init`, **Then** the system creates the `.radius/` directory structure with `config/`, `model/`, `plan/`, `deploy/`, and `graph/` subdirectories
2. **Given** I am not in a Git repository, **When** I run `rad init`, **Then** the system displays an error message indicating a Git repository is required
3. **Given** a `.radius/` directory already exists, **When** I run `rad init`, **Then** the system either preserves existing configuration or prompts for confirmation before overwriting
4. **Given** I run `rad init` in a valid Git repository, **When** initialization completes, **Then** the system creates Resource Types in `.radius/config/types/` from the Radius resource-types-contrib repository
5. **Given** existing `.env` files exist in the repository, **When** I run `rad init`, **Then** the system examines them for AWS, Azure, or Kubernetes configuration and validates their completeness
6. **Given** no `.env` file exists or it is insufficient, **When** I run `rad init`, **Then** the system prompts me to select a cloud platform (Local only, AWS, Azure, or Kubernetes only) and collects the required configuration
7. **Given** I select AWS or Azure as the cloud platform, **When** prompted for configuration, **Then** the system also collects Kubernetes details
8. **Given** initialization prompts are complete, **When** I am asked about deployment tooling, **Then** I can choose between Terraform or Bicep
9. **Given** I select a deployment tool, **When** initialization completes, **Then** the system creates a default `recipes.yaml` in `.radius/config/` based on my selection

---

### User Story 2 - Generate Deployment Scripts and Visualize Plan (Priority: P1)

As a developer, I want to generate deployment scripts from my application model and see a visual representation of what will be deployed so that I can understand and audit the infrastructure changes before deployment.

**Why this priority**: Script generation and visualization are the core value proposition of Repo Radius—translating an application model into executable deployment artifacts with clear visibility.

**Independent Test**: Can be fully tested by running `rad plan` with a valid application model and verifying deployment scripts are generated in `.radius/plan/` and a Mermaid diagram is output.

**Acceptance Scenarios**:

1. **Given** a valid application model exists in `.radius/model/`, **When** I run `rad plan`, **Then** the system generates deployment scripts in `.radius/plan/` based on the recipe manifest specified in the `.env` file
2. **Given** I run `rad plan`, **When** script generation completes, **Then** the system outputs a Mermaid diagram visualizing the application graph including the application model and the physical resources to be created or modified
3. **Given** the deployment engine is Terraform, **When** I run `rad plan`, **Then** the system executes `terraform plan` and stores the output in `.radius/plan/`
4. **Given** the deployment engine is Bicep, **When** I run `rad plan`, **Then** the system generates Bicep deployment scripts in `.radius/plan/`
5. **Given** no application model exists, **When** I run `rad plan`, **Then** the system displays an error indicating the model is missing
6. **Given** the Mermaid diagram is generated, **When** I view it, **Then** I can see both the logical application model and the physical resources that will be provisioned

---

### User Story 3 - Deploy from Git Commit (Priority: P1)

As a developer, I want to deploy infrastructure from a specific Git commit or tag so that I have an auditable, reproducible deployment process with visualization of what was deployed.

**Why this priority**: Deployment is the ultimate goal of the workflow. Requiring Git commits ensures auditability and prevents accidental deployment of uncommitted changes.

**Independent Test**: Can be fully tested by committing changes, running `rad deploy`, and verifying resources are deployed, details captured in `.radius/deploy/`, and Mermaid diagram is updated.

**Acceptance Scenarios**:

1. **Given** I run `rad deploy`, **When** the command starts, **Then** the system validates that required environment variables are set for the target platform (AWS_ACCOUNT_ID, AWS_REGION for AWS; AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID for Azure)
2. **Given** deployment scripts exist in `.radius/plan/` and are committed to Git, **When** I run `rad deploy` specifying a commit hash or tag, **Then** the system orchestrates the application deployment by executing the deployment scripts from that commit
3. **Given** I have uncommitted changes in `.radius/plan/`, **When** I run `rad deploy`, **Then** the system refuses to deploy and displays an error requiring committed changes
4. **Given** a successful deployment completes, **When** the deployment finishes, **Then** the system captures and stores resource details in `.radius/deploy/` including Environment used, cloud resource IDs, and full resource properties as returned by the cloud platform
5. **Given** a successful deployment completes, **When** resource details are captured, **Then** the system updates the Mermaid diagram in `.radius/graph/` with the physical resources that were created or modified
6. **Given** required environment variables are not set, **When** I run `rad deploy`, **Then** the system displays a clear error indicating which variables are missing
7. **Given** a deployment fails, **When** the failure occurs, **Then** the system provides clear error output indicating what failed and why

---

### User Story 4 - Configure Multiple Environments (Priority: P2)

As a developer, I want to configure multiple deployment environments so that I can deploy the same application model to different cloud accounts, regions, or clusters.

**Why this priority**: Multi-environment support enables the dev/staging/production workflow that enterprise teams require.

**Independent Test**: Can be fully tested by creating multiple `.env` files and verifying `rad plan` and `rad deploy` respect the selected environment.

**Acceptance Scenarios**:

1. **Given** I need a default environment, **When** I create environment configuration, **Then** I store it in `.env` in the repository root
2. **Given** I need additional named environments, **When** I create environment configuration, **Then** I store it in `.env.<ENVIRONMENT_NAME>` (e.g., `.env.production`, `.env.staging`)
3. **Given** I need to deploy to AWS, **When** I create an environment configuration, **Then** I can specify AWS account and region without including credentials
4. **Given** I need to deploy to Azure, **When** I create an environment configuration, **Then** I can specify Azure subscription and resource group
5. **Given** I need to deploy to Kubernetes, **When** I create an environment configuration, **Then** I can specify the Kubernetes context name and namespace
6. **Given** I have multiple environments configured, **When** I run `rad plan` or `rad deploy`, **Then** I can specify which environment to target

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

### Edge Cases

- What happens when `rad init` is run in a non-Git directory?
  - System MUST display a clear error message and refuse to initialize
- What happens when `rad deploy` is run with uncommitted changes in `.radius/plan/`?
  - System MUST refuse deployment and require changes to be committed
- What happens when the specified Environment does not exist?
  - System MUST display an error listing available environments
- What happens when the recipe manifest specified in `.env` does not exist?
  - System MUST fail with a clear error during `rad plan`
- What happens when required environment variables (AWS_ACCOUNT_ID, AZURE_CLIENT_ID, etc.) are missing at deployment time?
  - System MUST fail with a clear error listing the missing variables without exposing any credential values
- What happens when `rad init` finds an existing `.env` file without required cloud configuration?
  - System MUST prompt the user to provide missing configuration

## Requirements *(mandatory)*

### Functional Requirements

#### Execution Model

- **FR-001**: System MUST run as a single executable on Windows, Linux, and macOS
- **FR-002**: System MUST be installable via WinGet (Windows), Homebrew (macOS), apt (Debian/Ubuntu), and dnf (Fedora/RHEL)
- **FR-003**: System MUST be optimized for non-interactive execution in GitHub Actions
- **FR-004**: System MUST expose a command surface similar to the existing `rad` CLI

#### `rad init` Command

- **FR-010**: System MUST verify the current working directory is a Git repository before initialization
- **FR-011**: System MUST create the directory structure: `.radius/config/`, `.radius/model/`, `.radius/plan/`, `.radius/deploy/`, `.radius/graph/`
- **FR-012**: System MUST create Resource Types in `.radius/config/types/` from the Radius resource-types-contrib repository
- **FR-013**: System MUST search the repository for existing `.env` files and validate they contain cloud platform configuration (AWS account/region, Azure subscription/resource group, or Kubernetes context/namespace)
- **FR-014**: System MUST prompt the user to select a cloud platform if no `.env` file exists or existing files are insufficient: Local only, AWS, Azure, or Kubernetes only
- **FR-015**: System MUST collect Kubernetes details in addition to cloud provider details when AWS or Azure is selected
- **FR-016**: System MUST prompt the user to select a deployment tool: Terraform or Bicep
- **FR-017**: System MUST create a default `recipes.yaml` in `.radius/config/` based on the selected deployment tool

#### `rad plan` Command

- **FR-020**: System MUST generate ready-to-execute deployment scripts (bash or PowerShell)
- **FR-021**: System MUST derive deployment scripts from the recipe manifest specified in the `.env` file
- **FR-022**: System MUST support Terraform (plan/apply) as a deployment engine
- **FR-023**: System MUST support Bicep as a deployment engine
- **FR-024**: Generated scripts are captured for auditability and to be part of the application graph (not intended for user modification)
- **FR-025**: Generated scripts MUST be stored in `.radius/plan/`
- **FR-026**: When the deployment engine is Terraform, system MUST execute `terraform plan` and store the output in `.radius/plan/`
- **FR-027**: System MUST output a Mermaid diagram visualizing the application graph including the application model and the physical resources to be created or modified
- **FR-028**: System MUST store Mermaid diagrams in `.radius/graph/`

#### `rad deploy` Command

- **FR-030**: System MUST validate that required environment variables are set for the target platform before deployment (AWS_ACCOUNT_ID, AWS_REGION for AWS; AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID for Azure)
- **FR-031**: System MUST orchestrate application deployment by executing deployment scripts only from a Git commit hash or tag
- **FR-032**: System MUST NOT deploy directly from uncommitted local files
- **FR-033**: System MUST capture structured details about deployed resources after deployment
- **FR-034**: System MUST record the Environment used for each deployment
- **FR-035**: System MUST record cloud platform resource IDs for each deployed resource
- **FR-036**: System MUST record the full set of properties for each deployed resource as returned by the cloud platform
- **FR-037**: System MUST store deployment details in `.radius/deploy/`
- **FR-038**: System MUST update the Mermaid diagram in `.radius/graph/` with the physical resources that were created or modified after deployment

#### Configuration Model (Input Files)

- **FR-040**: All configuration MUST be stored in the Git repository
- **FR-041**: Resource Types MUST be stored as YAML files in `.radius/config/types/` (same format as Radius Resource Types today without modification)
- **FR-042**: Default Environment configuration MUST be stored in `.env` file in the repository root
- **FR-043**: Named Environment configurations MUST be stored as `.env.<ENVIRONMENT_NAME>` files
- **FR-044**: Environment files MUST support AWS account and region configuration
- **FR-045**: Environment files MUST support Azure subscription and resource group configuration
- **FR-046**: Environment files MUST support Kubernetes context name and namespace configuration
- **FR-047**: Environment files MUST support specifying an alternative `recipes.yaml` manifest file
- **FR-048**: Environment files MUST support Terraform CLI configuration (`terraformrc`)
- **FR-049**: Environment files MUST NOT contain credentials
- **FR-050**: Recipes MUST be stored as a YAML file in the `.radius/config/` directory

### Explicit Non-Goals

- **NG-001**: Repo Radius does NOT have a concept of Radius Resource Groups
- **NG-002**: Repo Radius does NOT have a formal Environment object beyond the simple `.env.<ENVIRONMENT_NAME>` files
- **NG-003**: Repo Radius does NOT have a Terraform Settings object because it relies upon the user's existing Terraform configuration in their execution environment
- **NG-004**: Repo Radius does NOT have Credentials or Bicep Settings objects because it uses the existing authentication to an OCI registry in the user's execution environment

### Key Entities

- **Environment**: A deployment target configuration specifying cloud provider details (AWS account/region, Azure subscription/resource group, Kubernetes context/namespace), recipe manifest reference, and deployment engine settings. Default stored in `.env`; named environments in `.env.<NAME>`. Does not contain credentials. No formal Environment object exists—configuration is file-based only.
- **Recipes**: A YAML file (`recipes.yaml`) that defines how abstract resource types are deployed to specific cloud platforms. Stored in `.radius/config/`. Can be overridden per-environment via `.env` configuration.
- **Application Model**: The user-defined model describing the application and its resource dependencies. Stored in `.radius/model/`. Produced by a separate project (out of scope).
- **Deployment Script**: A ready-to-execute script (bash or PowerShell) generated by `rad plan` that deploys infrastructure using a supported deployment engine. Stored in `.radius/plan/`. Not intended for user modification.
- **Deployment Record**: Structured details captured after `rad deploy` completes, including Environment, resource IDs, and full resource properties. Stored in `.radius/deploy/`.
- **Resource Type**: A definition of an abstract resource type stored as YAML (same format as existing Radius Resource Types). Stored in `.radius/config/types/`. Initialized from the Radius resource-types-contrib repository.
- **Application Graph**: A Mermaid diagram visualizing both the logical application model and the physical resources provisioned. Stored in `.radius/graph/`. Updated during `rad plan` (planned resources) and `rad deploy` (actual resources).

### Assumptions

- The application model in `.radius/model/` is produced by a separate project and is available at `rad plan` time
- Cloud provider credentials are provided via environment variables (AWS_ACCOUNT_ID, AWS_REGION, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID) and are not managed by Repo Radius
- Users have Git installed and understand basic Git operations
- The Radius resource-types-contrib repository is accessible during `rad init`
- Users have their own Terraform configuration in their execution environment (Repo Radius does not manage Terraform settings)
- Users have existing OCI registry authentication configured (Repo Radius does not manage Bicep/OCI credentials)
- Generated deployment artifacts are expected to become part of the Radius application graph in the future (future enhancement)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a repository for Repo Radius in under 2 minutes (including interactive prompts)
- **SC-002**: Users can generate deployment scripts and Mermaid diagram with `rad plan` in under 2 minutes for a typical application model
- **SC-003**: 90% of GitHub Actions workflows using Repo Radius complete without manual intervention
- **SC-004**: Users can deploy infrastructure with `rad deploy` and have full resource details captured automatically
- **SC-005**: Users can install Repo Radius via their native package manager without additional manual steps
- **SC-006**: Users can view a Mermaid diagram showing both planned and deployed resources

## Future Enhancements

The following features are out of scope for the initial implementation but are planned for future releases:

- **FE-001**: Adapt to existing deployment scripts by examining the Git repository for existing IaC code (`*.tf`, `*.bicep`, `chart.yaml`, `kustomize.yaml`, `template.yaml`)
- **FE-002**: Adapt to existing GitOps configurations
- **FE-003**: Support for Helm as a deployment engine
- **FE-004**: Support for CloudFormation as a deployment engine
- **FE-005**: Support for Crossplane as a deployment engine
- **FE-006**: Support for Ansible as a deployment engine

## Open Questions

The following items require clarification before implementation:

### Question 1: GitHub Actions Optimization

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

### Question 2: Deployment Record File Format

**Context**: `rad deploy` captures "structured details about the deployed resources" in `.radius/deploy/`.

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

---

### Question 3: AWS State Store for Idempotency

**Context**: Radius needs to maintain idempotency when deploying AWS resources across multiple `rad deploy` invocations.

**What we need to know**: Does Radius need a state store for AWS resources to maintain idempotency? The assumption is "no" if only Terraform is used (Terraform manages its own state), but "yes" may be required if Bicep is supported for AWS deployments.

**Suggested Answers**:

| Option | Answer                                              | Implications                                                                   |
| ------ | --------------------------------------------------- | ------------------------------------------------------------------------------ |
| A      | No state store needed (Terraform-only for AWS)      | Simpler implementation; limits AWS deployments to Terraform                    |
| B      | Radius-managed state store for Bicep AWS support    | Enables Bicep for AWS but requires state management implementation             |
| C      | Defer Bicep AWS support to future enhancement       | Avoids state store complexity now; limits initial AWS support to Terraform     |
| D      | Use external state store (S3, DynamoDB)             | Leverages existing AWS services but adds configuration complexity              |
| Custom | Provide your own answer                             | Specify approach and rationale                                                 |

**Your choice**: _[Awaiting user response]_
