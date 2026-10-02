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
#   DIAGNOSTICS_LOG_NAMESPACES Space-separated namespaces for container logs.
#                              Empty by default (no additional collection).
#   DIAGNOSTICS_SINCE_TIME     UTC RFC3339 start time; otherwise read the window
#                              saved by run-with-cluster-diagnostics.sh.
#   DIAGNOSTICS_COLLECTION_SECONDS Container log sweep budget (default: 120).
# Requires kubectl; optional container collection also requires jq and timeout
# (GNU coreutils). It has a 120-second budget and a 100 MiB log budget.
# ============================================================================

set -euo pipefail

readonly OUTPUT_DIR="${RADIUS_CONTAINER_LOG_PATH:-./dist/container_logs}"
readonly DIAGNOSTICS_NAME="${DIAGNOSTICS_NAME:-cluster}"
readonly KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-}"

readonly STATE_LOG="${OUTPUT_DIR}/${DIAGNOSTICS_NAME}-tests-pod-states.log"
readonly NODE_LOG_DIR="${OUTPUT_DIR}/${DIAGNOSTICS_NAME}-kind-logs"
readonly DIAGNOSTICS_DIR="${OUTPUT_DIR}/${DIAGNOSTICS_NAME}-diagnostics"
readonly LOG_NAMESPACES="${DIAGNOSTICS_LOG_NAMESPACES:-}"

# Append a labelled section for a command, recording failures inline instead of
# aborting so later sections are still collected.
capture() {
    local description="$1"
    shift

    {
        echo "=== ${description} ==="
        if [[ -n "${LOG_NAMESPACES}" ]] &&
            command -v timeout >/dev/null 2>&1; then
            timeout --kill-after=2s 15s "$@" 2>&1 \
                || echo "FAILED: ${description} (exit $?)"
        else
            "$@" 2>&1 || echo "FAILED: ${description} (exit $?)"
        fi
        echo
    } >>"${STATE_LOG}"
}

collect_container_logs() {
    local tool since_time namespace inventory pod uid container restarts
    local remaining limit size result
    local file previous
    local collection_seconds="${DIAGNOSTICS_COLLECTION_SECONDS:-120}"
    local deadline
    local budget=$((100 * 1024 * 1024))
    local per_file_limit=$((10 * 1024 * 1024))
    local -a namespaces log_args

    mkdir -p "${DIAGNOSTICS_DIR}"
    local manifest="${DIAGNOSTICS_DIR}/collection.log"
    echo "Collection started: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"${manifest}"
    if [[ ! "${collection_seconds}" =~ ^[1-9][0-9]{0,3}$ ]]; then
        echo "FAILED: DIAGNOSTICS_COLLECTION_SECONDS must be 1-9999." \
            | tee -a "${manifest}" >&2
        return
    fi
    deadline=$((SECONDS + collection_seconds))
    for tool in jq timeout; do
        if ! command -v "${tool}" >/dev/null 2>&1; then
            echo "FAILED: ${tool} is required for container logs." \
                | tee -a "${manifest}" >&2
            return
        fi
    done

    since_time="${DIAGNOSTICS_SINCE_TIME:-}"
    if [[ -z "${since_time}" && -f "${DIAGNOSTICS_DIR}/window-start.txt" ]]; then
        since_time="$(<"${DIAGNOSTICS_DIR}/window-start.txt")"
    fi
    if [[ -z "${since_time}" ]]; then
        echo "SKIPPED: no test window recorded; tests may not have started." \
            >>"${manifest}"
        return
    fi
    if [[ ! "${since_time}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]; then
        echo "FAILED: invalid DIAGNOSTICS_SINCE_TIME: ${since_time}" \
            | tee -a "${manifest}" >&2
        return
    fi
    {
        echo "Requested log window: ${since_time} through collection time"
        echo "Limits: ${per_file_limit} bytes/file; ${budget} bytes total; ${collection_seconds}s"
        echo "Only retained logs of surviving pods are available."
        echo "Previous logs cover at most one restart per container."
        echo "Missing log timestamps do not establish rotation or an outage."
        echo "Inventory columns: pod uid container restarts image currentContainerID currentStarted currentFinished previousContainerID previousStarted previousFinished"
    } >>"${manifest}"
    read -r -a namespaces <<<"${LOG_NAMESPACES}"
    for namespace in "${namespaces[@]}"; do
        if [[ ! "${namespace}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
            echo "FAILED: invalid log namespace: ${namespace}" \
                | tee -a "${manifest}" >&2
            continue
        fi
        remaining=$((deadline - SECONDS))
        if ((remaining <= 0)); then
            echo "LIMIT_REACHED: collection time budget exhausted." >>"${manifest}"
            break
        fi
        ((remaining > 15)) && remaining=15
        mkdir -p "${DIAGNOSTICS_DIR}/${namespace}"
        inventory="${DIAGNOSTICS_DIR}/${namespace}/containers.tsv"
        # Persist identities and states, not pod specs containing environment
        # values. Include init containers, including native sidecars.
        if ! timeout --kill-after=2s "${remaining}s" \
            kubectl get pods -n "${namespace}" --request-timeout=10s -o json \
            2>>"${manifest}" | jq -r '
                .items[] as $pod |
                ($pod.spec.containers + ($pod.spec.initContainers // []))[] as $c |
                ([$pod.status.containerStatuses[]?,
                  $pod.status.initContainerStatuses[]?] |
                    map(select(.name == $c.name)) | .[0] // {}) as $s |
                [$pod.metadata.name, $pod.metadata.uid, $c.name,
                 ($s.restartCount // 0), $c.image, ($s.containerID // "-"),
                 ($s.state.running.startedAt //
                  $s.state.terminated.startedAt // "-"),
                 ($s.state.terminated.finishedAt // "-"),
                 ($s.lastState.terminated.containerID // "-"),
                 ($s.lastState.terminated.startedAt // "-"),
                 ($s.lastState.terminated.finishedAt // "-")] | @tsv
            ' >"${inventory}" 2>>"${manifest}"; then
            echo "FAILED: inventory for ${namespace}; see errors above." \
                | tee -a "${manifest}" >&2
            continue
        fi
        if [[ ! -s "${inventory}" ]]; then
            echo "NO_CONTAINERS: ${namespace} returned no containers." \
                >>"${manifest}"
        fi

        while IFS=$'\t' read -r pod uid container restarts _; do
            for previous in current previous; do
                if [[ "${previous}" == previous && "${restarts}" == 0 ]]; then
                    continue
                fi
                remaining=$((deadline - SECONDS))
                if ((remaining <= 0 || budget <= 0)); then
                    echo "LIMIT_REACHED: time or total log budget exhausted." \
                        >>"${manifest}"
                    return
                fi
                ((remaining > 15)) && remaining=15
                limit="${per_file_limit}"
                ((limit > budget)) && limit="${budget}"
                file="${DIAGNOSTICS_DIR}/${namespace}/${pod}.${container}.${previous}.log"
                log_args=(logs "${pod}" -n "${namespace}" -c "${container}"
                    --request-timeout=10s --timestamps
                    "--since-time=${since_time}" "--limit-bytes=${limit}")
                [[ "${previous}" == previous ]] && log_args+=(--previous)
                echo "START $(date -u +%Y-%m-%dT%H:%M:%SZ) ${namespace}/${pod} uid=${uid} container=${container} ${previous}" \
                    >>"${manifest}"
                result=0
                # head enforces the byte cap even if kubectl returns a partial
                # final line beyond --limit-bytes. Keep errors with the capture.
                timeout --kill-after=2s "${remaining}s" kubectl \
                    "${log_args[@]}" 2>&1 | head -c "${limit}" >"${file}" \
                    || result=$?
                size="$(wc -c <"${file}")"
                budget=$((budget - size))
                echo "END $(date -u +%Y-%m-%dT%H:%M:%SZ) exit=${result} bytes=${size} file=${file}" \
                    >>"${manifest}"
                if ((result != 0)); then
                    echo "FAILED: ${file} (exit ${result}); capture may be partial." \
                        | tee -a "${manifest}" >&2
                fi
                if ((size >= limit)); then
                    echo "LIMIT_REACHED: ${file}; capture may be incomplete." \
                        >>"${manifest}"
                fi
            done
        done <"${inventory}"
    done
    echo "Collection finished: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >>"${manifest}"
}

mkdir -p "${OUTPUT_DIR}"

# Collect logs before the potentially slow cluster-wide descriptions. Legacy
# callers retain their existing snapshot behavior unless namespaces are set.
if [[ -n "${LOG_NAMESPACES}" ]]; then
    collect_container_logs
fi

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
