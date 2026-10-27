#!/bin/bash
# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

# Run a command while collecting every observed Radius container incarnation.
# Requires Bash, jq, and kubectl. AUTHZ_LOG_DIR must be a new directory.

set -euo pipefail

# Give each background job its own process group, including on macOS where
# setsid is not installed. Workers disable monitor mode for their descendants.
set -m

OUTPUT="${AUTHZ_LOG_DIR-dist/authz-logs}"
NAMESPACE="${AUTHZ_NAMESPACE-radius-system}"
WORK_DIR=""
WATCH_PID=""
EVENT_BUFFER=""
PIDS=()
KEYS=()
PODS=()
CONTAINERS=()
STREAM_PIDS=()
ACTIVE=()
RUNNING=()

die() {
    echo "authz log collection failed: $*" >&2
    exit 2
}

spawn() {
    "$@" &
    SPAWN_PID=$!
    PIDS+=("${SPAWN_PID}")
}

finished() {
    ! kill -0 "$1" 2>/dev/null
}

exit_status() {
    JOB_STATUS=0
    wait "$1" || JOB_STATUS=$?
}

run_bounded() {
    local pid deadline status
    spawn "$@"
    pid="${SPAWN_PID}"
    deadline=$((SECONDS + 30))
    while ! finished "${pid}"; do
        [[ "${SECONDS}" -lt "${deadline}" ]] ||
            die "kubectl timed out after 30 seconds"
        sleep .1
    done
    exit_status "${pid}"
    status="${JOB_STATUS}"
    stop "${pid}"
    unset "PIDS[$((${#PIDS[@]} - 1))]"
    [[ "${status}" -eq 0 ]] || die "kubectl exited with status ${status}"
}

stop() {
    local pid="$1" attempt
    if kill -0 -- "-${pid}" 2>/dev/null; then
        kill -TERM -- "-${pid}" ||
            ! kill -0 -- "-${pid}" 2>/dev/null
        for ((attempt = 0; attempt < 30; attempt++)); do
            kill -0 -- "-${pid}" 2>/dev/null || break
            sleep .1
        done
        if kill -0 -- "-${pid}" 2>/dev/null; then
            kill -KILL -- "-${pid}" ||
                ! kill -0 -- "-${pid}" 2>/dev/null
        fi
    fi
    # Signals sent by this script are expected during cleanup.
    wait "${pid}" || :
}

# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
    local status=$? i
    trap - EXIT INT TERM
    for ((i = ${#PIDS[@]} - 1; i >= 0; i--)); do
        stop "${PIDS[i]}"
    done
    if [[ -n "${WORK_DIR}" ]]; then
        rm -rf "${WORK_DIR}"
    fi
    exit "${status}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'die "command failed at line ${LINENO}"' ERR
set -E

inventory() {
    run_bounded kubectl get pods --namespace "${NAMESPACE}" \
        --request-timeout=20s -o json >"${WORK_DIR}/inventory.json"
    jq -e '.items | type == "array"' \
        "${WORK_DIR}/inventory.json" >/dev/null
}

snapshot() {
    local path="$1"
    shift
    run_bounded kubectl "$@" --request-timeout=20s >>"${path}"
}

observe() {
    local pod="$1" deleted="${2:-false}"
    local name uid rows container restart state key path i index
    name="$(jq -er '.metadata.name |
        select(test("^[a-z0-9][a-z0-9.-]*$"))' <<<"${pod}")"
    uid="$(jq -er '.metadata.uid |
        select(test("^[a-zA-Z0-9-]+$"))' <<<"${pod}")"
    for i in "${!KEYS[@]}"; do
        if [[ "${KEYS[i]}" == "${uid}."* ]]; then
            ACTIVE[i]=false
        fi
    done
    rows="$(jq -r '
        [.status.containerStatuses // [],
         .status.initContainerStatuses // [],
         .status.ephemeralContainerStatuses // []] | add | .[] |
        if (.name | type == "string" and
                test("^[a-z0-9][a-z0-9.-]*$")) and
            (.restartCount | type == "number" and . >= 0 and
                . == floor)
        then [.name, .restartCount,
            (if .state.running then "running"
             elif .state.terminated then "terminated"
             else "waiting" end)] | @tsv
        else error("invalid container status") end' <<<"${pod}")"
    while IFS=$'\t' read -r container restart state; do
        [[ -n "${container}" ]] || continue
        key="${uid}.${container}.${restart}"
        path="${OUTPUT}/${name}.${key}.log"
        local args=(logs "${name}" --namespace "${NAMESPACE}"
            --container "${container}" --timestamps=true --tail=-1)
        if [[ "${restart}" -gt 0 &&
            ! -f "${WORK_DIR}/previous.${key}" ]]; then
            snapshot "${path%.log}.previous.log" \
                "${args[@]}" --previous
            touch "${WORK_DIR}/previous.${key}"
        fi
        [[ "${state}" != waiting ]] || continue
        index="${#KEYS[@]}"
        for i in "${!KEYS[@]}"; do
            if [[ "${KEYS[i]}" == "${key}" ]]; then
                index="${i}"
                break
            fi
        done
        if [[ "${index}" -eq "${#KEYS[@]}" ]]; then
            snapshot "${path}" "${args[@]}"
            spawn kubectl "${args[@]}" --follow >>"${path}"
            KEYS+=("${key}")
            PODS+=("${name}")
            CONTAINERS+=("${container}")
            STREAM_PIDS+=("${SPAWN_PID}")
        fi
        ACTIVE[index]=true
        [[ "${deleted}" == false ]] || ACTIVE[index]=false
        RUNNING[index]=false
        [[ "${state}" != running ]] || RUNNING[index]=true
    done <<<"${rows}"
}

# shellcheck disable=SC2329 # Invoked as a background job by spawn.
watch_pods() {
    set +m
    trap - ERR EXIT INT TERM
    local source parser status=0 source_status=0
    kubectl get --raw "$1" --request-timeout=0 \
        >"${WORK_DIR}/watch-pipe" &
    source=$!
    jq --unbuffered -c . <"${WORK_DIR}/watch-pipe" &
    parser=$!
    # Stop only the source first. jq must consume EOF and reject truncated
    # JSON before final collection can be marked complete.
    trap '
        if finished "${source}"; then
            echo "pod watch source ended before final collection" >&2
            status=2
        else
            kill -TERM "${source}" || ! kill -0 "${source}" 2>/dev/null
        fi
        wait "${source}" || :
        wait "${parser}" || status=$?
        exit "${status}"
    ' TERM
    wait "${parser}" || status=$?
    if ! finished "${source}"; then
        kill -TERM "${source}" || ! kill -0 "${source}" 2>/dev/null
    fi
    wait "${source}" || source_status=$?
    [[ "${status}" -ne 0 ]] || status="${source_status}"
    return "${status}"
}

finish_watch() {
    local attempt
    check_watch
    kill -TERM "${WATCH_PID}"
    for ((attempt = 0; attempt < 50; attempt++)); do
        finished "${WATCH_PID}" && break
        sleep .1
    done
    finished "${WATCH_PID}" || die "pod watch reader did not stop"
    exit_status "${WATCH_PID}"
    [[ "${JOB_STATUS}" -eq 0 ]] ||
        die "pod watch failed during final collection: ${JOB_STATUS}"
}

drain_events() {
    local line event kind pod
    while true; do
        line=""
        if IFS= read -r line <&3; then
            event="${EVENT_BUFFER}${line}"
            EVENT_BUFFER=""
            [[ "${#event}" -le 16777216 ]] ||
                die "pod watch object exceeded 16 MiB"
            kind="$(jq -er '.type' <<<"${event}")"
            case "${kind}" in
                ADDED | MODIFIED | DELETED)
                    pod="$(jq -ce '.object' <<<"${event}")"
                    if [[ "${kind}" == DELETED ]]; then
                        observe "${pod}" true
                    else
                        observe "${pod}"
                    fi
                    ;;
                *) die "unexpected pod watch event: ${event}" ;;
            esac
        else
            # A regular-file reader can reach EOF midway through a jq write.
            EVENT_BUFFER+="${line}"
            [[ "${#EVENT_BUFFER}" -le 16777216 ]] ||
                die "pod watch object exceeded 16 MiB"
            break
        fi
    done
}

check_watch() {
    if finished "${WATCH_PID}"; then
        exit_status "${WATCH_PID}"
        die "pod watch ended before the command: ${JOB_STATUS}"
    fi
}

check_streams() {
    local i key uid restart current path
    for i in "${!KEYS[@]}"; do
        finished "${STREAM_PIDS[i]}" || continue
        exit_status "${STREAM_PIDS[i]}"
        [[ "${JOB_STATUS}" -eq 0 ]] ||
            die "log stream failed for ${PODS[i]}/${CONTAINERS[i]}: ${JOB_STATUS}"
        if [[ "${ACTIVE[i]}" != true || "${RUNNING[i]}" != true ]]; then
            continue
        fi
        run_bounded kubectl get pod "${PODS[i]}" --namespace "${NAMESPACE}" \
            --ignore-not-found --request-timeout=20s -o json \
            >"${WORK_DIR}/current.json"
        key="${KEYS[i]}"
        uid="${key%%.*}"
        restart="${key##*.}"
        # Empty output is only accepted for an explicit NotFound. A failed
        # lookup above is fatal, not evidence that the container disappeared.
        current="$(jq -r --arg uid "${uid}" \
            --arg container "${CONTAINERS[i]}" --argjson restart "${restart}" '
            select(.metadata.uid == $uid) |
            [.status.containerStatuses // [],
             .status.initContainerStatuses // [],
             .status.ephemeralContainerStatuses // []] | add | .[] |
            select(.name == $container and .restartCount == $restart) |
            if .state.running then "running" else "stopped" end
            ' "${WORK_DIR}/current.json")"
        ACTIVE[i]=false
        RUNNING[i]=false
        [[ -n "${current}" ]] || continue
        ACTIVE[i]=true
        [[ "${current}" == running ]] || continue
        RUNNING[i]=true
        # Re-read without a timestamp cutoff: duplicate lines are harmless,
        # whereas equal-timestamp lines could otherwise be lost.
        path="${OUTPUT}/${PODS[i]}.${key}.log"
        spawn kubectl logs "${PODS[i]}" --namespace "${NAMESPACE}" \
            --container "${CONTAINERS[i]}" --timestamps=true --tail=-1 \
            --follow >>"${path}"
        STREAM_PIDS[i]="${SPAWN_PID}"
    done
}

finish() {
    local pod i uid attempt
    drain_events
    check_watch
    inventory
    jq -c '.items[]' "${WORK_DIR}/inventory.json" \
        >"${WORK_DIR}/final-pods"
    while IFS= read -r pod; do
        observe "${pod}"
        uid="$(jq -er '.metadata.uid' <<<"${pod}")"
        for i in "${!KEYS[@]}"; do
            if [[ "${KEYS[i]}" == "${uid}."* &&
                "${ACTIVE[i]}" == true ]]; then
                snapshot "${OUTPUT}/${PODS[i]}.${KEYS[i]}.log" \
                    logs "${PODS[i]}" --namespace "${NAMESPACE}" \
                    --container "${CONTAINERS[i]}" \
                    --timestamps=true --tail=-1
            fi
        done
    done <"${WORK_DIR}/final-pods"
    finish_watch
    drain_events
    [[ -z "${EVENT_BUFFER}" ]] || die "incomplete JSON from pod watch"
    check_streams
    for i in "${!KEYS[@]}"; do
        [[ "${ACTIVE[i]}" == false ]] || continue
        for ((attempt = 0; attempt < 50; attempt++)); do
            finished "${STREAM_PIDS[i]}" && break
            sleep .1
        done
        finished "${STREAM_PIDS[i]}" ||
            die "retired log stream did not stop: ${PODS[i]}/${CONTAINERS[i]}"
        exit_status "${STREAM_PIDS[i]}"
        [[ "${JOB_STATUS}" -eq 0 ]] ||
            die "retired log stream failed: ${PODS[i]}/${CONTAINERS[i]}"
    done
}

[[ $# -gt 0 ]] || die "Usage: $0 command [args...]"
for tool in jq kubectl; do
    command -v "${tool}" >/dev/null || die "${tool} is required"
done
[[ -n "${NAMESPACE}" ]] || die "AUTHZ_NAMESPACE must not be empty"
[[ -n "${OUTPUT}" ]] || die "AUTHZ_LOG_DIR must not be empty"
while [[ "${OUTPUT}" == */ && "${OUTPUT}" != / ]]; do
    OUTPUT="${OUTPUT%/}"
done
mkdir -p "$(dirname "${OUTPUT}")"
mkdir "${OUTPUT}"
WORK_DIR="$(mktemp -d)"
inventory
jq -e '.items | length > 0' "${WORK_DIR}/inventory.json" >/dev/null ||
    die "no pods in ${NAMESPACE}"
VERSION="$(jq -er '.metadata.resourceVersion |
    select(type == "string" and length > 0) | @uri' \
    "${WORK_DIR}/inventory.json")"
NAMESPACE_URI="$(jq -rn --arg namespace "${NAMESPACE}" \
    '$namespace | @uri')"
WATCH_URL="/api/v1/namespaces/${NAMESPACE_URI}/pods"
WATCH_URL+="?watch=true&resourceVersion=${VERSION}&timeoutSeconds=5400"
touch "${WORK_DIR}/events"
mkfifo "${WORK_DIR}/watch-pipe"
exec 3<"${WORK_DIR}/events"
spawn watch_pods "${WATCH_URL}" >"${WORK_DIR}/events"
WATCH_PID="${SPAWN_PID}"
jq -c '.items[]' "${WORK_DIR}/inventory.json" >"${WORK_DIR}/initial-pods"
while IFS= read -r POD; do
    observe "${POD}"
done <"${WORK_DIR}/initial-pods"
drain_events
check_watch
check_streams
spawn "$@"
COMMAND_PID="${SPAWN_PID}"
while ! finished "${COMMAND_PID}"; do
    drain_events
    check_watch
    check_streams
    sleep .1
done
exit_status "${COMMAND_PID}"
COMMAND_STATUS="${JOB_STATUS}"
finish
for ((INDEX = ${#PIDS[@]} - 1; INDEX >= 0; INDEX--)); do
    stop "${PIDS[INDEX]}"
done
PIDS=()
touch "${OUTPUT}/complete"
exit "${COMMAND_STATUS}"
