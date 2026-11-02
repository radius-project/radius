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
# Start a self-healing port-forward to the in-cluster Git HTTP backend.
#
# `kubectl port-forward svc/...` binds to a single pod and exits when that pod
# is deleted or the API server connection drops. The Flux functional tests use
# the forward minutes after it starts, so restart it whenever it exits instead
# of leaving a dead listener behind. kubectl output and every restart are
# appended to the log file so the artifact shows why a forward was lost.
#
# Usage: start-git-http-port-forward.sh <log-file> [namespace] [service]
#            [local-port] [remote-port]
# Environment:
#   GIT_HTTP_PORT_FORWARD_TIMEOUT  Seconds to wait for the first connection.
#                                  Default: 60
# Requires kubectl and curl.
# ============================================================================

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <log-file> [namespace] [service] [local-port]" \
        "[remote-port]" >&2
    exit 1
fi

readonly LOG_FILE="$1"
readonly NAMESPACE="${2:-git-http-backend}"
readonly SERVICE="${3:-git-http}"
readonly LOCAL_PORT="${4:-30080}"
readonly REMOTE_PORT="${5:-3000}"
readonly READY_TIMEOUT="${GIT_HTTP_PORT_FORWARD_TIMEOUT:-60}"

log() {
    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) $*"
}

# Run each child in the background and `wait` for it, so a TERM from the parent
# runs the trap right away and stops the active child instead of orphaning it.
forward_forever() {
    local child="" status
    trap 'if [[ -n "${child}" ]]; then kill "${child}" 2>/dev/null || true; fi; exit 0' TERM
    while true; do
        log "starting port-forward svc/${SERVICE}" \
            "${LOCAL_PORT}:${REMOTE_PORT} in ${NAMESPACE}"
        status=0
        kubectl port-forward -n "${NAMESPACE}" "svc/${SERVICE}" \
            "${LOCAL_PORT}:${REMOTE_PORT}" &
        child=$!
        wait "${child}" || status=$?
        log "port-forward exited with status ${status}, restarting"
        sleep 1 &
        child=$!
        wait "${child}" || true
    done
}

wait_until_ready() {
    local deadline=$((SECONDS + READY_TIMEOUT))
    until curl -s -o /dev/null --max-time 5 "http://localhost:${LOCAL_PORT}"; do
        if ((SECONDS >= deadline)); then
            echo "Port forwarding failed after ${READY_TIMEOUT}s:" >&2
            cat "${LOG_FILE}" >&2
            return 1
        fi
        sleep 1
    done
}

mkdir -p "$(dirname "${LOG_FILE}")"
forward_forever </dev/null >>"${LOG_FILE}" 2>&1 &
forward_pid=$!
if ! wait_until_ready; then
    kill "${forward_pid}" 2>/dev/null || true
    wait "${forward_pid}" || true
    exit 1
fi
echo "Port forwarding established on localhost:${LOCAL_PORT} (log: ${LOG_FILE})"
