#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.

# Requires yq v4, jq, Go, and Make; no cluster or cloud credentials.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT
readonly WORKFLOW="${REPO_ROOT}/.github/workflows/long-running-azure.yaml"
readonly FRESH_WORKFLOW="${REPO_ROOT}/.github/workflows/functional-test-noncloud.yaml"

for tool in yq jq go make; do
    command -v "${tool}" >/dev/null 2>&1 || {
        echo "${tool} is required for long-running Azure workflow tests." >&2
        exit 1
    }
done

WORK_DIR="$(mktemp -d)"
readonly WORK_DIR
trap 'rm -rf "${WORK_DIR}"' EXIT
unset MAKEFLAGS MFLAGS GOTEST_OPTS GOTEST_TOOL

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

yq -e -r '.jobs.tests.steps[] |
    select(.name == "Run functional tests") | .run' \
    "${WORKFLOW}" >"${WORK_DIR}/run.sh"
GOTEST_OPTS="$(yq -e -r '.jobs.tests.steps[] |
    select(.name == "Run functional tests") | .env.GOTEST_OPTS' \
    "${WORKFLOW}")"
export GOTEST_OPTS

# Exercise the workflow's quoting and Make's $$ expansion with Go's real -skip.
mkdir -p "${WORK_DIR}/fixture"
printf 'package fixture\nimport "testing"\n' \
    >"${WORK_DIR}/fixture/selection_test.go"
excluded=(
    Test_ACI
    Test_RadiusCore_AzureMySql_PortalLink
    Test_RabbitMQ
    Test_RedisCache
    Test_Container_ManagedSecretConnection
    Test_AuthzMode_LoggedByEveryComponent
)
included=(
    Test_AuthzMode_LoggedByEveryComponent_AdditionalCheck
    Test_Prefix_Test_AuthzMode_LoggedByEveryComponent
    Test_Boundary_UnauthenticatedCallToApplicationsRP
    Test_ExpectedMode
    Test_LoggedMode
    Test_ExpectedTestMode
    Test_ACI_AdditionalCheck
)
for name in "${excluded[@]}"; do
    printf 'func %s(t *testing.T) { t.Fatal("must be excluded in LRT") }\n' \
        "${name}" >>"${WORK_DIR}/fixture/selection_test.go"
done
for name in "${included[@]}"; do
    printf 'func %s(t *testing.T) {}\n' \
        "${name}" >>"${WORK_DIR}/fixture/selection_test.go"
done

cat >"${WORK_DIR}/run-go-tests.sh" <<'MOCK'
#!/bin/bash
set -euo pipefail
[[ "$1" == ./test/functional-portable/authz/noncloud/... ]]
shift
cd "${SELECTION_FIXTURE}"
GO111MODULE=off GOWORK=off exec go test "$@"
MOCK
export SELECTION_FIXTURE="${WORK_DIR}/fixture"
make --no-print-directory -C "${REPO_ROOT}" -f build/test.mk \
    test-functional-authz-noncloud \
    GOTEST_TOOL="bash '${WORK_DIR}/run-go-tests.sh'" \
    TEST_TIMEOUT=60s >"${WORK_DIR}/selection.log" 2>&1 || {
    cat "${WORK_DIR}/selection.log" >&2
    fail "LRT test selection through Make"
}
for name in "${included[@]}"; do
    grep -Fq -- "--- PASS: ${name} " "${WORK_DIR}/selection.log" \
        || fail "${name} was incorrectly excluded"
done

# Execute the entire workflow step with read-only readiness and test doubles.
export GITHUB_WORKSPACE="${WORK_DIR}/workspace"
mkdir -p "${GITHUB_WORKSPACE}/bin" "${GITHUB_WORKSPACE}/.github/scripts"
cat >"${GITHUB_WORKSPACE}/bin/rad" <<'MOCK'
#!/bin/bash
exit 0
MOCK
cat >"${GITHUB_WORKSPACE}/bin/kubectl" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo "kubectl $*" >>"${MOCK_CALLS}"
if [[ "$3" == "deployment/${MOCK_UNREADY:-}" ]]; then
    echo "deployment ${MOCK_UNREADY} did not become ready" >&2
    exit 42
fi
if [[ "$1" == get ]]; then
    if [[ "$3" == "${MOCK_GET_FAILURE:-}" ]]; then
        echo "deployment read failed" >&2
        exit 44
    elif [[ "$3" == "${MOCK_ZERO_READY:-}" ]]; then
        echo '{"status":{"readyReplicas":0}}'
    elif [[ "$3" == "${MOCK_MISSING_READY:-}" ]]; then
        echo '{"status":{}}'
    else
        echo '{"status":{"readyReplicas":1}}'
    fi
fi
MOCK
cat >"${GITHUB_WORKSPACE}/bin/make" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo "make $*" >>"${MOCK_CALLS}"
if [[ "$1" == "${MOCK_FAILED_TARGET:-}" ]]; then
    exit 43
fi
MOCK
cat >"${GITHUB_WORKSPACE}/.github/scripts/run-with-cluster-diagnostics.sh" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo diagnostics >>"${MOCK_CALLS}"
exec "$@"
MOCK
chmod +x "${GITHUB_WORKSPACE}/bin/"*
export FUNCTEST_PREPROVISIONED_RESOURCE_JSON='{
    "AZURE_COSMOS_MONGODB_ACCOUNT_ID": "fixture",
    "AZURE_MSSQL_RESOURCE_ID": "fixture",
    "AZURE_MSSQL_USERNAME": "fixture",
    "AZURE_MSSQL_PASSWORD": "fixture"
}'
export MOCK_CALLS="${WORK_DIR}/calls"

printf 'diagnostics\n' >"${WORK_DIR}/expected"
for component in ucp applications-rp dynamic-rp controller; do
    printf 'kubectl rollout status deployment/%s --namespace radius-system --timeout=120s\n' \
        "${component}" >>"${WORK_DIR}/expected"
    printf 'kubectl get deployment %s --namespace radius-system --request-timeout=10s -o json\n' \
        "${component}" >>"${WORK_DIR}/expected"
done
printf 'make test-functional-all-cloud\nmake test-functional-all-noncloud\n' \
    >>"${WORK_DIR}/expected"

bash "${WORK_DIR}/run.sh" >"${WORK_DIR}/run.log" 2>&1 || {
    cat "${WORK_DIR}/run.log" >&2
    fail "healthy deployments should run both aggregates"
}
diff -u "${WORK_DIR}/expected" "${MOCK_CALLS}"

for component in ucp applications-rp dynamic-rp controller; do
    : >"${MOCK_CALLS}"
    result=0
    MOCK_UNREADY="${component}" bash "${WORK_DIR}/run.sh" \
        >"${WORK_DIR}/run.log" 2>&1 || result=$?
    [[ "${result}" == 42 ]] || {
        cat "${WORK_DIR}/run.log" >&2
        fail "${component} readiness failure was not propagated"
    }
    grep -Fxq diagnostics "${MOCK_CALLS}" \
        || fail "readiness failure bypassed diagnostics"
    grep -Fq "deployment/${component} " "${MOCK_CALLS}" \
        || fail "${component} readiness was not checked"
    if grep -q '^make ' "${MOCK_CALLS}"; then
        fail "functional tests ran after ${component} readiness failed"
    fi
done

for scenario in MOCK_ZERO_READY MOCK_MISSING_READY MOCK_GET_FAILURE; do
    : >"${MOCK_CALLS}"
    result=0
    env "${scenario}=controller" bash "${WORK_DIR}/run.sh" \
        >"${WORK_DIR}/run.log" 2>&1 || result=$?
    [[ "${result}" != 0 ]] || fail "${scenario} did not fail readiness"
    if grep -q '^make ' "${MOCK_CALLS}"; then
        fail "functional tests ran after ${scenario}"
    fi
done

: >"${MOCK_CALLS}"
result=0
MOCK_FAILED_TARGET=test-functional-all-cloud bash "${WORK_DIR}/run.sh" \
    >"${WORK_DIR}/run.log" 2>&1 || result=$?
[[ "${result}" == 43 ]] || fail "cloud test failure was not propagated"
if grep -Fq 'make test-functional-all-noncloud' "${MOCK_CALLS}"; then
    fail "non-cloud tests ran after cloud tests failed"
fi

fresh_options="$(yq -r '.jobs[].steps[] |
    select(.env.AUTHZ_EXPECTED_MODE != null) | .env.AUTHZ_EXPECTED_MODE' \
    "${FRESH_WORKFLOW}")"
[[ "${fresh_options}" == \
    "\${{ matrix.name == 'authz-noncloud' && 'dryRun' || '' }}" ]] \
    || fail "fresh-install CI no longer requires dryRun mode"
fresh_skip_options="$(yq -r '.jobs[].steps[] | select(.env.GOTEST_OPTS != null) |
    .env.GOTEST_OPTS' "${FRESH_WORKFLOW}")"
if [[ "${fresh_skip_options}" == *Test_AuthzMode_LoggedByEveryComponent* ]]; then
    fail "fresh-install CI excludes the startup-log smoke test"
fi

echo "Long-running Azure workflow tests passed"
