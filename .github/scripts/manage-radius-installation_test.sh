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
        dynamicrp.resources.requests.memory=512Mi \
        dynamicrp.resources.limits.memory=2Gi; do
        assert_contains " $1 " " --set ${value} "
    done
}

run_matching_version_case() (
    local initial_irsa_state="$1"
    local configured_memory="$2"
    local expect_upgrade="$3"
    local upgrade_behavior="${4:-success}"
    local irsa_enabled="${initial_irsa_state}"
    local upgrade_args=""
    local upgrade_calls=0
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
            echo "$*" >>"${work_dir}/upgrades"
            if [[ "${upgrade_behavior}" == "failure" ]]; then
                return 1
            fi
            upgrade_args="$*"
            upgrade_calls=$((upgrade_calls + 1))
            if [[ "${upgrade_behavior}" != "unchanged-irsa" ]]; then
                irsa_enabled="true"
            fi
            if [[ "${upgrade_behavior}" != "unchanged-memory" ]]; then
                configured_memory="512Mi 2Gi"
            fi
        else
            fail "unexpected rad invocation: $*"
        fi
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    kubectl() {
        if [[ "$1 $2" == "get deployment" ]]; then
            if [[ "${*: -1}" == *".resources.requests.memory"* ]]; then
                [[ "$3" == "dynamic-rp" ]] ||
                    fail "expected dynamic-rp memory inspection"
                if [[ "${configured_memory}" == "error" ]]; then
                    return 1
                fi
                printf '%s' "${configured_memory}"
                return 0
            fi
            if [[ "${irsa_enabled}" == "error" ]]; then
                return 1
            fi
            if [[ "${irsa_enabled}" == "true" ]]; then
                printf 'aws-iam-token'
            fi
        elif [[ "$1 $2" == "get resources.ucp.dev" ]]; then
            echo "resource-id"
        else
            fail "unexpected kubectl invocation: $*"
        fi
    }

    cd "${work_dir}"

    if [[ "${irsa_enabled}" == "error" || "${configured_memory}" == "error" ||
        "${upgrade_behavior}" != "success" ]]; then
        if (main) >"${work_dir}/output" 2>&1; then
            fail "expected failed inspection or reconciliation to stop setup"
        fi
        if [[ "${irsa_enabled}" == "error" || "${configured_memory}" == "error" ]]; then
            [[ ! -e "${work_dir}/upgrades" ]] ||
                fail "must not upgrade after deployment inspection fails"
            assert_contains "$(cat "${work_dir}/output")" "Failed to inspect"
        elif [[ "${upgrade_behavior}" == "failure" ]]; then
            assert_contains "$(cat "${work_dir}/output")" "Failed to reconcile"
        else
            assert_contains "$(cat "${work_dir}/output")" "still missing after reconciliation"
        fi
        [[ ! -e skip-delete-resources-list.txt ]] ||
            fail "must not save resources after failed reconciliation"
        return 0
    fi

    main

    if [[ "${expect_upgrade}" == "true" ]]; then
        [[ -n "${upgrade_args}" ]] || fail "expected matching-version reconciliation to run"
        assert_contains "${upgrade_args}" "--skip-preflight"
        assert_required_values "${upgrade_args}"
    elif [[ -n "${upgrade_args}" ]]; then
        fail "did not expect an upgrade when required values were configured: ${upgrade_args}"
    fi

    [[ -s skip-delete-resources-list.txt ]] || fail "expected the skip-resources list to be saved"

    local previous_calls="${upgrade_calls}"
    main
    [[ "${upgrade_calls}" -eq "${previous_calls}" ]] ||
        fail "must not upgrade again after reconciliation"
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

run_matching_version_case "false" "512Mi 2Gi" "true"
run_matching_version_case "true" "160Mi 500Mi" "true"
run_matching_version_case "true" "160Mi 2Gi" "true"
run_matching_version_case "true" "512Mi 500Mi" "true"
run_matching_version_case "true" " " "true"
run_matching_version_case "false" "160Mi 500Mi" "true"
run_matching_version_case "true" "512Mi 2Gi" "false"
run_matching_version_case "error" "512Mi 2Gi" "false"
run_matching_version_case "true" "error" "false"
run_matching_version_case "false" "error" "false"
run_matching_version_case "true" "160Mi 500Mi" "true" "failure"
run_matching_version_case "true" "160Mi 500Mi" "true" "unchanged-memory"
run_matching_version_case "false" "512Mi 2Gi" "true" "unchanged-irsa"
run_install_or_upgrade_case "install"
run_install_or_upgrade_case "upgrade"

echo "manage-radius-installation tests passed"
