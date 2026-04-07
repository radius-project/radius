# Research: Long-Running Tests Use Current Release

**Feature**: 001-lrt-current-release
**Date**: 2024-12-15

## Summary

This document consolidates research findings for implementing the long-running test workflow changes. All NEEDS CLARIFICATION items from the Technical Context have been resolved.

## Research Tasks & Findings

### 1. Radius CLI Installation Method

**Task**: Determine the official method for installing the Radius CLI that end users use.

**Finding**: The official Radius installer script is available at:

```bash
wget -q "https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh" -O - | /bin/bash
```

This script:

- Detects the operating system and architecture
- Downloads the appropriate binary from GitHub releases
- Installs to `/usr/local/bin/rad` by default
- Requires `wget` and `bash` (both available on GitHub runners)

**Decision**: Use the official installer script via wget
**Rationale**: Matches the end-user experience exactly as specified in FR-001
**Alternatives Considered**:

- Download binary directly from GitHub releases (rejected: diverges from documented user method)
- Use `go install` (rejected: requires building from source)

### 2. Version Detection via rad version

**Task**: Determine how to parse CLI and control plane versions from `rad version` output.

**Finding**: The `rad version` command has two relevant output formats:

**Text format (default)**:

```text
CLI Version Information:
RELEASE   VERSION   BICEP     COMMIT
0.54.0    v0.54.0   0.39.26   f06410904c8b92bcc3aaa1f1ed6450981e510107

Control Plane Information:
STATUS     VERSION
Installed  0.54.0
```

**JSON format** (`--output json`): Outputs invalid JSON (two separate JSON objects), making it unreliable for parsing.

**Parsing approach for text format**:

```bash
# Extract CLI version
CLI_VERSION=$(rad version | grep -A1 "RELEASE" | tail -1 | awk '{print $1}')

# Extract control plane version and status
CP_INFO=$(rad version | grep -A1 "STATUS" | tail -1)
CP_STATUS=$(echo "$CP_INFO" | awk '{print $1}')
CP_VERSION=$(echo "$CP_INFO" | awk '{print $2}')
```

**Decision**: Parse the text output using `grep` and `awk`
**Rationale**: The JSON output is malformed; text parsing is reliable and straightforward
**Alternatives Considered**:

- JSON parsing with `jq` (rejected: invalid JSON from CLI)
- Only checking CLI version (rejected: need control plane version for comparison)

### 3. Control Plane Status Detection

**Task**: Determine how to detect if Radius is installed on the cluster.

**Finding**: The `rad version` command reports control plane status as:

- `Installed` - Radius is installed with the given version
- `Not Installed` - Radius is not present on the cluster

When not installed, the output shows:

```text
Control Plane Information:
STATUS         VERSION
Not Installed
```

**Decision**: Check for "Installed" vs "Not Installed" in the STATUS column
**Rationale**: Direct output from CLI is authoritative
**Alternatives Considered**:

- Check for Helm releases (rejected: adds dependency on Helm CLI)
- Check for Kubernetes namespace (rejected: namespace may exist without full installation)

### 4. Upgrade Command Behavior

**Task**: Understand `rad upgrade kubernetes` behavior and error handling.

**Findings**:

1. **Same version upgrade attempt**: Returns non-zero exit code with message "Target version is the same as current version"

2. **Preflight check option**: `--preflight-only` flag runs checks without upgrading, useful for validation

3. **Exit codes**:
   - Success: 0
   - Preflight failure: non-zero (exact code varies)
   - Version compatibility failure: non-zero

4. **Version compatibility**: The upgrade command handles all version compatibility checks including:
   - Same version (fails with clear message)
   - Incompatible downgrades (fails with clear message)
   - Supported upgrade paths (succeeds)

**Decision**: Run `rad upgrade kubernetes` directly and let it fail naturally for incompatible transitions
**Rationale**: The CLI provides clear error messages; no need to duplicate validation logic
**Alternatives Considered**:

- Use `--preflight-only` first (rejected: adds complexity, CLI already handles this)
- Manual version comparison (rejected: duplicates CLI logic, may miss edge cases)

### 5. Fresh Installation Method

**Task**: Determine how to install Radius when not present on cluster.

**Finding**: Use `rad install kubernetes` command:

```bash
rad install kubernetes
```

The command will install the control plane version matching the CLI version by default.

**Decision**: Use `rad install kubernetes` for fresh installations
**Rationale**: Standard CLI command, matches CLI version automatically
**Alternatives Considered**:

- Helm install directly (rejected: CLI abstracts Helm complexity)

### 6. Workflow Steps to Remove (Build Logic)

**Task**: Identify all build-related steps to remove from the workflow.

**Finding**: The following should be removed from the `build` job:

- Restore cached binaries step
- Skip build if valid step
- Set up checkout target steps
- Generate ID for release step
- Login to Azure (build-specific)
- Build and Push container images step
- Upload CLI binary step
- Log build result steps
- Move/Store binaries to cache steps
- Publish UDT types step (uses built binary)
- Publish Bicep Test Recipes step (uses built binary)

The entire `build` job can be removed or significantly simplified.

**Decision**: Remove the `build` job entirely; move necessary setup (checkout, Go setup) to `tests` job
**Rationale**: No build artifacts needed; CLI comes from installer
**Alternatives Considered**:

- Keep build job as placeholder (rejected: unnecessary complexity)

### 7. Environment Variables to Update

**Task**: Identify environment variables related to build that should be removed.

**Finding**: The following env vars are build-related and can be removed:

- `VALID_RADIUS_BUILD_WINDOW` - No longer needed (no time-based build logic)
- `CONTAINER_REGISTRY` - Only needed for pushing built images
- Container image settings (`rp.image`, `rp.tag`, etc.) - Will use release images

The following should be retained:

- `BICEP_RECIPE_REGISTRY` - Still needed for test recipes
- `TEST_BICEP_TYPES_REGISTRY` - Still needed for UDT types

**Decision**: Remove build-specific env vars, retain test infrastructure vars
**Rationale**: Clean up unused configuration
**Alternatives Considered**: N/A

### 8. Test Recipe Publishing

**Task**: Determine how test recipes should be published without built CLI.

**Finding**: The test recipes can still be published using the installed CLI:

```bash
rad bicep download
make publish-test-bicep-recipes
```

The UDT types step also uses the CLI and can work with the installed version.

**Decision**: Move recipe publishing to the `tests` job after CLI installation
**Rationale**: Uses same CLI, just installed differently
**Alternatives Considered**:

- Skip recipe publishing (rejected: still needed for tests)

## Implementation Decisions Summary

| Decision          | Choice                            | Key Reason                           |
|-------------------|-----------------------------------|--------------------------------------|
| CLI Installation  | Official installer script         | Matches end-user experience (FR-001) |
| Version Parsing   | Text output with grep/awk         | JSON output is malformed             |
| Status Detection  | Check "Installed"/"Not Installed" | Direct CLI output                    |
| Upgrade Handling  | Let CLI handle errors             | CLI provides clear messages          |
| Build Job         | Remove entirely                   | No longer needed                     |
| Recipe Publishing | Move to tests job                 | Still needed, uses installed CLI     |

## Open Items

None. All research items resolved.
