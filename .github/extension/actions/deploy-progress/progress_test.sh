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

mkdir -p "${TEST_ROOT}/bin" "${TEST_ROOT}/runner"
export RUNNER_TEMP="${TEST_ROOT}/runner"
export GITHUB_RUN_ID=12345
export RADIUS_PROGRESS_NODE="${TEST_ROOT}/bin/node"
export RADIUS_PROGRESS_UPLOADER="${TEST_ROOT}/uploader.js"
export RAD_RESPONSE_FILE="${TEST_ROOT}/rad-response.json"
export UPLOAD_CALLS="${TEST_ROOT}/upload-calls"
touch "${UPLOAD_CALLS}" "${RADIUS_PROGRESS_UPLOADER}"

cat >"${TEST_ROOT}/bin/rad" <<'EOF'
#!/bin/bash
set -euo pipefail

[[ "$*" == "resource list --preview --application todo --output json" ]]
[[ "${RAD_SHOULD_FAIL:-false}" != "true" ]]
cat "${RAD_RESPONSE_FILE}"
EOF

cat >"${RADIUS_PROGRESS_NODE}" <<'EOF'
#!/bin/bash
set -euo pipefail

shift
printf '%s\n' "$*" >>"${UPLOAD_CALLS}"
printf '{"ok":true,"artifactId":42,"size":128}\n'
EOF
chmod +x "${TEST_ROOT}/bin/rad" "${RADIUS_PROGRESS_NODE}"
export PATH="${TEST_ROOT}/bin:${PATH}"

# shellcheck source=.github/extension/actions/deploy-progress/progress.sh
source "${SCRIPT_DIR}/progress.sh"

runtime_file=$(radius_artifact_runtime_file)
mkdir -p "$(radius_progress_dir)"
jq -n \
    --arg runtimeToken "runtime-token" \
    --arg resultsUrl "https://results.example.test/path" \
    '{runtimeToken: $runtimeToken, resultsUrl: $resultsUrl}' >"${runtime_file}"
radius_load_artifact_runtime
[[ "${ACTIONS_RUNTIME_TOKEN}" == "runtime-token" ]] ||
    fail "artifact runtime token was not loaded"
[[ "${ACTIONS_RESULTS_URL}" == "https://results.example.test/path" ]] ||
    fail "artifact results URL was not loaded"

write_response() {
    local state="$1"
    jq -n --arg state "${state}" '[{
        id: "/resources/todo",
        name: "todo",
        type: "Radius.Compute/containers",
        properties: {
            provisioningState: $state,
            status: {message: "working"}
        }
    }]' >"${RAD_RESPONSE_FILE}"
}

write_response "Provisioning"
radius_publish_live_progress_once "todo" "dev" \
    "radius-deploy-status-dev-todo"

progress_file="$(radius_progress_dir)/deploy-progress.json"
jq -e '
    .schemaVersion == 1
    and .runId == 12345
    and .sequence == 1
    and .state == "in_progress"
    and .resources[0].status == "in_progress"
' "${progress_file}" >/dev/null || fail "invalid first progress payload"
grep -q \
    'radius-deploy-status-dev-todo-live-12345-slot-0 .* 1 false' \
    "${UPLOAD_CALLS}" || fail "first snapshot must use slot 0"

radius_publish_live_progress_once "todo" "dev" \
    "radius-deploy-status-dev-todo"
[[ "$(wc -l <"${UPLOAD_CALLS}" | tr -d ' ')" == "1" ]] ||
    fail "unchanged resources must not upload"

for sequence in $(seq 2 9); do
    write_response "State${sequence}"
    radius_publish_live_progress_once "todo" "dev" \
        "radius-deploy-status-dev-todo"
done

[[ "$(radius_last_live_sequence)" == "9" ]] ||
    fail "expected sequence checkpoint 9"
[[ "$(radius_next_terminal_sequence)" == "10" ]] ||
    fail "expected terminal sequence handoff 10"
tail -1 "${UPLOAD_CALLS}" |
    grep -q 'radius-deploy-status-dev-todo-live-12345-slot-0 .* 1 true' ||
    fail "ninth snapshot must replace slot 0"

printf 'not-json\n' >"${RAD_RESPONSE_FILE}"
radius_publish_live_progress_once "todo" "dev" \
    "radius-deploy-status-dev-todo"
[[ "$(radius_last_live_sequence)" == "9" ]] ||
    fail "malformed JSON must not advance sequence"

export RAD_SHOULD_FAIL=true
radius_publish_live_progress_once "todo" "dev" \
    "radius-deploy-status-dev-todo"
unset RAD_SHOULD_FAIL
[[ "$(radius_last_live_sequence)" == "9" ]] ||
    fail "poll failure must not advance sequence"

[[ "$(radius_deploy_artifact_name 'Dev Env' 'My App')" == \
    "radius-deploy-status-dev-env-my-app" ]] ||
    fail "artifact name sanitization changed"

radius_clear_artifact_runtime
[[ ! -f "${runtime_file}" ]] || fail "artifact runtime file was not removed"
[[ -z "${ACTIONS_RUNTIME_TOKEN:-}" ]] ||
    fail "artifact runtime token was not cleared"
[[ -z "${ACTIONS_RESULTS_URL:-}" ]] ||
    fail "artifact results URL was not cleared"

repo_root=$(cd "${SCRIPT_DIR}/../../../.." && pwd)
for workflow in \
    "${repo_root}/.github/extension/run-rad-commands-aws.yml" \
    "${repo_root}/.github/extension/run-rad-commands-azure.yml"; do
    grep -qF \
        'actions/deploy-progress/artifact-uploader@{{RADIUS_REF}}' \
        "${workflow}" || fail "$(basename "${workflow}") does not prepare the artifact runtime"
done

echo "deploy progress tests passed"
