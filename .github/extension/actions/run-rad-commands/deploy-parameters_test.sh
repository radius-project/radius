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

if ! command -v jq >/dev/null 2>&1; then
    fail "jq is required; run 'make install-jq'"
fi
REAL_JQ="$(command -v jq)"
readonly REAL_JQ

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

# Architecture-aware build platforms: injected only when the app declares a
# `platforms` parameter and the deploy computed an effective platform list.
RADIUS_EFFECTIVE_BUILD_PLATFORMS="linux/amd64"

run_generated_params_case '{"platforms":{}}'
assert_param_present "platforms=linux/amd64"

# App does not declare `platforms`: nothing injected.
run_generated_params_case '{"environment":{}}'
assert_param_absent "platforms=linux/amd64"

# No effective platforms computed (feature off/undetermined): nothing injected.
RADIUS_EFFECTIVE_BUILD_PLATFORMS=""
run_generated_params_case '{"platforms":{}}'
assert_param_absent "platforms=linux/amd64"

# A value supplied via RADIUS_DEPLOY_PARAMS is not overridden by the computed one.
RADIUS_EFFECTIVE_BUILD_PLATFORMS="linux/amd64"
DEPLOY_PARAMS_JSON='{"platforms":"linux/arm64"}'
run_generated_params_case '{"platforms":{}}'
assert_param_absent "platforms=linux/amd64"
DEPLOY_PARAMS_JSON=""
RADIUS_EFFECTIVE_BUILD_PLATFORMS=""

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

# A missing application file must fast-fail before the Bicep compiler is invoked.
reset_discovery
missing_app_file="${TEST_ROOT}/missing-app.bicep"
rm -f "${missing_app_file}"
if load_declared_app_params "${missing_app_file}" 2>"${TEST_ROOT}/missing-app.err"; then
    fail "expected a missing application file to stop parameter discovery"
fi
[[ "$(wc -l <"${BICEP_CALLS}" | tr -d ' ')" == "0" ]] ||
    fail "a missing application file must not invoke the Bicep compiler"
grep -q "not found on this branch/commit" "${TEST_ROOT}/missing-app.err" ||
    fail "expected a helpful not-found error for a missing application file"
[[ "${DECLARED_APP_PARAMS_LOADED}" == "false" ]] ||
    fail "a missing application file must not populate the parameter cache"

# A missing-file path containing workflow-command metacharacters must be escaped
# in the ::error:: annotation so it cannot break or inject the workflow command.
reset_discovery
injecting_app_file="${TEST_ROOT}/pct%evil.bicep"
rm -f "${injecting_app_file}"
if load_declared_app_params "${injecting_app_file}" 2>"${TEST_ROOT}/inject.err"; then
    fail "expected a missing application file to stop parameter discovery"
fi
grep -q 'pct%25evil.bicep' "${TEST_ROOT}/inject.err" ||
    fail "expected '%' in the app-file path to be escaped as %25 in the error"
grep -q 'pct%evil.bicep' "${TEST_ROOT}/inject.err" &&
    fail "expected the raw '%' app-file path to not appear unescaped in the error"

fake_jq_dir="${TEST_ROOT}/fake-jq"
mkdir "${fake_jq_dir}"
cat >"${fake_jq_dir}/jq" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$*" == *'(.parameters // {}) | keys[]'* ]]; then
    echo "synthetic parameter extraction failure" >&2
    exit 1
fi
exec "${REAL_JQ}" "$@"
EOF
chmod +x "${fake_jq_dir}/jq"
export REAL_JQ

reset_discovery
write_template '{"image":{}}'
original_path="${PATH}"
PATH="${fake_jq_dir}:${PATH}"
if load_declared_app_params "${APP_FILE}"; then
    PATH="${original_path}"
    fail "expected parameter extraction failure to stop discovery"
fi
PATH="${original_path}"
[[ "${DECLARED_APP_PARAMS_LOADED}" == "false" ]] ||
    fail "failed parameter extraction must not populate the parameter cache"

action_file="${SCRIPT_DIR}/action.yml"
[[ "$(grep -c 'append_generated_app_params' "${action_file}")" == "2" ]] ||
    fail "expected both deploy paths to use generated parameter filtering"

# The "Verify app namespace" step must tolerate a missing app file so it cannot
# pre-empt the actionable not-found error raised in Run rad commands. Guard
# against regression by requiring the grep there to use the `|| true` form.
grep -qE "grep -oP .*head -1 \|\| true" "${action_file}" ||
    fail "expected the app-name grep in action.yml to tolerate a missing file via '|| true'"

echo "run-rad-commands deploy parameter tests passed"
