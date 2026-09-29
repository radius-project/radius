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
        de.resources.limits.memory=512Mi; do
        assert_contains " $1 " " --set ${value} "
    done
}

run_matching_version_case() (
    local initial_irsa_state="$1"
    local expect_upgrade="$2"
    local irsa_enabled="${initial_irsa_state}"
    local upgrade_args=""
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
            upgrade_args="$*"
            irsa_enabled="true"
        else
            fail "unexpected rad invocation: $*"
        fi
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    kubectl() {
        if [[ "$1 $2" == "get deployment" ]]; then
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

    if [[ "${irsa_enabled}" == "error" ]]; then
        if (main); then
            fail "expected deployment inspection failure to stop reconciliation"
        fi
        return 0
    fi

    main

    if [[ "${expect_upgrade}" == "true" ]]; then
        [[ -n "${upgrade_args}" ]] || fail "expected matching-version reconciliation to run"
        assert_contains "${upgrade_args}" "--skip-preflight"
        assert_required_values "${upgrade_args}"
    elif [[ -n "${upgrade_args}" ]]; then
        fail "did not expect an upgrade when IRSA was already enabled: ${upgrade_args}"
    fi

    [[ -s skip-delete-resources-list.txt ]] || fail "expected the skip-resources list to be saved"
)

run_install_or_upgrade_case() (
    local operation="$1" calls=0

    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/manage-radius-installation.sh"

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    rad() {
        [[ "$1 $2" == "${operation} kubernetes" ]] ||
            fail "unexpected rad invocation: $*"
        [[ $# -eq 12 ]] || fail "expected only the five required --set flags"
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

run_matching_version_case "false" "true"
run_matching_version_case "true" "false"
run_matching_version_case "error" "false"
run_install_or_upgrade_case "install"
run_install_or_upgrade_case "upgrade"

echo "manage-radius-installation tests passed"
