#!/bin/bash

# ============================================================================
# Collect Kubernetes cluster diagnostics for functional test post-mortems.
#
# Functional tests intermittently fail with `connection reset by peer` or `EOF`
# against the Radius API. Diagnosing those runs requires distinguishing a
# control-plane outage (etcd slowness, a kube-apiserver liveness kill) from a
# Radius bug, and the pod snapshot alone cannot do that. This script captures
# the missing evidence:
#
#   * pod state as YAML, which preserves `lastState.terminated.reason` and
#     `exitCode` for containers that already restarted (`kubectl describe` has
#     been observed to omit the `Last State` block entirely)
#   * cluster-wide events, which record liveness/readiness probe failures and
#     the resulting kills
#   * node conditions, which record disk/memory/PID pressure
#   * the current and previous logs of every kube-system container, including
#     etcd - the previous instance's log is where an apiserver crash reason and
#     etcd's `apply request took too long` / `slow fdatasync` warnings live
#   * verbose /livez and /readyz output, which names the specific failing
#     apiserver health check
#
# Collection is best effort by design: it runs after a test job that may have
# already started tearing the cluster down, so an individual kubectl failure is
# recorded in the output and never aborts the rest of the collection.
# ============================================================================

set -euo pipefail

readonly SCRIPT_NAME="$(basename "$0")"

OUTPUT_DIR=""
PREFIX="cluster"

usage() {
    echo "Usage: ${SCRIPT_NAME} --output-dir DIR [--prefix NAME]"
    echo "Options:"
    echo "  -o, --output-dir   Directory to write diagnostics into (required)"
    echo "  -p, --prefix       Filename prefix for the output files"
    echo "                     (default: cluster)"
    echo "  -h, --help         Show this help"
    exit 0
}

validate_requirements() {
    if [[ -z "${OUTPUT_DIR}" ]]; then
        echo "Error: --output-dir is required" >&2
        exit 1
    fi

    if ! command -v kubectl >/dev/null; then
        echo "Error: kubectl is required but not installed" >&2
        exit 1
    fi
}

# capture appends the command line and its combined output to a file. A failing
# command is recorded rather than propagated so one unavailable resource does
# not stop the remaining collection.
capture() {
    local output_file="$1"
    shift

    {
        echo "=============================================================="
        echo "\$ $*"
        echo "=============================================================="
        "$@" 2>&1 || echo "(command exited with code $?)"
        echo
    } >>"${output_file}"
}

collect_pod_state() {
    # The plain-text snapshot keeps the historical pod-states file intact, and
    # the YAML dump adds the machine-readable lastState/exitCode fields.
    capture "${OUTPUT_DIR}/${PREFIX}-pod-states.log" kubectl get pods -A -o wide
    capture "${OUTPUT_DIR}/${PREFIX}-pod-states.log" kubectl describe pods -A

    # Written raw, with no headers, so the file stays parseable by a YAML
    # reader. stderr goes to the pod-states log so a failed dump is still
    # explained there rather than silently producing an empty file.
    kubectl get pods -A -o yaml >"${OUTPUT_DIR}/${PREFIX}-pods.yaml" \
        2>>"${OUTPUT_DIR}/${PREFIX}-pod-states.log" || true
}

collect_events() {
    capture "${OUTPUT_DIR}/${PREFIX}-events.log" \
        kubectl get events -A --sort-by=.lastTimestamp
}

collect_node_state() {
    capture "${OUTPUT_DIR}/${PREFIX}-nodes.log" kubectl get nodes -o wide
    capture "${OUTPUT_DIR}/${PREFIX}-nodes.log" kubectl describe nodes
}

collect_control_plane_health() {
    local output_file="${OUTPUT_DIR}/${PREFIX}-control-plane-health.log"

    capture "${output_file}" kubectl get --raw "/livez?verbose"
    capture "${output_file}" kubectl get --raw "/readyz?verbose"
    capture "${output_file}" kubectl get apiservices -o wide
}


# collect_kube_system_logs writes the current and previous logs of every
# kube-system container. Timestamps are included so the logs can be correlated
# with Radius pod logs and with the test output.
collect_kube_system_logs() {
    local logs_dir="${OUTPUT_DIR}/${PREFIX}-kube-system-logs"
    mkdir -p "${logs_dir}"

    local pods
    if ! pods="$(kubectl get pods -n kube-system \
        -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')"; then
        echo "Warning: unable to list kube-system pods" >&2
        return 0
    fi

    local pod container containers previous_file
    for pod in ${pods}; do
        if ! containers="$(kubectl get pod "${pod}" -n kube-system \
            -o jsonpath='{range .spec.initContainers[*]}{.name}{"\n"}{end}{range .spec.containers[*]}{.name}{"\n"}{end}')"; then
            continue
        fi

        for container in ${containers}; do
            kubectl logs "${pod}" -n kube-system -c "${container}" \
                --timestamps >"${logs_dir}/${pod}.${container}.log" 2>&1 || true

            # A previous instance only exists once a container has restarted,
            # and it holds the reason it died. Drop the file when there is no
            # previous instance so the artifact only contains real restarts.
            previous_file="${logs_dir}/${pod}.${container}.previous.log"
            if ! kubectl logs "${pod}" -n kube-system -c "${container}" \
                --previous --timestamps >"${previous_file}" 2>&1; then
                rm -f "${previous_file}"
            fi
        done
    done
}

main() {
    validate_requirements

    mkdir -p "${OUTPUT_DIR}"

    echo "Collecting cluster diagnostics into ${OUTPUT_DIR}"
    collect_pod_state
    collect_events
    collect_node_state
    collect_control_plane_health
    collect_kube_system_logs
    echo "Cluster diagnostics collection complete"
}

while [[ $# -gt 0 ]]; do
    case $1 in
        -o | --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -p | --prefix)
            PREFIX="$2"
            shift 2
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

main "$@"
