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
export RAD_CALLS="${TEST_ROOT}/rad-calls"
export UPLOAD_CALLS="${TEST_ROOT}/upload-calls"
touch "${RAD_CALLS}" "${UPLOAD_CALLS}" "${RADIUS_PROGRESS_UPLOADER}"

cat >"${TEST_ROOT}/bin/rad" <<'EOF'
#!/bin/bash
set -euo pipefail

printf '%s\n' "$*" >>"${RAD_CALLS}"
[[ "$*" == "resource list --preview --application todo --output json" ]]
[[ "${RAD_SHOULD_FAIL:-false}" != "true" ]]
cat "${RAD_RESPONSE_FILE}"
EOF

cat >"${RADIUS_PROGRESS_NODE}" <<'EOF'
#!/bin/bash
set -euo pipefail

shift
if [[ -n "${UPLOAD_BLOCK_DIR:-}" ]]; then
    mkdir "${UPLOAD_BLOCK_DIR}/active" 2>/dev/null ||
        touch "${UPLOAD_BLOCK_DIR}/overlap"
    touch "${UPLOAD_BLOCK_DIR}/started"
    while [[ ! -f "${UPLOAD_BLOCK_DIR}/release" ]]; do
        sleep 0.05
    done
    rmdir "${UPLOAD_BLOCK_DIR}/active" 2>/dev/null || true
    touch "${UPLOAD_BLOCK_DIR}/completed"
fi
printf '%s\n' "$*" >>"${UPLOAD_CALLS}"
printf '{"ok":true,"artifactId":42,"size":128}\n'
EOF
chmod +x "${TEST_ROOT}/bin/rad" "${RADIUS_PROGRESS_NODE}"
export PATH="${TEST_ROOT}/bin:${PATH}"

# shellcheck source=.github/extension/actions/deploy-progress/progress.sh
source "${SCRIPT_DIR}/progress.sh"

literal_app_file="${TEST_ROOT}/literal-app.bicep"
dynamic_app_file="${TEST_ROOT}/dynamic-app.bicep"
printf "resource app 'Radius.Core/applications@2025-08-01-preview' = { name: 'todo' }\n" \
    >"${literal_app_file}"
printf "resource app 'Radius.Core/applications@2025-08-01-preview' = { name: appName }\n" \
    >"${dynamic_app_file}"
[[ "$(radius_resolve_application_name "${literal_app_file}")" == "todo" ]] ||
    fail "literal application name was not resolved"
[[ -z "$(radius_resolve_application_name "${dynamic_app_file}")" ]] ||
    fail "dynamic application name must disable live progress"
[[ ! -s "${RAD_CALLS}" ]] ||
    fail "application name resolution must not call rad app list"

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
start_output=$(start_live_deploy_progress "${dynamic_app_file}" "dev")
[[ "${start_output}" == *"Could not determine application name"* ]] ||
    fail "dynamic application name must warn that live progress is disabled"
[[ -z "${RADIUS_PROGRESS_PID:-}" ]] ||
    fail "dynamic application name must not start live progress"
[[ ! -s "${RAD_CALLS}" ]] ||
    fail "disabled live progress must not call rad"

write_response() {
    local state="$1"
    jq -n --arg state "${state}" '[{
        id: "/resources/todo",
        name: "todo",
        type: "Radius.Compute/containers",
        properties: {
            provisioningState: $state,
            status: {
                message: "working",
                outputResources: [
                    {id: "/planes/kubernetes/local/providers/core/Service/todo"},
                    {id: "/planes/kubernetes/local/providers/apps/Deployment/todo"},
                    {id: "/planes/kubernetes/local/providers/core/Service/todo"},
                    {id: ""},
                    {}
                ]
            }
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
    and .resources[0].outputResourceIds == [
        "/planes/kubernetes/local/providers/apps/Deployment/todo",
        "/planes/kubernetes/local/providers/core/Service/todo"
    ]
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

rm -f "$(radius_progress_dir)/last-resources.json"
write_response "Stopping"
export UPLOAD_BLOCK_DIR="${TEST_ROOT}/blocked-upload"
export RADIUS_PROGRESS_INTERVAL_SECONDS=1
mkdir -p "${UPLOAD_BLOCK_DIR}"
start_live_deploy_progress "${literal_app_file}" "dev"
for _ in $(seq 1 100); do
    [[ -f "${UPLOAD_BLOCK_DIR}/started" ]] && break
    sleep 0.05
done
[[ -f "${UPLOAD_BLOCK_DIR}/started" ]] ||
    fail "blocked upload did not start"
(
    for _ in $(seq 1 100); do
        [[ -f "$(radius_progress_stop_file)" ]] && break
        sleep 0.05
    done
    [[ -f "$(radius_progress_stop_file)" ]] ||
        fail "poller stop was not requested"
    [[ -d "${UPLOAD_BLOCK_DIR}/active" ]] ||
        fail "stop must wait for the active upload"
    touch "${UPLOAD_BLOCK_DIR}/release"
) &
release_pid=$!
stop_live_deploy_progress
wait "${release_pid}"
[[ -f "${UPLOAD_BLOCK_DIR}/completed" ]] ||
    fail "stop returned before the active upload completed"
[[ ! -f "${UPLOAD_BLOCK_DIR}/overlap" ]] ||
    fail "final publish overlapped the active upload"
unset UPLOAD_BLOCK_DIR RADIUS_PROGRESS_INTERVAL_SECONDS

printf '%s\n' '[{
    "id": "/resources/no-outputs",
    "name": "no-outputs",
    "type": "Radius.Core/example",
    "properties": {"provisioningState": "Succeeded"}
}]' >"${RAD_RESPONSE_FILE}"
rm -f "$(radius_progress_dir)/last-resources.json"
radius_publish_live_progress_once "todo" "dev" \
    "radius-deploy-status-dev-todo"
jq -e '.resources[0].outputResourceIds == []' "${progress_file}" \
    >/dev/null || fail "resources without resolved outputs need an empty ID array"

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
