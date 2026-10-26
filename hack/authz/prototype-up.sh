#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# PROTOTYPE: one command to build everything from this checkout and install
# Radius with internal component authorization on a local kind cluster.
# Wraps up.sh: builds rad, builds images for the host architecture, and maps
# the "default" namespace for the controller. See hack/authz/PROTOTYPE.md.
# ============================================================================

set -euo pipefail

# shellcheck source=hack/authz/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() {
    cat <<EOF
Usage: $(basename "$0") [dryRun|enforce] [up.sh options]

Builds rad and all images from this checkout, creates or reuses the kind cluster
"${AUTHZ_CLUSTER_NAME}", and installs Radius in the given mode (default: enforce).
Other options (--skip-build, --tag) go to up.sh.

Environment:
  AUTHZ_MAPPED_NAMESPACE  Namespace mapped for the controller (default: default).
  AUTHZ_MAPPED_GROUP      Resource group it maps to
                          (default: /planes/radius/local/resourceGroups/default).
EOF
}

MODE="enforce"
ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        dryRun | enforce)
            MODE="$1"
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            ARGS+=("$1")
            ;;
    esac
    shift
done

case "$(uname -m)" in
    arm64 | aarch64) export DOCKER_BUILD_ARCH="${DOCKER_BUILD_ARCH:-arm64}" ;;
    *) export DOCKER_BUILD_ARCH="${DOCKER_BUILD_ARCH:-amd64}" ;;
esac

RAD_BIN="${AUTHZ_REPO_ROOT}/dist/authz-kit/rad"
echo "Building rad into ${RAD_BIN}"
mkdir -p "$(dirname "${RAD_BIN}")"
(cd "${AUTHZ_REPO_ROOT}" && go build -o "${RAD_BIN}" ./cmd/rad)
export RAD="${RAD:-${RAD_BIN}}"

namespace="${AUTHZ_MAPPED_NAMESPACE:-default}"
group="${AUTHZ_MAPPED_GROUP:-/planes/radius/local/resourceGroups/default}"
export AUTHZ_EXTRA_SET_VALUES="global.rbac.namespaceMappings[0].namespace=${namespace}
global.rbac.namespaceMappings[0].resourceGroup=${group}"

exec "${AUTHZ_KIT_DIR}/up.sh" "${MODE}" ${ARGS[@]+"${ARGS[@]}"}
