#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    local actual="$1"
    local expected="$2"

    [[ "${actual}" == *"${expected}"* ]] || fail "expected '${actual}' to contain '${expected}'"
}

run_case() (
    local name="$1" mock_cp_version="$2" mock_irsa_enabled="$3"
    local mock_memory_values="$4"
    local operation_result="$5" expected_operation="$6" expected_status="$7"
    local expected_message="${8:-}"
    local work_dir
    work_dir="$(mktemp -d)"
    trap 'rm -rf "${work_dir}"' EXIT
    local calls="${work_dir}/calls"
    touch "${calls}"

    # The production script is checked separately; this test sources it dynamically.
    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/manage-radius-installation.sh"

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    rad() {
        if [[ "$1" == "version" ]]; then
            printf 'RELEASE\n0.61.0\nSTATUS VERSION\n%s\n' "${mock_cp_version}"
        elif [[ "$1 $2" == "workspace create" ]]; then
            return 0
        elif [[ "$1 $2" == "resource-provider list" ]]; then
            echo "Applications.Core"
        elif [[ "$1 $2" == "upgrade kubernetes" || \
                "$1 $2" == "install kubernetes" ]]; then
            printf '%s\n' "$*" >> "${calls}"
            if [[ "${operation_result}" == "failure" ]]; then
                echo 'Apply failed: conflicts with "kubectl-set"' >&2
                return 1
            fi
            if [[ "${operation_result}" == "unchanged" ]]; then
                return 0
            fi
            mock_irsa_enabled="true"
            mock_memory_values="256Mi 512Mi"
        else
            fail "unexpected rad invocation: $*"
        fi
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    kubectl() {
        if [[ "$1 $2" == "get deployment" ]]; then
            if [[ "$3" == "bicep-de" ]]; then
                [[ "$*" == *'.resources.requests.memory'* ]] ||
                    fail "expected a memory request query"
                [[ "$*" == *'.resources.limits.memory'* ]] ||
                    fail "expected a memory limit query"
                if [[ "${mock_memory_values}" == "error" ]]; then
                    echo "Forbidden: cannot read bicep-de" >&2
                    return 1
                fi
                printf '%s' "${mock_memory_values}"
                return 0
            fi
            if [[ "${mock_irsa_enabled}" == "error" ]]; then
                echo "Forbidden: cannot read IRSA configuration" >&2
                return 1
            fi
            if [[ "${mock_irsa_enabled}" == "true" ]]; then
                printf 'aws-iam-token'
            fi
        elif [[ "$1 $2" == "get resources.ucp.dev" ]]; then
            echo "resource-id"
        else
            fail "unexpected kubectl invocation: $*"
        fi
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    verify_manifests_registered() {
        echo "Manifest verification complete."
    }

    cd "${work_dir}"

    local status=0
    (main) > output.log 2>&1 || status=$?
    if [[ "${status}" -ne "${expected_status}" ]]; then
        cat output.log >&2
        fail "${name}: expected status ${expected_status}, got ${status}"
    fi
    local operation_args
    operation_args="$(<"${calls}")"
    if [[ "${expected_operation}" == "none" ]]; then
        [[ -z "${operation_args}" ]] ||
            fail "${name}: unexpected operation: ${operation_args}"
    else
        [[ "$(wc -l < "${calls}")" -eq 1 ]] ||
            fail "${name}: expected exactly one operation"
        assert_contains "${operation_args}" "${expected_operation} kubernetes"
        assert_contains "${operation_args}" "global.azureWorkloadIdentity.enabled=true"
        assert_contains "${operation_args}" "global.aws.irsa.enabled=true"
        assert_contains "${operation_args}" "database.enabled=false"
        assert_contains "${operation_args}" "de.resources.requests.memory=256Mi"
        assert_contains "${operation_args}" "de.resources.limits.memory=512Mi"
        if [[ "${mock_cp_version}" == "Ready 0.61.0" ]]; then
            assert_contains "${operation_args}" "--skip-preflight"
        else
            [[ "${operation_args}" != *"--skip-preflight"* ]] ||
                fail "${name}: unexpectedly skipped upgrade preflight"
        fi
    fi

    if [[ -n "${expected_message}" ]]; then
        assert_contains "$(<output.log)" "${expected_message}"
    fi
    if [[ "${status}" -eq 0 ]]; then
        [[ -s skip-delete-resources-list.txt ]] ||
            fail "${name}: expected the skip-resources list to be saved"
    else
        [[ ! -e skip-delete-resources-list.txt ]] ||
            fail "${name}: saved skip-resources list after failure"
        [[ "$(<output.log)" != *"Radius Control Plane Management Complete"* ]] ||
            fail "${name}: reported completion after failure"
    fi
    echo "PASS: ${name}"
)

run_case "fresh install" "Not installed" false "" success install 0
run_case "version upgrade adopts live tuning" "Ready 0.60.2" true \
    "256Mi 512Mi" success upgrade 0
run_case "version upgrade configures chart defaults" "Ready 0.60.2" true \
    "130Mi 300Mi" success upgrade 0
run_case "matching version configures IRSA" "Ready 0.61.0" false \
    "256Mi 512Mi" success upgrade 0
run_case "matching version configures memory" "Ready 0.61.0" true \
    "130Mi 300Mi" success upgrade 0
run_case "matching version configures missing memory" "Ready 0.61.0" true \
    " " success upgrade 0
run_case "matching configuration is a no-op" "Ready 0.61.0" true \
    "256Mi 512Mi" success none 0
run_case "IRSA inspection failure" "Ready 0.61.0" error \
    "256Mi 512Mi" success none 1 "Failed to inspect deployment"
run_case "memory inspection failure" "Ready 0.61.0" true \
    error success none 1 "Failed to inspect bicep-de"
run_case "upgrade memory inspection failure" "Ready 0.60.2" true \
    error success none 1 "Failed to inspect bicep-de"
run_case "install failure" "Not installed" false "" failure install 1 \
    "Apply failed"
run_case "upgrade conflict" "Ready 0.60.2" true \
    "256Mi 512Mi" failure upgrade 1 "Apply failed"
run_case "reconciliation conflict" "Ready 0.61.0" true \
    "130Mi 300Mi" failure upgrade 1 "Apply failed"
run_case "upgrade postcondition failure" "Ready 0.60.2" true \
    "130Mi 300Mi" unchanged upgrade 1 "missing after upgrade"
run_case "reconciliation postcondition failure" "Ready 0.61.0" false \
    "256Mi 512Mi" unchanged upgrade 1 "missing after upgrade"
run_case "install postcondition failure" "Not installed" false \
    "" unchanged install 1 "missing after installation"

echo "manage-radius-installation tests passed"
