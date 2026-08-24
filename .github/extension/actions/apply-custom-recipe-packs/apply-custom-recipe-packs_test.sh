#!/bin/bash

# Unit tests for the apply-custom-recipe-packs composite action. The test
# extracts and executes the action's real run body with stubbed CLIs.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly ACTION_FILE="${SCRIPT_DIR}/action.yml"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

readonly BODY_SCRIPT="${TEST_ROOT}/action-body.sh"
readonly STUB_BIN="${TEST_ROOT}/bin"
readonly BICEP_STUB="${STUB_BIN}/bicep"
readonly RAD_STUB="${STUB_BIN}/rad"
readonly RAD_CALLS="${TEST_ROOT}/rad-calls.log"
readonly ACTION_LOG="${TEST_ROOT}/action.log"
readonly APP_DIR="${TEST_ROOT}/app"
readonly APP_FILE="${APP_DIR}/app.bicep"
readonly RECIPE_PACK_BICEP="${APP_DIR}/custom-recipe-pack.bicep"
readonly CUSTOM_TYPES_YAML="${APP_DIR}/custom-types.yaml"
readonly COMPILED_FIXTURE="${TEST_ROOT}/compiled.json"
readonly ENVIRONMENT="test-environment"
readonly PACK_SCOPE="/planes/radius/local/resourcegroups/default"
readonly PACK_ID_PREFIX="${PACK_SCOPE}/providers/Radius.Core/recipePacks"
readonly DEFAULT_PACK_ID="${PACK_ID_PREFIX}/default"
readonly CUSTOM_PACK_ID="${PACK_ID_PREFIX}/custom"
readonly OTHER_PACK_ID="${PACK_ID_PREFIX}/other"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

for command in jq python3; do
    command -v "${command}" >/dev/null 2>&1 ||
        fail "${command} is required"
done

extract_action_body() {
    python3 - "${ACTION_FILE}" "${BODY_SCRIPT}" <<'PYTHON'
import re
import sys

action_file, out_file = sys.argv[1], sys.argv[2]
lines = open(action_file, encoding="utf-8").read().splitlines()
starts = [i for i, line in enumerate(lines)
          if re.match(r"^\s*run:\s*\|\s*$", line)]
if len(starts) != 1:
    sys.exit("expected exactly one run block, found %d" % len(starts))

body, base = [], None
for line in lines[starts[0] + 1:]:
    if not line.strip():
        body.append("")
        continue
    indent = len(line) - len(line.lstrip(" "))
    if base is None:
        base = indent
    if indent < base:
        break
    body.append(line[base:])

while body and not body[-1]:
    body.pop()
if not body:
    sys.exit("extracted an empty run block")

open(out_file, "w", encoding="utf-8").write("\n".join(body) + "\n")
PYTHON
}

write_stubs() {
    mkdir -p "${STUB_BIN}"

    cat >"${BICEP_STUB}" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "${BICEP_SHOULD_FAIL:-false}" == "true" ]]; then
    echo "synthetic Bicep failure" >&2
    exit 1
fi
[[ "$1" == "build" && "$3" == "--outfile" ]]
cp "${COMPILED_FIXTURE}" "$4"
EOF

    cat >"${RAD_STUB}" <<'EOF'
#!/bin/bash
set -euo pipefail

printf '%s\n' "$*" >>"${RAD_CALLS}"
case "${1:-} ${2:-}" in
    "resource-type create")
        [[ "${RAD_TYPE_CREATE_SHOULD_FAIL:-false}" != "true" ]]
        ;;
    "deploy "*)
        if [[ "${RAD_DEPLOY_SHOULD_FAIL:-false}" == "true" ]]; then
            echo "synthetic deploy failure" >&2
            exit 1
        fi
        ;;
    "recipe-pack show")
        if [[ "${RAD_SHOW_SHOULD_FAIL:-false}" == "true" ]]; then
            echo "synthetic recipe-pack lookup failure" >&2
            exit 1
        fi
        if [[ "${RAD_SHOW_EMPTY_ID:-false}" == "true" ]]; then
            printf '%s\n' '{"id":""}'
        else
            jq -nc --arg name "$3" \
                '{id:("/planes/radius/local/resourcegroups/default/providers/Radius.Core/recipePacks/" + $name)}'
        fi
        ;;
    "recipe-pack list")
        echo "global recipe-pack listing must not be used" >&2
        exit 97
        ;;
    "env show")
        if [[ "${RAD_ENV_SHOW_SHOULD_FAIL:-false}" == "true" ]]; then
            echo "synthetic environment lookup failure" >&2
            exit 1
        fi
        printf '%s\n' "${ENV_RESOURCE_JSON}"
        if [[ -n "${ENV_TRAILING_JSON:-}" ]]; then
            printf '%s\n' "${ENV_TRAILING_JSON}"
        fi
        ;;
    "env update")
        if [[ "${RAD_ENV_UPDATE_SHOULD_FAIL:-false}" == "true" ]]; then
            echo "synthetic environment update failure" >&2
            exit 1
        fi
        ;;
    *)
        echo "unexpected rad command: $*" >&2
        exit 98
        ;;
esac
EOF

    chmod +x "${BICEP_STUB}" "${RAD_STUB}"
}

write_compiled_template() {
    local resources="$1"
    jq -nc --argjson resources "${resources}" \
        '{resources:$resources}' >"${COMPILED_FIXTURE}"
}

single_pack_template() {
    write_compiled_template '{
      "custom": {
        "import": "radius",
        "type": "Radius.Core/recipePacks@2025-08-01-preview",
        "properties": { "name": "custom", "properties": { "recipes": {} } }
      }
    }'
}

reset_case() {
    mkdir -p "${APP_DIR}"
    touch "${APP_FILE}" "${RECIPE_PACK_BICEP}"
    rm -f "${CUSTOM_TYPES_YAML}"
    : >"${RAD_CALLS}"
    single_pack_template

    ENV_RESOURCE_JSON="$(
        jq -nc --arg pack "${DEFAULT_PACK_ID}" \
            '{properties:{recipePacks:[$pack]}}'
    )"
    ENV_TRAILING_JSON=""
    BICEP_SHOULD_FAIL=false
    RAD_TYPE_CREATE_SHOULD_FAIL=false
    RAD_DEPLOY_SHOULD_FAIL=false
    RAD_SHOW_SHOULD_FAIL=false
    RAD_SHOW_EMPTY_ID=false
    RAD_ENV_SHOW_SHOULD_FAIL=false
    RAD_ENV_UPDATE_SHOULD_FAIL=false
    export ENV_RESOURCE_JSON ENV_TRAILING_JSON BICEP_SHOULD_FAIL
    export RAD_TYPE_CREATE_SHOULD_FAIL RAD_DEPLOY_SHOULD_FAIL
    export RAD_SHOW_SHOULD_FAIL RAD_SHOW_EMPTY_ID
    export RAD_ENV_SHOW_SHOULD_FAIL RAD_ENV_UPDATE_SHOULD_FAIL
}

ACTION_EXIT=0

run_action() {
    set +e
    (
        export APP_FILE BICEP_BIN="${BICEP_STUB}" COMPILED_FIXTURE
        export ENVIRONMENT PATH="${STUB_BIN}:${PATH}" RAD_CALLS
        TMPDIR="${TEST_ROOT}"
        export TMPDIR
        bash "${BODY_SCRIPT}"
    ) >"${ACTION_LOG}" 2>&1
    ACTION_EXIT=$?
    set -e
}

assert_success() {
    ((ACTION_EXIT == 0)) ||
        fail "$1: expected success, got ${ACTION_EXIT}
$(cat "${ACTION_LOG}")"
}

assert_failure() {
    ((ACTION_EXIT != 0)) ||
        fail "$1: expected failure
$(cat "${ACTION_LOG}")"
}

assert_call_present() {
    grep -qF -- "$1" "${RAD_CALLS}" ||
        fail "expected rad call containing '$1'
$(cat "${RAD_CALLS}")"
}

assert_call_absent() {
    if grep -qF -- "$1" "${RAD_CALLS}"; then
        fail "did not expect rad call containing '$1'
$(cat "${RAD_CALLS}")"
    fi
}

assert_update_packs() {
    local expected="$1"
    local actual
    actual="$(
        sed -n 's/.*--recipe-packs \([^ ]*\).*/\1/p' "${RAD_CALLS}" |
            tail -n 1
    )"
    [[ "${actual}" == "${expected}" ]] ||
        fail "expected update packs '${expected}', got '${actual}'"
}

extract_action_body
write_stubs

# First deploy attaches the authored pack while preserving the provider pack.
reset_case
run_action
assert_success "first deploy"
assert_call_present "recipe-pack show custom -o json"
assert_call_absent "recipe-pack list"
assert_update_packs "${CUSTOM_PACK_ID},${DEFAULT_PACK_ID}"

# A restored-state redeploy resolves the same authored identity even though the
# deploy is idempotent and no globally new resource exists.
reset_case
run_action
assert_success "repeat deploy after state restore"
assert_update_packs "${CUSTOM_PACK_ID},${DEFAULT_PACK_ID}"

# Packs already attached for another purpose remain attached. Globally known
# packs are irrelevant because the action never lists them.
reset_case
ENV_RESOURCE_JSON="$(
    jq -nc \
        --arg default "${DEFAULT_PACK_ID}" \
        --arg other "${OTHER_PACK_ID}" \
        '{properties:{recipePacks:[$default,$other]}}'
)"
export ENV_RESOURCE_JSON
run_action
assert_success "preserve unrelated attached pack"
assert_update_packs \
    "${CUSTOM_PACK_ID},${DEFAULT_PACK_ID},${OTHER_PACK_ID}"

# Every authored recipe-pack resource is resolved and attached.
reset_case
write_compiled_template '{
  "custom": {
    "import": "radius",
    "type": "Radius.Core/recipePacks@2025-08-01-preview",
    "properties": { "name": "custom", "properties": { "recipes": {} } }
  },
  "other": {
    "import": "radius",
    "type": "radius.core/recipepacks@2025-08-01-preview",
    "properties": { "name": "other", "properties": { "recipes": {} } }
  }
}'
run_action
assert_success "multiple authored packs"
assert_call_present "recipe-pack show custom -o json"
assert_call_present "recipe-pack show other -o json"
assert_update_packs \
    "${CUSTOM_PACK_ID},${DEFAULT_PACK_ID},${OTHER_PACK_ID}"

# Existing recipe-pack resources are references, not packs authored by this
# template, so they are not resolved or attached.
reset_case
write_compiled_template '{
  "shared": {
    "import": "radius",
    "existing": true,
    "type": "Radius.Core/recipePacks@2025-08-01-preview",
    "properties": { "name": "shared" }
  },
  "custom": {
    "import": "radius",
    "type": "Radius.Core/recipePacks@2025-08-01-preview",
    "properties": { "name": "custom", "properties": { "recipes": {} } }
  }
}'
run_action
assert_success "ignore existing recipe-pack reference"
assert_call_absent "recipe-pack show shared -o json"
assert_call_present "recipe-pack show custom -o json"
assert_update_packs "${CUSTOM_PACK_ID},${DEFAULT_PACK_ID}"

# Legacy array-shaped bicep output (languageVersion 1.x) with a top-level name
# is discovered the same way as symbolic-name object output.
reset_case
write_compiled_template '[
  {
    "type": "Radius.Core/recipePacks@2025-08-01-preview",
    "name": "custom",
    "properties": { "recipes": {} }
  }
]'
run_action
assert_success "array-shaped compiled template"
assert_call_present "recipe-pack show custom -o json"
assert_update_packs "${CUSTOM_PACK_ID},${DEFAULT_PACK_ID}"
reset_case
rm -f "${RECIPE_PACK_BICEP}"
touch "${CUSTOM_TYPES_YAML}"
run_action
assert_success "custom types without recipe pack"
assert_call_present "resource-type create --from-file ${CUSTOM_TYPES_YAML}"
assert_call_absent "deploy "
assert_call_absent "env update"

reset_case
rm -f "${RECIPE_PACK_BICEP}" "${CUSTOM_TYPES_YAML}"
run_action
assert_success "no custom artifacts"
[[ ! -s "${RAD_CALLS}" ]] ||
    fail "expected no rad calls when custom artifacts are absent"

# Empty and dynamic pack identities fail before deployment.
reset_case
write_compiled_template '{}'
run_action
assert_failure "empty pack set"
assert_call_absent "deploy "

reset_case
write_compiled_template '{
  "custom": {
    "import": "radius",
    "type": "Radius.Core/recipePacks@2025-08-01-preview",
    "properties": {
      "name": "[parameters(\"packName\")]",
      "properties": { "recipes": {} }
    }
  }
}'
run_action
assert_failure "dynamic pack name"
assert_call_absent "deploy "

# A conditional recipe-pack resource cannot be resolved deterministically: a
# runtime condition may skip it, and a same-named pack in restored state would
# otherwise be attached. Reject it before deploying.
reset_case
write_compiled_template '{
  "custom": {
    "import": "radius",
    "type": "Radius.Core/recipePacks@2025-08-01-preview",
    "condition": "[parameters(\"enablePack\")]",
    "properties": { "name": "custom", "properties": { "recipes": {} } }
  }
}'
run_action
assert_failure "conditional pack resource"
assert_call_absent "deploy "

# Command and output failures propagate instead of producing a replacement
# environment update with incomplete data.
reset_case
touch "${CUSTOM_TYPES_YAML}"
RAD_TYPE_CREATE_SHOULD_FAIL=true
export RAD_TYPE_CREATE_SHOULD_FAIL
run_action
assert_failure "custom type registration failure"
assert_call_absent "deploy "

reset_case
BICEP_SHOULD_FAIL=true
export BICEP_SHOULD_FAIL
run_action
assert_failure "Bicep compile failure"
assert_call_absent "deploy "

reset_case
RAD_DEPLOY_SHOULD_FAIL=true
export RAD_DEPLOY_SHOULD_FAIL
run_action
assert_failure "recipe-pack deploy failure"
assert_call_absent "env update"

reset_case
RAD_SHOW_SHOULD_FAIL=true
export RAD_SHOW_SHOULD_FAIL
run_action
assert_failure "recipe-pack lookup failure"
assert_call_absent "env update"

reset_case
RAD_SHOW_EMPTY_ID=true
export RAD_SHOW_EMPTY_ID
run_action
assert_failure "empty recipe-pack ID"
assert_call_absent "env update"

reset_case
RAD_ENV_SHOW_SHOULD_FAIL=true
export RAD_ENV_SHOW_SHOULD_FAIL
run_action
assert_failure "environment lookup failure"
assert_call_absent "env update"

reset_case
RAD_ENV_UPDATE_SHOULD_FAIL=true
export RAD_ENV_UPDATE_SHOULD_FAIL
run_action
assert_failure "environment update failure"

echo "apply-custom-recipe-packs action tests passed"
