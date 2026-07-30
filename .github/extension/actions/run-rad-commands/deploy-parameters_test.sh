#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_param_present() {
    local expected="$1"
    local actual

    for actual in "${DEPLOY_ARGS[@]-}"; do
        if [[ "${actual}" == "${expected}" ]]; then
            return 0
        fi
    done

    fail "expected deploy parameters to contain '${expected}'"
}

assert_param_absent() {
    local unexpected="$1"
    local actual

    for actual in "${DEPLOY_ARGS[@]-}"; do
        if [[ "${actual}" == "${unexpected}" ]]; then
            fail "did not expect deploy parameters to contain '${unexpected}'"
        fi
    done
}

deploy_params_has_key() {
    [[ -n "${DEPLOY_PARAMS_JSON}" ]] || return 1
    printf '%s' "${DEPLOY_PARAMS_JSON}" |
        jq -e --arg k "$1" \
            'has($k) and (.[$k] != null) and (.[$k] != "")' \
            >/dev/null
}

reset_discovery() {
    DECLARED_APP_PARAMS=()
    DECLARED_APP_PARAMS_LOADED=false
    GENERATED_APP_PARAMS=()
    DEPLOY_ARGS=()
    : >"${BICEP_CALLS}"
}

write_template() {
    local parameters="$1"
    jq -n --argjson parameters "${parameters}" \
        '{parameters:$parameters, resources:[]}' >"${FAKE_ARM_TEMPLATE}"
}

run_generated_params_case() {
    local parameters="$1"

    reset_discovery
    write_template "${parameters}"
    load_declared_app_params "${APP_FILE}"
    append_generated_app_params
    if ((${#GENERATED_APP_PARAMS[@]} > 0)); then
        DEPLOY_ARGS+=("${GENERATED_APP_PARAMS[@]}")
    fi
}

readonly FAKE_ARM_TEMPLATE="${TEST_ROOT}/app.json"
readonly BICEP_CALLS="${TEST_ROOT}/bicep-calls"
readonly BICEP_BIN="${TEST_ROOT}/bicep"
readonly APP_FILE="${TEST_ROOT}/app.bicep"

cat >"${BICEP_BIN}" <<'EOF'
#!/bin/bash
set -euo pipefail

printf '%s\n' "$*" >>"${BICEP_CALLS}"
if [[ "${BICEP_SHOULD_FAIL:-false}" == "true" ]]; then
    echo "synthetic compile failure" >&2
    exit 1
fi
[[ "$1" == "build" && "$3" == "--outfile" ]]
cp "${FAKE_ARM_TEMPLATE}" "$4"
EOF
chmod +x "${BICEP_BIN}"
touch "${APP_FILE}" "${BICEP_CALLS}"
export BICEP_BIN BICEP_CALLS FAKE_ARM_TEMPLATE

# The production helper is sourced by the composite action.
# shellcheck source=.github/extension/actions/run-rad-commands/deploy-parameters.sh
source "${SCRIPT_DIR}/deploy-parameters.sh"

APP_IMAGE="ghcr.io/example/app:sha"
REGISTRY_USERNAME="octocat"
REGISTRY_PASSWORD="token"
DEPLOY_PARAMS_JSON=""

run_generated_params_case \
    '{"environment":{},"registryUsername":{},"registryPassword":{}}'
assert_param_absent "image=${APP_IMAGE}"
assert_param_present "registryUsername=${REGISTRY_USERNAME}"
assert_param_present "registryPassword=${REGISTRY_PASSWORD}"

run_generated_params_case \
    '{"environment":{},"image":{},"registryUsername":{},"registryPassword":{}}'
assert_param_present "image=${APP_IMAGE}"
assert_param_present "registryUsername=${REGISTRY_USERNAME}"
assert_param_present "registryPassword=${REGISTRY_PASSWORD}"

run_generated_params_case '{"environment":{}}'
assert_param_absent "image=${APP_IMAGE}"
assert_param_absent "registryUsername=${REGISTRY_USERNAME}"
assert_param_absent "registryPassword=${REGISTRY_PASSWORD}"

DEPLOY_PARAMS_JSON='{"registryUsername":"configured-user"}'
run_generated_params_case \
    '{"image":{},"registryUsername":{},"registryPassword":{}}'
assert_param_present "image=${APP_IMAGE}"
assert_param_absent "registryUsername=${REGISTRY_USERNAME}"
assert_param_present "registryPassword=${REGISTRY_PASSWORD}"
DEPLOY_PARAMS_JSON=""

reset_discovery
write_template '{"image":{}}'
load_declared_app_params "${APP_FILE}"
load_declared_app_params "${APP_FILE}"
[[ "$(wc -l <"${BICEP_CALLS}" | tr -d ' ')" == "1" ]] ||
    fail "expected the app template to compile once"

reset_discovery
export BICEP_SHOULD_FAIL=true
if load_declared_app_params "${APP_FILE}"; then
    fail "expected a Bicep compilation failure to stop parameter discovery"
fi
unset BICEP_SHOULD_FAIL
[[ "${DECLARED_APP_PARAMS_LOADED}" == "false" ]] ||
    fail "failed compilation must not populate the parameter cache"

action_file="${SCRIPT_DIR}/action.yml"
[[ "$(grep -c 'append_generated_app_params' "${action_file}")" == "2" ]] ||
    fail "expected both deploy paths to use generated parameter filtering"

echo "run-rad-commands deploy parameter tests passed"
