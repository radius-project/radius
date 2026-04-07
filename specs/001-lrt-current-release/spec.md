# Feature Specification: Long-Running Tests Use Current Release

**Feature Branch**: `001-lrt-current-release`  
**Created**: 2024-12-15  
**Status**: Draft  
**Input**: User description: "Update the long-running tests workflow to use the current Radius release instead of building from main"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run Tests Against Current Release (Priority: P1)

As a Radius maintainer, I want the long-running test workflow to use the current official Radius release or release candidate instead of building from the main branch, so that tests validate the released version that users actually consume.

**Why this priority**: This is the core value of the feature - ensuring long-running tests validate the same Radius version that end users install.

**Independent Test**: Can be fully tested by triggering the workflow and verifying it installs the current release version (not a build from main).

**Acceptance Scenarios**:

1. **Given** the workflow is triggered, **When** the workflow starts, **Then** it installs the current Radius CLI release using the official installer script.
2. **Given** the workflow has installed the CLI, **When** tests run, **Then** the CLI version matches the current official release.

---

### User Story 2 - Smart Control Plane Installation (Priority: P1)

As a Radius maintainer, I want the workflow to intelligently manage the Radius control plane on the test cluster, so that unnecessary installations are avoided and version mismatches are handled appropriately.

**Why this priority**: This is essential for the workflow to function correctly and efficiently, avoiding redundant work while ensuring version consistency.

**Independent Test**: Can be tested by running the workflow against clusters in various states (no Radius, same version, different version, edge version).

**Acceptance Scenarios**:

1. **Given** Radius is not installed on the cluster, **When** the workflow runs, **Then** it installs the current release version of the control plane.
2. **Given** Radius is installed on the cluster with the same version as the CLI, **When** the workflow runs, **Then** no installation activities occur.
3. **Given** Radius is installed on the cluster with a different version, **When** the workflow runs, **Then** it attempts to upgrade using the Radius upgrade command.

---

### User Story 3 - Graceful Upgrade Failure Handling (Priority: P2)

As a Radius maintainer, I want the workflow to stop gracefully when an upgrade is not possible, so that I receive clear feedback about incompatible version transitions.

**Why this priority**: Error handling is important for maintainability but is secondary to the core installation logic.

**Independent Test**: Can be tested by simulating an upgrade failure scenario and verifying the workflow stops with an appropriate error message.

**Acceptance Scenarios**:

1. **Given** an upgrade is attempted, **When** the Radius upgrade command reports that upgrade is not possible, **Then** the workflow reports an error and stops running.
2. **Given** an upgrade fails, **When** the workflow stops, **Then** the error message clearly indicates why the upgrade failed.

---

### User Story 4 - Simplified Workflow (Priority: P2)

As a Radius maintainer, I want all build-related logic removed from the workflow, so that the workflow is simpler and easier to maintain.

**Why this priority**: Reducing complexity improves maintainability but is a byproduct of the core change.

**Independent Test**: Can be verified by reviewing the workflow file to confirm no build steps remain.

**Acceptance Scenarios**:

1. **Given** the updated workflow, **When** reviewed, **Then** no steps for building Radius from source exist.
2. **Given** the updated workflow, **When** reviewed, **Then** no caching logic for built binaries exists.
3. **Given** the updated workflow, **When** reviewed, **Then** no "skip build" logic exists.

---

### Edge Cases

- What happens when the cluster is unreachable during version detection?
- How does the system handle network failures during CLI installation?
- What happens if the Radius installer script is temporarily unavailable?
- How does the workflow behave if the upgrade command hangs or times out?

## Requirements *(mandatory)*

### Functional Requirements

#### CLI Installation

- **FR-001**: System MUST install the current Radius CLI release using the official installer script (the same method end users use).
- **FR-002**: System MUST verify the installed CLI version after installation and fail immediately with a clear error message if verification fails.
- **FR-003**: System MUST install and use release candidates when a release candidate is published, treating release candidates as the latest version of Radius.

#### Build Logic Removal

> **Note**: "Build logic removal" refers to building the Radius CLI and control plane container images. The Radius codebase must still be present on the build runner because the functional tests are contained within the codebase. Running the test make targets will build the test code as a side effect—no separate build commands are required. The CLI itself is installed via the official installer script, not built locally.

- **FR-004**: System MUST NOT contain any steps to build the Radius CLI or control plane container images from source code.
- **FR-005**: System MUST NOT contain any caching logic for previously built binaries.
- **FR-006**: System MUST NOT contain any "skip build" conditional logic.
- **FR-007**: System MUST NOT contain any logic to determine if a build is required based on time windows.

#### Control Plane Version Detection

- **FR-008**: System MUST detect whether Radius is currently installed on the test cluster.
- **FR-009**: System MUST retrieve the version of the Radius control plane if installed.

#### Control Plane Installation Logic

- **FR-010**: System MUST install the current release control plane when Radius is not installed on the cluster.
- **FR-011**: System MUST skip installation when the installed control plane version matches the CLI version.
- **FR-012**: System MUST attempt an upgrade when the installed control plane version differs from the CLI version. The Radius upgrade command will determine if the transition is valid (including rejecting downgrades).

#### Upgrade Handling

- **FR-013**: System MUST rely on the Radius upgrade command to determine if an upgrade is possible.
- **FR-014**: System MUST report an error and stop execution if the upgrade command indicates upgrade is not possible.
- **FR-015**: System MUST provide a clear error message when upgrade fails.

### Assumptions

- The official Radius installer script is available and functional at the standard URL.
- The Radius CLI provides commands to detect the control plane version on a cluster.
- The Radius upgrade command returns appropriate exit codes or messages to indicate upgrade feasibility.
- The test cluster (AKS) is accessible and credentials are valid when the workflow runs.
- The "current release" refers to the latest published release of Radius, including release candidates when they are published. Release candidates are considered the latest version until superseded by a stable release or a newer release candidate.
- The Radius codebase must be checked out on the build runner because the functional tests reside in the repository.
- The make targets for running tests will build the test code as needed—no explicit build step is required.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Workflow completes successfully when run against a cluster with no Radius installed.
- **SC-002**: Workflow completes successfully when run against a cluster with the same Radius version as the CLI.
- **SC-003**: Workflow completes successfully when run against a cluster with an upgradeable Radius version.
- **SC-004**: Workflow fails gracefully with clear error message when upgrade is not possible.
- **SC-005**: Workflow file contains zero build-from-source steps after changes.
- **SC-006**: Workflow uses the same installation method as documented for end users.
- **SC-007**: Functional tests pass at the same rate as before the workflow changes (baseline comparison).

## Clarifications

### Session 2024-12-15

- Q: How should the workflow handle downgrades (cluster has newer version than CLI)? → A: Rely on Radius upgrade command to reject incompatible transitions
- Q: How should timeout behavior be handled for long-running operations? → A: Rely on existing job-level timeout and CLI built-in timeouts
- Q: What action to take if CLI version verification fails? → A: Fail workflow immediately with clear error message
- Q: How to determine if Radius is installed on the cluster? → A: Use rad version command; implementation details deferred to plan
