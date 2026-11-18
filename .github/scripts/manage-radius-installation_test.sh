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

assert_required_values() {
    local value
    for value in \
        global.azureWorkloadIdentity.enabled=true \
        global.aws.irsa.enabled=true \
        database.enabled=false \
        de.resources.requests.memory=256Mi \
        de.resources.limits.memory=512Mi \
        dynamicrp.resources.limits.memory=1Gi; do
        assert_contains " $1 " " --set ${value} "
    done
}

run_matching_version_case() (
    local initial_irsa_state="$1"
    local initial_memory_limit="$2"
    local expect_upgrade="$3"
    local failure_mode="${4:-}"
    local expected_error="${5:-}"
    local irsa_enabled="${initial_irsa_state}"
    local configured_memory_limit="${initial_memory_limit}"
    local work_dir
    work_dir="$(mktemp -d)"
    trap 'rm -rf "${work_dir}"' EXIT

    # The production script is checked separately; this test sources it dynamically.
    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/manage-radius-installation.sh"

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    rad() {
        if [[ "$1" == "version" ]]; then
            printf 'RELEASE\n0.60.0\nSTATUS VERSION\nReady 0.60.0\n'
        elif [[ "$1 $2" == "workspace create" ]]; then
            return 0
        elif [[ "$1 $2" == "resource-provider list" ]]; then
            echo "Applications.Core"
        elif [[ "$1 $2" == "upgrade kubernetes" ]]; then
            printf '%s\n' "$*" >> "${work_dir}/upgrade-args.txt"
            if [[ "${failure_mode}" == "upgrade-fails" ]]; then
                return 1
            fi
            if [[ "${failure_mode}" != "irsa-unreconciled" ]]; then
                irsa_enabled="true"
            fi
            if [[ "${failure_mode}" != "memory-unreconciled" ]]; then
                configured_memory_limit="1Gi"
            fi
        else
            fail "unexpected rad invocation: $*"
        fi
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    kubectl() {
        if [[ "$1 $2" == "get deployment" ]]; then
            if [[ "$*" == *".resources.limits.memory"* ]]; then
                [[ "${configured_memory_limit}" != "error" ]] || return 1
                printf '%s' "${configured_memory_limit}"
            else
                if [[ "${irsa_enabled}" == "error" ]]; then
                    return 1
                fi
                if [[ "${irsa_enabled}" == "true" ]]; then
                    printf 'aws-iam-token'
                fi
            fi
        elif [[ "$1 $2" == "get resources.ucp.dev" ]]; then
            echo "resource-id"
        else
            fail "unexpected kubectl invocation: $*"
        fi
    }

    cd "${work_dir}"

    if [[ -n "${expected_error}" ]]; then
        if (main) > output.log 2>&1; then
            fail "expected reconciliation failure: ${expected_error}"
        fi
        if grep -q '^FAIL:' output.log; then
            cat output.log >&2
            fail "unexpected mock invocation"
        fi
        assert_contains "$(cat output.log)" "${expected_error}"
        [[ ! -e skip-delete-resources-list.txt ]] || fail "saved skip-resources list after failure"
    else
        main
        [[ -s skip-delete-resources-list.txt ]] || fail "expected the skip-resources list to be saved"
    fi

    if [[ "${expect_upgrade}" == "true" ]]; then
        [[ -s upgrade-args.txt ]] || fail "expected matching-version reconciliation to run"
        local upgrade_args
        upgrade_args="$(cat upgrade-args.txt)"
        assert_contains "${upgrade_args}" "--skip-preflight"
        assert_required_values "${upgrade_args}"
        [[ "$(wc -l < upgrade-args.txt)" -eq 1 ]] || fail "expected exactly one reconciliation"
    elif [[ -e upgrade-args.txt ]]; then
        fail "did not expect an upgrade: $(cat upgrade-args.txt)"
    fi
)

run_install_or_upgrade_case() (
    local operation="$1" calls=0

    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/manage-radius-installation.sh"

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    rad() {
        [[ "$1 $2" == "${operation} kubernetes" ]] ||
            fail "unexpected rad invocation: $*"
        [[ $# -eq $((${#REQUIRED_CHART_VALUES[@]} + 2)) ]] ||
            fail "expected only the required --set flags"
        assert_required_values "$*"
        calls=$((calls + 1))
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    verify_manifests_registered() {
        return 0
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    save_skip_resources_list() {
        return 0
    }

    "${operation}_radius"
    [[ "${calls}" -eq 1 ]] || fail "expected exactly one ${operation}"
)

run_matching_version_case "false" "500Mi" "true"
run_matching_version_case "false" "1Gi" "true"
run_matching_version_case "true" "500Mi" "true"
run_matching_version_case "true" "" "true"
run_matching_version_case "true" "1Gi" "false"
run_matching_version_case "error" "1Gi" "false" "" \
    "Failed to inspect deployment ucp for AWS IRSA configuration."
run_matching_version_case "true" "error" "false" "" \
    "Failed to inspect dynamic-rp memory limit."
run_matching_version_case "true" "500Mi" "true" "upgrade-fails" \
    "Failed to reconcile required Radius chart values."
run_matching_version_case "false" "1Gi" "true" "irsa-unreconciled" \
    "AWS IRSA token volumes are still missing after reconciliation."
run_matching_version_case "true" "500Mi" "true" "memory-unreconciled" \
    "dynamic-rp memory limit is still not 1Gi after reconciliation."
run_install_or_upgrade_case "install"
run_install_or_upgrade_case "upgrade"

echo "manage-radius-installation tests passed"
