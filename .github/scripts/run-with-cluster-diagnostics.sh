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

# Run a command with periodic Kubernetes event snapshots. Requires kubectl and
# timeout (GNU coreutils). Sampling failures never replace the command's exit
# status. The final collector reads window-start.txt from this same directory.
#
# RADIUS_CONTAINER_LOG_PATH / DIAGNOSTICS_NAME match the final collector.
# DIAGNOSTICS_SAMPLE_INTERVAL is the delay between samples (default: 60s).
# Samples stop after 130 minutes or 20 MiB, even if the parent is lost.

set -euo pipefail

readonly OUTPUT_DIR="${RADIUS_CONTAINER_LOG_PATH:-./dist/container_logs}"
readonly DIAGNOSTICS_NAME="${DIAGNOSTICS_NAME:-cluster}"
readonly DIAGNOSTICS_DIR="${OUTPUT_DIR}/${DIAGNOSTICS_NAME}-diagnostics"
readonly SAMPLE_INTERVAL="${DIAGNOSTICS_SAMPLE_INTERVAL:-60}"
sampler_pid=""
command_pid=""

sample_events() {
    local sleep_pid=""
    # shellcheck disable=SC2329 # Invoked by the EXIT trap.
    stop_sleep() {
        if [[ -n "${sleep_pid}" ]]; then
            if kill -0 "${sleep_pid}" 2>/dev/null; then
                kill "${sleep_pid}" || echo "FAILED: stopping sampler sleep." >&2
            fi
            wait "${sleep_pid}" || :
        fi
    }
    trap stop_sleep EXIT
    trap 'exit 0' TERM INT

    local deadline=$((SECONDS + 130 * 60))
    local remaining=$((20 * 1024 * 1024))
    local file size result sample=0
    while ((SECONDS < deadline && remaining > 0)); do
        sample=$((sample + 1))
        file="${DIAGNOSTICS_DIR}/events-${sample}.log"
        result=0
        {
            echo "Sample started: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
            # JSON retains absolute event timestamps and counts, unlike the
            # human-readable AGE column in the post-test snapshot.
            timeout --kill-after=2s 15s kubectl get events -A \
                --request-timeout=10s -o json \
                || echo "FAILED: get events (exit $?)"
            timeout --kill-after=2s 15s kubectl get pods -n radius-system \
                --request-timeout=10s -o wide \
                || echo "FAILED: get radius-system pods (exit $?)"
            echo "Sample finished: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        } 2>&1 | head -c "${remaining}" >"${file}" || result=$?
        size="$(wc -c <"${file}")"
        remaining=$((remaining - size))
        echo "Sample ${sample}: exit=${result} bytes=${size}" \
            >>"${DIAGNOSTICS_DIR}/sampler.log"
        if ((result != 0)); then
            echo "FAILED: sample ${sample}; see ${file}." \
                >>"${DIAGNOSTICS_DIR}/sampler.log"
        fi
        if ((remaining <= 0)); then
            echo "LIMIT_REACHED: 20 MiB event budget; final sample may be partial." \
                >>"${DIAGNOSTICS_DIR}/sampler.log"
            exit 0
        fi
        sleep "${SAMPLE_INTERVAL}" &
        sleep_pid=$!
        wait "${sleep_pid}"
        sleep_pid=""
    done
    echo "LIMIT_REACHED: event sampling duration." \
        >>"${DIAGNOSTICS_DIR}/sampler.log"
}

sampling_failure() {
    echo "FAILED: $*" >&2
    echo "FAILED: $*" >>"${DIAGNOSTICS_DIR}/sampler.log" \
        || echo "FAILED: could not record sampling failure." >&2
}

stop_sampler() {
    local result=$?
    trap - EXIT
    if [[ -n "${command_pid}" ]] && kill -0 "${command_pid}" 2>/dev/null; then
        # timeout owns the command's process group and forwards termination to
        # its descendants as well, including make and the test processes.
        kill "${command_pid}" || echo "FAILED: stopping wrapped command." >&2
        wait "${command_pid}" || :
    fi
    if [[ -n "${sampler_pid}" ]]; then
        if kill -0 "${sampler_pid}" 2>/dev/null; then
            kill "${sampler_pid}" || echo "FAILED: stopping event sampler." >&2
        fi
        if wait "${sampler_pid}"; then
            echo "Sampler stopped: $(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                >>"${DIAGNOSTICS_DIR}/sampler.log" \
                || echo "FAILED: writing sampler completion." >&2
        else
            echo "FAILED: event sampler exited unexpectedly." >&2
        fi
    fi
    echo "Command finished: $(date -u +%Y-%m-%dT%H:%M:%SZ) exit=${result}" \
        >>"${DIAGNOSTICS_DIR}/sampler.log" \
        || echo "FAILED: writing command completion." >&2
    exit "${result}"
}

if (($# == 0)); then
    echo "Usage: $0 command [args...]" >&2
    exit 2
fi

if ! mkdir -p "${DIAGNOSTICS_DIR}" ||
    ! date -u +%Y-%m-%dT%H:%M:%SZ >"${DIAGNOSTICS_DIR}/window-start.txt"; then
    echo "FAILED: cannot initialize diagnostics; running command without sampling." >&2
    exec "$@"
fi

for tool in kubectl timeout; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        sampling_failure "${tool} is required; running command without sampling."
        exec "$@"
    fi
done
if [[ ! "${SAMPLE_INTERVAL}" =~ ^[1-9][0-9]{0,3}$ ]]; then
    sampling_failure "DIAGNOSTICS_SAMPLE_INTERVAL must be 1-9999 seconds."
    exec "$@"
fi

if ! echo "Sampling initialized: $(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    >>"${DIAGNOSTICS_DIR}/sampler.log"; then
    echo "FAILED: cannot write sampler status; running without sampling." >&2
    exec "$@"
fi
trap stop_sampler EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
sample_events &
sampler_pid=$!
echo "Sampler started: pid=${sampler_pid} interval=${SAMPLE_INTERVAL}s" \
    >>"${DIAGNOSTICS_DIR}/sampler.log" \
    || echo "FAILED: writing sampler startup." >&2
# This safety ceiling exceeds LRT's existing 120-minute test-step timeout.
# Background wait lets signal traps run immediately rather than after tests.
timeout --kill-after=2s 130m "$@" &
command_pid=$!
wait "${command_pid}"
