#!/bin/bash

# Test the persistent-cluster test selection without a cluster. Requires yq.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly REPO_ROOT

command -v yq >/dev/null 2>&1 || {
    echo "yq is required for long-running Azure workflow tests." >&2
    exit 1
}

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

options="$(yq -er '.jobs.tests.steps[] |
    select(.name == "Run functional tests") | .env.GOTEST_OPTS' \
    "${REPO_ROOT}/.github/workflows/long-running-azure.yaml")"

# Exercise Make's dollar escaping and shell quoting, not just the YAML string.
args="$(make --no-print-directory -s -f "${REPO_ROOT}/build/test.mk" \
    test-functional-authz-noncloud GOTEST_TOOL="printf '%s\n'" \
    GOTEST_OPTS="${options}")"
skip="$(grep '^-skip=' <<<"${args}")"
pattern="${skip#-skip=}"

for name in \
    Test_AuthzMode_LoggedByEveryComponent \
    Test_ACI \
    Test_RadiusCore_AzureMySql_PortalLink \
    Test_RabbitMQ \
    Test_RedisCache \
    Test_Container_ManagedSecretConnection; do
    [[ "${name}" =~ ${pattern} ]] || fail "${name} must be excluded"
    for other in "Prefix${name}" "${name}_Other"; do
        [[ ! "${other}" =~ ${pattern} ]] \
            || fail "exclusion must match the exact name, not ${other}"
    done
done

for name in Test_ExpectedMode Test_LoggedMode Test_ExpectedTestMode \
    Test_Boundary_UnauthenticatedCallToApplicationsRP; do
    [[ ! "${name}" =~ ${pattern} ]] || fail "${name} must remain enabled"
done

args="$(make --no-print-directory -s -f "${REPO_ROOT}/build/test.mk" \
    test-functional-authz-noncloud GOTEST_TOOL="printf '%s\n'" GOTEST_OPTS=)"
[[ "${args}" != *-skip* ]] \
    || fail "the default authz target must retain startup-log coverage"

fresh_config="$(yq -r '.. | select(tag == "!!str")' \
    "${REPO_ROOT}/.github/workflows/functional-test-noncloud.yaml")"
if grep -Fq 'Test_AuthzMode_LoggedByEveryComponent' <<<"${fresh_config}"; then
    fail "the persistent-cluster exclusion must not apply to fresh-cluster CI"
fi

echo "long-running Azure workflow tests passed"
