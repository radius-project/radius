#!/bin/bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
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
# Collect Kubernetes Cluster Diagnostics
#
# Captures a post-mortem snapshot of a functional test cluster: pod, node and
# event state, plus the KinD node logs (kube-apiserver, etcd, kubelet,
# containerd) when a cluster name is supplied.
#
# Collection is best-effort by design. Functional test failures are frequently
# caused by an unhealthy control plane, which is exactly when every kubectl call
# fails; each command therefore records its own failure and collection
# continues, so one unreachable API server cannot discard the whole snapshot.
#
# Configuration (environment variables):
#   RADIUS_CONTAINER_LOG_PATH  Output directory. Default: ./dist/container_logs
#   DIAGNOSTICS_NAME           Prefix for output names. Default: cluster
#   KIND_CLUSTER_NAME          KinD cluster to export node logs from. Node logs
#                              are skipped when empty.
# ============================================================================

set -euo pipefail

readonly OUTPUT_DIR="${RADIUS_CONTAINER_LOG_PATH:-./dist/container_logs}"
readonly DIAGNOSTICS_NAME="${DIAGNOSTICS_NAME:-cluster}"
readonly KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-}"

readonly STATE_LOG="${OUTPUT_DIR}/${DIAGNOSTICS_NAME}-tests-pod-states.log"
readonly NODE_LOG_DIR="${OUTPUT_DIR}/${DIAGNOSTICS_NAME}-kind-logs"

# Append a labelled section for a command, recording failures inline instead of
# aborting so later sections are still collected.
capture() {
    local description="$1"
    shift

    {
        echo "=== ${description} ==="
        "$@" 2>&1 || echo "FAILED: ${description} (exit $?)"
        echo
    } >>"${STATE_LOG}"
}

mkdir -p "${OUTPUT_DIR}"

capture "kubectl get pods -A" kubectl get pods -A -o wide
capture "kubectl describe pods -A" kubectl describe pods -A
capture "kubectl get nodes" kubectl get nodes -o wide
# Surfaces MemoryPressure/DiskPressure and allocatable-vs-requested capacity,
# which is what distinguishes a resource-exhausted runner from an app bug.
capture "kubectl describe nodes" kubectl describe nodes
capture "kubectl get events -A" kubectl get events -A --sort-by=.metadata.creationTimestamp

echo "Cluster state written to ${STATE_LOG}"

if [[ -z "${KIND_CLUSTER_NAME}" ]]; then
    exit 0
fi

if ! command -v kind >/dev/null 2>&1; then
    echo "Warning: kind is not installed; skipping node log export." >&2
    exit 0
fi

mkdir -p "${NODE_LOG_DIR}"
if kind export logs "${NODE_LOG_DIR}" --name "${KIND_CLUSTER_NAME}"; then
    echo "Node logs written to ${NODE_LOG_DIR}"
else
    echo "Warning: failed to export logs for KinD cluster ${KIND_CLUSTER_NAME}." >&2
fi
