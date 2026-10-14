#!/bin/bash

# Requires: jq, yq (mikefarah)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly RP_PATH="/providers/System.Resources/resourceproviders"

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

# Two files share the Test.Compute namespace, and an SVG icon must be ignored,
# which matches how the UCP initializer merges the manifest directory.
write_manifests() {
    local dir="$1"
    mkdir -p "${dir}"
    cat >"${dir}/core.yaml" <<'EOF'
namespace: Test.Core
location:
  global:
    "http://applications-rp.radius-system:5443"
types:
  widgets:
    apiVersions:
      "2023-10-01-preview":
        schema: {}
EOF
    cat >"${dir}/containers.yaml" <<'EOF'
namespace: Test.Compute
types:
  containers:
    apiVersions:
      "2025-08-01-preview":
        schema: {}
EOF
    cat >"${dir}/routes.yaml" <<'EOF'
namespace: Test.Compute
types:
  routes:
    apiVersions:
      "2025-08-01-preview":
        schema: {}
      "2026-01-01-preview":
        schema: {}
EOF
    echo "<svg/>" >"${dir}/routes.svg"
}

# Fill UCP_STORE with every record the initializer writes for the fixture.
populate_complete_store() {
    UCP_STORE=()
    local path
    for path in \
        "${RP_PATH}/Test.Core" \
        "${RP_PATH}/Test.Core/locations/global" \
        "${RP_PATH}/Test.Core/resourcetypes/widgets" \
        "${RP_PATH}/Test.Core/resourcetypes/widgets/apiversions/2023-10-01-preview" \
        "${RP_PATH}/Test.Compute" \
        "${RP_PATH}/Test.Compute/locations/global" \
        "${RP_PATH}/Test.Compute/resourcetypes/containers" \
        "${RP_PATH}/Test.Compute/resourcetypes/containers/apiversions/2025-08-01-preview" \
        "${RP_PATH}/Test.Compute/resourcetypes/routes" \
        "${RP_PATH}/Test.Compute/resourcetypes/routes/apiversions/2025-08-01-preview" \
        "${RP_PATH}/Test.Compute/resourcetypes/routes/apiversions/2026-01-01-preview"; do
        UCP_STORE["${path}"]="{}"
    done
    UCP_STORE["/providers/Test.Core"]='{"name":"Test.Core","locations":{"global":{}},"resourceTypes":{"widgets":{"apiVersions":{"2023-10-01-preview":{}}}}}'
    UCP_STORE["/providers/Test.Compute"]='{"name":"Test.Compute","locations":{"global":{}},"resourceTypes":{"containers":{"apiVersions":{"2025-08-01-preview":{}}},"routes":{"apiVersions":{"2025-08-01-preview":{},"2026-01-01-preview":{}}}}}'
}

# Load the script under test with a fake UCP store and a fixture manifest
# directory. Each case runs in its own subshell.
setup_case() {
    WORK_DIR="$(mktemp -d)"
    # shellcheck disable=SC2064 # Expand WORK_DIR now.
    trap "rm -rf '${WORK_DIR}'" EXIT

    export RADIUS_MANIFEST_DIR="${WORK_DIR}/manifests"
    write_manifests "${RADIUS_MANIFEST_DIR}"
    export REGISTRATION_CHECK_ATTEMPTS=2
    export REGISTRATION_CHECK_INTERVAL_SECONDS=0

    # The production script is checked separately; this test sources it dynamically.
    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/manage-radius-installation.sh"

    declare -gA UCP_STORE=()
    UCP_QUERY_FAILS="false"
    IRSA_ENABLED="true"
    populate_complete_store

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    sleep() {
        return 0
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    kubectl() {
        if [[ "$1 $2" == "get --raw" ]]; then
            local path="${3#"${UCP_PLANE_PATH}"}"
            path="${path%%\?*}"
            if [[ "${UCP_QUERY_FAILS}" == "true" ]]; then
                echo "Error from server (ServiceUnavailable): the server is currently unable to handle the request" >&2
                return 1
            fi
            if [[ -z "${UCP_STORE["${path}"]+set}" ]]; then
                echo "Error from server (NotFound): the server could not find the requested resource" >&2
                return 1
            fi
            printf '%s' "${UCP_STORE["${path}"]}"
        elif [[ "$1 $2" == "get deployment" ]]; then
            if [[ "${IRSA_ENABLED}" == "error" ]]; then
                return 1
            fi
            if [[ "${IRSA_ENABLED}" == "true" ]]; then
                printf 'aws-iam-token'
            fi
        elif [[ "$1 $2" == "get resources.ucp.dev" ]]; then
            echo "resource-id"
        else
            fail "unexpected kubectl invocation: $*"
        fi
    }

    cd "${WORK_DIR}"
}

test_list_expected_registrations() (
    setup_case

    local actual expected
    actual=$(list_expected_registrations "${RADIUS_MANIFEST_DIR}" | sort)
    expected=$(
        printf '%s\n' \
            $'apiVersion\tTest.Compute\tcontainers\t2025-08-01-preview' \
            $'apiVersion\tTest.Compute\troutes\t2025-08-01-preview' \
            $'apiVersion\tTest.Compute\troutes\t2026-01-01-preview' \
            $'apiVersion\tTest.Core\twidgets\t2023-10-01-preview' \
            $'location\tTest.Compute\tglobal' \
            $'location\tTest.Core\tglobal' \
            $'provider\tTest.Compute' \
            $'provider\tTest.Core' \
            $'summary\tTest.Compute' \
            $'summary\tTest.Core' \
            $'type\tTest.Compute\tcontainers' \
            $'type\tTest.Compute\troutes' \
            $'type\tTest.Core\twidgets' | sort
    )
    [[ "${actual}" == "${expected}" ]] ||
        fail "unexpected registrations:"$'\n'"${actual}"
)

# Run verify_manifest_registration_complete and assert its status and output.
expect_verify_status() {
    local expected_status="$1" expected_text="${2:-}" output status=0
    output=$(verify_manifest_registration_complete 2>&1) || status=$?
    [[ "${status}" -eq "${expected_status}" ]] ||
        fail "expected status ${expected_status}, got ${status}: ${output}"
    if [[ -n "${expected_text}" ]]; then
        assert_contains "${output}" "${expected_text}"
    fi
}

test_verify_complete_registration() (
    setup_case
    expect_verify_status 0
)

# The provider entry exists, so the old presence check passed. The full
# check must detect the missing child records.
test_verify_detects_partial_registration() (
    setup_case
    local api_version="${RP_PATH}/Test.Compute/resourcetypes/routes/apiversions/2026-01-01-preview"
    unset 'UCP_STORE[${api_version}]'
    expect_verify_status 1 "apiVersion ${api_version}"

    populate_complete_store
    unset 'UCP_STORE[${RP_PATH}/Test.Core/locations/global]'
    expect_verify_status 1 "location ${RP_PATH}/Test.Core/locations/global"

    populate_complete_store
    unset 'UCP_STORE[${RP_PATH}/Test.Compute/resourcetypes/containers]'
    expect_verify_status 1 "type ${RP_PATH}/Test.Compute/resourcetypes/containers"
)

test_verify_detects_stale_summary() (
    setup_case
    UCP_STORE["/providers/Test.Compute"]='{"name":"Test.Compute","locations":{"global":{}},"resourceTypes":{"containers":{"apiVersions":{"2025-08-01-preview":{}}},"routes":{"apiVersions":{"2025-08-01-preview":{}}}}}'
    expect_verify_status 1 "summary apiVersion Test.Compute/routes@2026-01-01-preview"
)

test_verify_reports_query_failure() (
    setup_case
    UCP_QUERY_FAILS="true"
    expect_verify_status 2 "ServiceUnavailable"
)

run_matching_version_case() (
    local initial_irsa_state="$1"
    local expect_upgrade="$2"
    local upgrade_args=""

    setup_case
    IRSA_ENABLED="${initial_irsa_state}"

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    rad() {
        if [[ "$1" == "version" ]]; then
            printf 'RELEASE\n0.60.0\nSTATUS VERSION\nReady 0.60.0\n'
        elif [[ "$1 $2" == "upgrade kubernetes" ]]; then
            upgrade_args="$*"
            IRSA_ENABLED="true"
        else
            fail "unexpected rad invocation: $*"
        fi
    }

    if [[ "${IRSA_ENABLED}" == "error" ]]; then
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

# A same-version installation with a partial registration is reinstalled.
# The script must then confirm that every built-in record is usable before it
# saves the skip list. When the reinstall restores records, the run passes;
# when the initializer still leaves records missing, the run fails.
run_partial_recovery_case() (
    local install_restores="$1"

    setup_case
    local calls_file="${WORK_DIR}/rad-calls.txt"
    unset 'UCP_STORE[${RP_PATH}/Test.Compute/resourcetypes/routes]'

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    rad() {
        echo "$1 ${2:-}" >>"${calls_file}"
        if [[ "$1" == "version" ]]; then
            printf 'RELEASE\n0.60.0\nSTATUS VERSION\nReady 0.60.0\n'
        elif [[ "$1 $2" == "uninstall kubernetes" ]]; then
            UCP_STORE=()
        elif [[ "$1 $2" == "install kubernetes" ]]; then
            assert_required_values "$*"
            if [[ "${install_restores}" == "true" ]]; then
                populate_complete_store
            else
                UCP_STORE["${RP_PATH}/Test.Compute"]="{}"
            fi
        else
            fail "unexpected rad invocation: $*"
        fi
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    verify_manifests_registered() {
        return 0
    }

    if [[ "${install_restores}" == "true" ]]; then
        main
        [[ -s skip-delete-resources-list.txt ]] ||
            fail "expected the skip-resources list after recovery"
    else
        if (main); then
            fail "expected recovery to fail when built-in records stay missing"
        fi
        [[ ! -e skip-delete-resources-list.txt ]] ||
            fail "did not expect a skip-resources list after failed recovery"
    fi

    grep -Fxq "uninstall kubernetes" "${calls_file}" ||
        fail "expected a purge before reinstall"
    grep -Fxq "install kubernetes" "${calls_file}" ||
        fail "expected a reinstall"
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
    wait_for_manifest_registration() {
        return 0
    }

    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    save_skip_resources_list() {
        return 0
    }

    "${operation}_radius"
    [[ "${calls}" -eq 1 ]] || fail "expected exactly one ${operation}"
)

test_requires_manifest_dir() (
    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/manage-radius-installation.sh"
    RADIUS_MANIFEST_DIR="/does/not/exist"
    # shellcheck disable=SC2329 # Invoked indirectly by the sourced script.
    command() {
        return 0
    }
    if (require_prerequisites 2>/dev/null); then
        fail "expected a missing manifest directory to stop the script"
    fi
)

for tool in jq yq; do
    command -v "${tool}" >/dev/null || fail "${tool} is required"
done

test_list_expected_registrations
test_verify_complete_registration
test_verify_detects_partial_registration
test_verify_detects_stale_summary
test_verify_reports_query_failure
test_requires_manifest_dir
run_matching_version_case "false" "true"
run_matching_version_case "true" "false"
run_matching_version_case "error" "false"
run_partial_recovery_case "true"
run_partial_recovery_case "false"
run_install_or_upgrade_case "install"
run_install_or_upgrade_case "upgrade"

echo "manage-radius-installation tests passed"
