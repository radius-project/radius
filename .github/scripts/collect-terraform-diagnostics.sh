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

# Print selected Terraform failure diagnostics, not raw logs or pod specs.
# Requires Linux, kubectl, jq, GNU timeout, and free (procps).
# Each request has a 15-second limit.
set -euo pipefail

for tool in kubectl jq timeout; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
        echo "FAILED: ${tool} is required for Terraform diagnostics." >&2
        exit 1
    fi
done

capture() {
    local description="$1"
    shift
    echo "=== ${description} ==="
    "$@" || echo "FAILED: ${description} (exit $?)"
    echo
}

pod_inventory() {
    timeout --kill-after=2s 15s kubectl get pods -n radius-system \
        --request-timeout=10s -o json | jq '[
        .items[] as $pod | $pod.spec.containers[] |
        select(.name == "applications-rp" or .name == "dynamic-rp") as $c |
        ([$pod.status.containerStatuses[]? |
            select(.name == $c.name)][0] // {}) as $s |
        {
            pod: $pod.metadata.name, uid: $pod.metadata.uid,
            node: $pod.spec.nodeName, container: $c.name,
            image: $c.image, restarts: ($s.restartCount // 0),
            memoryRequest: $c.resources.requests.memory,
            memoryLimit: $c.resources.limits.memory,
            runningSince: $s.state.running.startedAt,
            waitingReason: $s.state.waiting.reason,
            terminated: ($s.state.terminated | if . == null then null else
                {reason, exitCode, signal, startedAt, finishedAt} end),
            previousTermination: ($s.lastState.terminated |
                if . == null then null else
                {reason, exitCode, signal, startedAt, finishedAt} end)
        }
    ]'
}

node_capacity() {
    timeout --kill-after=2s 15s kubectl get nodes \
        --request-timeout=10s -o json | jq '[
        .items[] | {
            node: .metadata.name,
            capacity: .status.capacity,
            allocatable: .status.allocatable,
            conditions: [.status.conditions[]? |
                {type, status, reason, lastTransitionTime}]
        }
    ]'
}

event_summary() {
    timeout --kill-after=2s 15s kubectl get events -A \
        --request-timeout=10s -o json | jq '[
        .items[] |
        select(.type == "Warning" or
            ((.reason // "") | test("OOM|Evict|Pressure|BackOff|Kill"; "i"))) |
        {
            time: (.eventTime // .lastTimestamp // .metadata.creationTimestamp),
            reason, count, type,
            object: (.involvedObject | {kind, namespace, name})
        }
    ]'
}

provider_summary() {
    timeout --kill-after=2s 15s kubectl logs "$1" -n radius-system -c "$2" \
        --request-timeout=10s --since=40m --tail=2000 --limit-bytes=1048576 \
        "${@:3}" | jq -Rn '
        reduce inputs as $line ({
            schemaErrorRecords: 0, startupTimeoutRecords: 0,
            pluginNoResponseRecords: 0, nonJSONRecords: 0
        };
            (try ($line | fromjson) catch null) as $entry |
            if ($entry | type) != "object" then .nonJSONRecords += 1
            else
                (($entry.message // "") + "\n" +
                    ($entry.error // "")) as $message |
                .schemaErrorRecords +=
                    (if $message | test(
                        "GetProviderSchema|Failed to load plugin schemas")
                     then 1 else 0 end) |
                .startupTimeoutRecords +=
                    (if $message | contains("timeout while waiting for plugin")
                     then 1 else 0 end) |
                .pluginNoResponseRecords +=
                    (if $message | contains("Plugin did not")
                     then 1 else 0 end)
            end
        )'
}

echo "Terraform diagnostics at $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "Post-test snapshot; counters and peaks cover the container lifetime."
echo "Provider summaries count log records, not failed operations."
echo "Log window: last 40 minutes, at most 2000 lines and 1 MiB per capture."

capture "Runner memory (MiB)" timeout --kill-after=2s 15s free -m
capture "Node capacity and pressure" node_capacity
capture "Warning and resource-pressure events (without messages)" event_summary

echo "=== Terraform container state ==="
if inventory="$(pod_inventory)"; then
    echo "${inventory}"
else
    echo "FAILED: Terraform pod inventory (exit $?)"
    exit 0
fi
if [[ "$(jq length <<<"${inventory}")" == 0 ]]; then
    echo "NO_CONTAINERS: no Terraform resource provider containers found."
fi

while IFS=$'\t' read -r pod container restarts; do
    # shellcheck disable=SC2016 # Expand variables in the remote container.
    capture "${pod}/${container} cgroup memory and CPU" \
        timeout --kill-after=2s 15s kubectl exec "${pod}" \
        -n radius-system -c "${container}" --request-timeout=10s -- sh -c '
            found=0
            for file in \
                /sys/fs/cgroup/memory.current \
                /sys/fs/cgroup/memory.peak \
                /sys/fs/cgroup/memory.max \
                /sys/fs/cgroup/memory.events \
                /sys/fs/cgroup/cpu.stat \
                /sys/fs/cgroup/pids.current \
                /sys/fs/cgroup/memory/memory.usage_in_bytes \
                /sys/fs/cgroup/memory/memory.max_usage_in_bytes \
                /sys/fs/cgroup/memory/memory.limit_in_bytes \
                /sys/fs/cgroup/memory/memory.oom_control; do
                if [ -r "$file" ]; then
                    found=1
                    printf "=== %s ===\n" "$file"
                    cat "$file" || exit 1
                fi
            done
            if [ "$found" = 0 ]; then
                echo "FAILED: no readable cgroup counters." >&2
                exit 1
            fi'
    capture "${pod}/${container} current provider error summary" \
        provider_summary "${pod}" "${container}"
    if ((restarts > 0)); then
        capture "${pod}/${container} previous provider error summary" \
            provider_summary "${pod}" "${container}" --previous
    fi
done < <(jq -r '.[] | [.pod, .container, .restarts] | @tsv' <<<"${inventory}")
