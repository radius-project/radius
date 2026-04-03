# Quickstart: Long-Running Tests Use Current Release

**Feature**: 001-lrt-current-release
**Date**: 2024-12-15

## Overview

This document provides a quick reference for implementing the workflow changes to use the current Radius release instead of building from main.

## High-Level Changes

### 1. Remove Build Job

The entire `build` job will be removed. It currently:

- Checks if build is needed based on time window
- Builds CLI and container images from source
- Publishes images to container registry
- Caches built artifacts

**After change**: No build job. CLI installed from official release.

### 2. Update Tests Job

The `tests` job will be modified to:

1. **Install CLI** (new step)

   ```bash
   wget -q "https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh" -O - | /bin/bash
   rad version  # Verify installation
   ```

2. **Detect Control Plane Version** (new step)

   ```bash
   # Parse rad version output to get CLI and control plane versions
   # Determine: not installed, same version, or different version
   ```

3. **Install/Upgrade Control Plane** (modified step)

   ```bash
   # If not installed: rad install kubernetes
   # If same version: skip (no action needed)
   # If different version: rad upgrade kubernetes
   ```

### 3. Remove Build-Related Environment Variables

Remove:

- `VALID_RADIUS_BUILD_WINDOW`
- Build-specific container registry settings

Keep:

- `BICEP_RECIPE_REGISTRY` (for test recipes)
- `TEST_BICEP_TYPES_REGISTRY` (for UDT types)
- Test infrastructure settings

### 4. Update Job Dependencies

- Remove `needs: build` from tests job
- Remove build job outputs from tests job env section

## Version Detection Logic

```bash
#!/bin/bash
# manage-radius-installation.sh

set -euo pipefail

# Get CLI version
CLI_VERSION=$(rad version | grep -A1 "RELEASE" | tail -1 | awk '{print $1}')

# Get control plane info
CP_INFO=$(rad version | grep -A1 "STATUS" | tail -1)
CP_STATUS=$(echo "$CP_INFO" | awk '{print $1}')
CP_VERSION=$(echo "$CP_INFO" | awk '{print $2}')

echo "CLI Version: $CLI_VERSION"
echo "Control Plane Status: $CP_STATUS"
echo "Control Plane Version: $CP_VERSION"

if [[ "$CP_STATUS" == "Not" ]]; then
    echo "Radius not installed. Installing..."
    rad install kubernetes
elif [[ "$CP_VERSION" == "$CLI_VERSION" ]]; then
    echo "Radius already at version $CLI_VERSION. No action needed."
else
    echo "Radius version mismatch. Attempting upgrade from $CP_VERSION to $CLI_VERSION..."
    rad upgrade kubernetes
fi
```

## Workflow Steps (Before → After)

### Before (build + tests jobs)

```text
build job:
  1. Restore cached binaries
  2. Check if build needed
  3. Checkout code
  4. Setup Go
  5. Build CLI and images
  6. Push images
  7. Upload artifacts
  8. Cache binaries

tests job:
  1. Download CLI artifact
  2. Checkout code
  3. Install Radius (if not skipped)
  4. Run tests
```

### After (tests job only)

```text
tests job:
  1. Checkout code
  2. Install CLI from release
  3. Verify CLI version
  4. Detect control plane version
  5. Install/upgrade control plane (conditional)
  6. Run tests
```

## Key Commands Reference

| Action                | Command                                                                                                      |
|-----------------------|--------------------------------------------------------------------------------------------------------------|
| Install CLI           | `wget -q "https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh" -O - | /bin/bash` |
| Check versions        | `rad version`                                                                                                |
| Install control plane | `rad install kubernetes`                                                                                     |
| Upgrade control plane | `rad upgrade kubernetes`                                                                                     |
| Download Bicep        | `rad bicep download`                                                                                         |

## Success Verification

After implementation, verify:

1. ✅ Workflow installs CLI via official script
2. ✅ `rad version` shows correct CLI version
3. ✅ Control plane version detection works
4. ✅ Fresh install works on empty cluster
5. ✅ Same-version scenario skips installation
6. ✅ Upgrade scenario attempts upgrade
7. ✅ Upgrade failure stops workflow with clear error
8. ✅ Functional tests execute successfully
