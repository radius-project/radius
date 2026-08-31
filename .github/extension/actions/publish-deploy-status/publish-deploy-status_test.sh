#!/bin/bash

# ============================================================================
# Unit tests for the publish-deploy-status composite action.
#
# The generating logic lives inline in the `run:` block of action.yml, so the
# test extracts that block and executes it directly. Testing the extracted real
# body rather than a copy means the test cannot drift from the action.
#
# The action is two composite steps: a `run:` step that writes the status files
# and sets step outputs, and an actions/upload-artifact step gated on those
# outputs. Only the first is executable here, so the expressions wiring the two
# together are checked structurally against the parsed YAML instead - nothing
# else covers them.
#
# `rad` is stubbed. There is no runner, no network and no artifact upload:
# RUNNER_TEMP, GITHUB_OUTPUT and TMPDIR are all redirected into a sandbox.
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly ACTION_FILE="${SCRIPT_DIR}/action.yml"
readonly PROGRESS_HELPER="${SCRIPT_DIR}/../deploy-progress/progress.sh"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
# Where the action lands when RUNNER_TEMP is unset. Fixed by the action, so it
# cannot be sandboxed; it is the action's own scratch directory and is removed
# on exit.
readonly FALLBACK_STATUS_DIR="/tmp/radius-deploy-status"
trap 'rm -rf "${TEST_ROOT}" "${FALLBACK_STATUS_DIR}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if ! command -v jq >/dev/null 2>&1; then
    fail "jq is required; run 'make install-jq'"
fi
if ! command -v python3 >/dev/null 2>&1; then
    fail "python3 is required to parse action.yml"
fi

readonly BODY_SCRIPT="${TEST_ROOT}/publisher-body.sh"
readonly STEP_FACTS="${TEST_ROOT}/step-facts.txt"
readonly STUB_BIN="${TEST_ROOT}/bin"
readonly PUBLISHER_LOG="${TEST_ROOT}/publisher.log"
readonly PUBLISHER_TMP="${TEST_ROOT}/tmp"
readonly RUNNER_TEMP="${TEST_ROOT}/runner-temp"
readonly GITHUB_OUTPUT="${TEST_ROOT}/github-output.txt"
readonly STATUS_DIR="${RUNNER_TEMP}/radius-deploy-status"
readonly RESULT_FILE="${TEST_ROOT}/rad-commands-result.json"
readonly LITERAL_APP_FILE="${TEST_ROOT}/app.bicep"
readonly NO_LITERAL_APP_FILE="${TEST_ROOT}/no-literal.bicep"
readonly UNICODE_APP_FILE="${TEST_ROOT}/unicode.bicep"
readonly MESSY_APP_FILE="${TEST_ROOT}/messy-name.bicep"

# ---------------------------------------------------------------------------
# Extract the run: block from action.yml.
#
# Deliberately stdlib-only: CI provisions a bare interpreter via
# actions/setup-python, where PyYAML is not guaranteed to be present. The block
# is a literal scalar, so dedenting its lines reproduces it exactly.
# ---------------------------------------------------------------------------
extract_action_body() {
    python3 - "${ACTION_FILE}" "${BODY_SCRIPT}" <<'PYTHON'
import re
import sys

action_file, out_file = sys.argv[1], sys.argv[2]
lines = open(action_file, encoding="utf-8").read().splitlines()

starts = [i for i, l in enumerate(lines) if re.match(r"^\s*run:\s*\|\s*$", l)]
if len(starts) != 1:
    sys.exit("expected exactly one 'run: |' block, found %d" % len(starts))

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
    sys.exit("extracted an empty run: block")

open(out_file, "w", encoding="utf-8").write("\n".join(body) + "\n")
PYTHON
}

# ---------------------------------------------------------------------------
# Flatten the parts of action.yml that the executable body cannot cover: the
# step ids and the three `${{ steps.generate.outputs.* }}` expressions that wire
# the generate step to the upload step. Emitted as `key=value` lines.
# ---------------------------------------------------------------------------
extract_step_facts() {
    python3 - "${ACTION_FILE}" "${STEP_FACTS}" <<'PYTHON'
import re
import sys

action_file, out_file = sys.argv[1], sys.argv[2]
lines = open(action_file, encoding="utf-8").read().splitlines()

KEY = re.compile(r"^\s*([A-Za-z0-9_.-]+):\s*(.*)$")


def flatten(block):
    """Map a YAML mapping's scalar leaves to dotted key paths."""
    base = min((len(l) - len(l.lstrip(" ")) for l in block if l.strip()),
               default=0)
    result, stack, block_scalar_at = {}, [], None
    for line in block:
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        indent = len(line) - len(line.lstrip(" ")) - base
        # Skip the contents of a literal block scalar: the shell inside `run: |`
        # contains `key: value` lines (jq filters, for one) that are not YAML.
        if block_scalar_at is not None:
            if indent > block_scalar_at:
                continue
            block_scalar_at = None
        matched = KEY.match(line)
        if not matched:
            continue
        key, value = matched.group(1), matched.group(2).strip()
        while stack and stack[-1][0] >= indent:
            stack.pop()
        path = ".".join([k for _, k in stack] + [key])
        if value in ("|", ">", "|-", ">-", "|+", ">+"):
            result[path] = value
            block_scalar_at = indent
        elif value:
            result[path] = value
        else:
            stack.append((indent, key))
    return result


steps_at = [i for i, l in enumerate(lines) if re.match(r"^\s*steps:\s*$", l)]
if len(steps_at) != 1:
    sys.exit("expected exactly one 'steps:' block")

blocks, item_indent = [], None
for line in lines[steps_at[0] + 1:]:
    if not line.strip() or line.lstrip().startswith("#"):
        continue
    indent = len(line) - len(line.lstrip(" "))
    if re.match(r"^\s*-\s", line) and indent == (item_indent or indent):
        item_indent = indent
        blocks.append([re.sub(r"^(\s*)-\s", r"\1 ", line)])
        continue
    if item_indent is None or indent <= item_indent:
        break
    blocks[-1].append(line)

steps = [flatten(b) for b in blocks]
if len(steps) != 2:
    sys.exit("expected exactly two composite steps, found %d" % len(steps))

generate, upload = steps[0], steps[1]
if not upload.get("uses", "").startswith("actions/upload-artifact@"):
    sys.exit("expected the second step to use actions/upload-artifact")

facts = {
    "generate.id": generate.get("id", ""),
    "generate.shell": generate.get("shell", ""),
    "upload.continue-on-error": upload.get("continue-on-error", ""),
    "upload.if": upload.get("if", ""),
    "upload.uses": upload.get("uses", ""),
    "upload.with.name": upload.get("with.name", ""),
    "upload.with.path": upload.get("with.path", ""),
    "outputs.artifact-name.value": flatten(lines).get(
        "outputs.artifact-name.value", ""),
}
with open(out_file, "w", encoding="utf-8") as handle:
    for key in sorted(facts):
        handle.write("%s=%s\n" % (key, facts[key]))
PYTHON
}

# The action hardcodes RESULT_FILE under /tmp. Redirect it into the test's
# sandbox so the test neither reads nor clobbers a real deploy's output. The
# substitution is asserted so a rename in action.yml fails loudly here instead
# of silently testing a path that no longer exists.
redirect_result_file() {
    local before after
    before="$(grep -c '^RESULT_FILE=' "${BODY_SCRIPT}")"
    [[ "${before}" == "1" ]] ||
        fail "expected one RESULT_FILE assignment, found ${before}"

    sed -i -E "s|^RESULT_FILE=.*|RESULT_FILE=${RESULT_FILE}|" "${BODY_SCRIPT}"

    after="$(grep -c "^RESULT_FILE=${RESULT_FILE}\$" "${BODY_SCRIPT}")"
    [[ "${after}" == "1" ]] ||
        fail "failed to redirect RESULT_FILE into the test sandbox"
}

write_rad_stub() {
    mkdir -p "${STUB_BIN}"
    cat >"${STUB_BIN}/rad" <<'EOF'
#!/bin/bash
set -uo pipefail

case "${1:-} ${2:-}" in
    "app graph")
        if [[ "${3:-}" != "${APP_FILE:-}" ]]; then
            echo "rad: app graph must receive APP_FILE as its positional argument" >&2
            exit 64
        fi
        if [[ "${RAD_GRAPH_SHOULD_FAIL:-false}" == "true" ]]; then
            echo "rad: application not found in environment" >&2
            exit 1
        fi
        printf '%s\n' \
            '{"resources":[{"id":"/planes/radius/local/rg/web","name":"web","type":"Radius.Compute/containers","outputResources":[{"id":"/planes/kubernetes/local/providers/apps/Deployment/web","name":"web","type":"apps/Deployment"},{"id":"/planes/kubernetes/local/providers/core/Service/web","name":"web","type":"core/Service"}]},{"id":"/planes/radius/local/rg/db","name":"db","type":"Radius.Data/mySqlDatabases","outputResources":[{"id":"/subscriptions/test/providers/Microsoft.DBforMySQL/flexibleServers/db","name":"db","type":"Microsoft.DBforMySQL/flexibleServers"}]}]}'
        ;;
    "app list")
        printf '[{"name":"%s"}]\n' "${RAD_APP_LIST_NAME-fallback-app}"
        ;;
    "resource list")
        if [[ " $* " != *" --preview "* ]]; then
            echo "rad: resource list must use --preview" >&2
            exit 64
        fi
        if [[ "${RAD_RESOURCE_LIST_SHOULD_FAIL:-false}" == "true" ]]; then
            echo "rad: could not reach the control plane" >&2
            exit 1
        fi
        # Shaped like []generated.GenericResource, which is what
        # `rad resource list -o json` marshals. The three provisioning states
        # cover each branch of the status normalization in the action.
        printf '%s\n' '[
          {"id":"/planes/radius/local/rg/web","name":"web",
           "type":"Radius.Compute/containers",
           "properties":{"provisioningState":"Succeeded",
                         "status":{"message":"container ready",
                           "outputResources":[
                             {"id":"/planes/kubernetes/local/providers/core/Service/web"},
                             {"id":"/planes/kubernetes/local/providers/apps/Deployment/web"},
                             {"id":"/planes/kubernetes/local/providers/core/Service/web"}
                           ]}}},
          {"id":"/planes/radius/local/rg/db","name":"db",
           "type":"Radius.Data/mySqlDatabases",
           "properties":{"provisioningState":"Failed",
                         "status":{"message":"recipe execution failed",
                           "outputResources":[
                             {"id":"/subscriptions/test/providers/Microsoft.DBforMySQL/flexibleServers/db"}
                           ]}}},
          {"id":"/planes/radius/local/rg/queue","name":"queue",
           "type":"Radius.Messaging/rabbitMQQueues",
           "properties":{"provisioningState":"Updating"}}
        ]'
        ;;
    "version "*)
        echo "RELEASE VERSION 0.99.0-test"
        ;;
    "env list")
        if [[ " $* " != *" --preview "* ]]; then
            echo "rad: env list must use --preview" >&2
            exit 64
        fi
        printf '%s\n' '[{"name":"aks-dev"}]'
        ;;
    *)
        printf '{}\n'
        ;;
esac
EOF
    chmod +x "${STUB_BIN}/rad"
}

# ---------------------------------------------------------------------------
# Harness
# ---------------------------------------------------------------------------
PUBLISHER_EXIT=0
PUBLISHER_LOCALE=""
PUBLISHER_WITHOUT_RUNNER_TEMP=false

run_publisher() {
    rm -rf "${PUBLISHER_TMP}"
    # "preserve" leaves an existing STATUS_DIR in place to prove the action
    # rebuilds it rather than republishing a previous run's files.
    [[ "${1:-}" == "preserve" ]] || rm -rf "${RUNNER_TEMP}"
    mkdir -p "${RUNNER_TEMP}" "${PUBLISHER_TMP}"
    : >"${GITHUB_OUTPUT}"

    set +e
    (
        PATH="${STUB_BIN}:${PATH}"
        # Keep the action's `mktemp` inside the sandbox.
        TMPDIR="${PUBLISHER_TMP}"
        GITHUB_ACTION_PATH="${SCRIPT_DIR}"
        export PATH TMPDIR GITHUB_OUTPUT GITHUB_ACTION_PATH
        if [[ -n "${PUBLISHER_LOCALE}" ]]; then
            LC_ALL="${PUBLISHER_LOCALE}"
            LANG="${PUBLISHER_LOCALE}"
            export LC_ALL LANG
        fi
        if [[ "${PUBLISHER_WITHOUT_RUNNER_TEMP}" == "true" ]]; then
            # Not exporting RUNNER_TEMP is NOT enough to hide it. On a GitHub
            # runner RUNNER_TEMP is already an exported environment variable, and
            # assigning to an exported name keeps the export attribute - so the
            # sandbox value would reach the step anyway and the fallback would
            # never be exercised. This passed locally and failed in CI for exactly
            # that reason. Remove it from the child's environment explicitly.
            env -u RUNNER_TEMP bash "${BODY_SCRIPT}"
        else
            export RUNNER_TEMP
            bash "${BODY_SCRIPT}"
        fi
    ) >"${PUBLISHER_LOG}" 2>&1
    PUBLISHER_EXIT=$?
    set -e
}

assert_exit_zero() {
    ((PUBLISHER_EXIT == 0)) ||
        fail "$1: expected exit 0, got ${PUBLISHER_EXIT}
$(cat "${PUBLISHER_LOG}")"
}

assert_output_contains() {
    grep -qF -- "$1" "${PUBLISHER_LOG}" ||
        fail "expected publisher output to contain '$1'
$(cat "${PUBLISHER_LOG}")"
}

assert_output_lacks() {
    if grep -qF -- "$1" "${PUBLISHER_LOG}"; then
        fail "did not expect publisher output to contain '$1'
$(cat "${PUBLISHER_LOG}")"
    fi
}

assert_equals() {
    [[ "$2" == "$3" ]] || fail "$1: expected '$3', got '$2'"
}

step_output() {
    sed -n "s/^$1=//p" "${GITHUB_OUTPUT}"
}

# A duplicate key in $GITHUB_OUTPUT leaves which value wins undefined, and the
# failure is silent: the upload step would be skipped after a good deploy, or
# run with an empty artifact name. Every exit path must emit each key once.
assert_step_outputs() {
    local expected_name="$1" expected_published="$2" key count
    local expected_dir="${3:-${STATUS_DIR}}"
    local lines
    lines="$(grep -c . "${GITHUB_OUTPUT}" || true)"
    [[ "${lines}" == "3" ]] ||
        fail "expected 3 step outputs, got ${lines}:
$(cat "${GITHUB_OUTPUT}")"

    for key in artifact-name status-dir published; do
        count="$(grep -c "^${key}=" "${GITHUB_OUTPUT}" || true)"
        [[ "${count}" == "1" ]] ||
            fail "expected '${key}' in \$GITHUB_OUTPUT exactly once, got \
${count}:
$(cat "${GITHUB_OUTPUT}")"
    done

    assert_equals "artifact-name output" \
        "$(step_output artifact-name)" "${expected_name}"
    assert_equals "published output" \
        "$(step_output published)" "${expected_published}"
    assert_equals "status-dir output" \
        "$(step_output status-dir)" "${expected_dir}"
}

# The whole directory ships inside the artifact, so anything stray in it becomes
# part of the published payload that users download.
assert_status_dir_contains_exactly() {
    local dir="${2:-${STATUS_DIR}}"
    local actual
    actual="$(cd "${dir}" && find . -mindepth 1 |
        sed 's|^\./||' | LC_ALL=C sort)"
    [[ "${actual}" == "$1" ]] ||
        fail "unexpected contents in ${dir}; expected:
$1
got:
${actual}"
}

assert_status_file_contains() {
    local file="${STATUS_DIR}/$1"
    [[ -f "${file}" ]] || fail "expected '$1' in STATUS_DIR"
    grep -qF -- "$2" "${file}" ||
        fail "expected '$1' to contain '$2', got:
$(cat "${file}")"
}

assert_status_files_differ() {
    if cmp -s "${STATUS_DIR}/$1" "${STATUS_DIR}/$2"; then
        fail "'$1' and '$2' must carry distinct content"
    fi
}

# deploy-progress.json is the cross-repo interface the canvas renders the graph
# from. A substring check would not notice a renamed field, and the canvas fails
# silently on one - it renders an empty graph - so assert the shape instead.
readonly PROGRESS_JSON_FILE_NAME="deploy-progress.json"

progress_jq() {
    local filter="$1"
    shift
    jq -e "${filter}" "$@" "${STATUS_DIR}/${PROGRESS_JSON_FILE_NAME}" \
        >/dev/null
}

assert_progress_contract() {
    local file="${STATUS_DIR}/${PROGRESS_JSON_FILE_NAME}"
    [[ -f "${file}" ]] || fail "expected ${PROGRESS_JSON_FILE_NAME} to be written"
    jq -e . "${file}" >/dev/null 2>&1 ||
        fail "${PROGRESS_JSON_FILE_NAME} is not valid JSON:
$(cat "${file}")"

    progress_jq '.schemaVersion == 1' ||
        fail "expected .schemaVersion == 1, got: $(jq -c '.schemaVersion' \
"${file}")"
    progress_jq '.state | type == "string" and length > 0' ||
        fail "expected a non-empty .state"
    progress_jq '.resources | type == "array"' ||
        fail "expected .resources to be an array"
    progress_jq '[.application, .environment, .updatedAt] |
        all(type == "string" and length > 0)' ||
        fail "expected non-empty .application, .environment and .updatedAt"
    progress_jq '.runId | type == "number"' ||
        fail "expected a numeric .runId"

    # Every resource must carry both the raw provisioningState and the
    # normalized status: the consumer needs the normalized value to paint the
    # graph, and the raw one to recover if the mapping goes stale.
    progress_jq 'all(.resources[]; has("id") and has("name") and has("type")
        and has("provisioningState") and has("outputResourceIds")
        and has("status") and has("message"))' ||
        fail "every .resources[] entry needs id, name, type, provisioningState,\
 outputResourceIds, status and message; got: $(jq -c '.resources' "${file}")"
    progress_jq 'all(.resources[]; .outputResourceIds | type == "array")' ||
        fail "every .resources[].outputResourceIds must be an array"
    progress_jq 'all(.resources[];
        .status == "success" or .status == "failed"
        or .status == "in_progress")' ||
        fail "unexpected .resources[].status value: $(jq -c \
'[.resources[].status]' "${file}")"
}

assert_resource_status() {
    # $name/$want are jq variables, not shell expansions.
    # shellcheck disable=SC2016
    progress_jq --arg name "$1" --arg want "$2" \
        'any(.resources[]; .name == $name and .status == $want)' ||
        fail "expected resource '$1' to normalize to status '$2', got: \
$(jq -c --arg n "$1" '.resources[] | select(.name == $n)' \
"${STATUS_DIR}/${PROGRESS_JSON_FILE_NAME}")"
}

assert_run_state() {
    # shellcheck disable=SC2016
    progress_jq --arg want "$1" '.state == $want' ||
        fail "expected run .state '$1', got: $(jq -c '.state' \
"${STATUS_DIR}/${PROGRESS_JSON_FILE_NAME}")"
}

assert_fact() {
    local actual
    actual="$(sed -n "s|^$1=||p" "${STEP_FACTS}")"
    [[ "${actual}" == "$2" ]] ||
        fail "action.yml: expected $1 to be '$2', got '${actual}'"
}

write_app_file() {
    printf 'resource app %s = {\n  name: %s\n}\n' \
        "'Radius.Core/applications'" "'$2'" >"$1"
}

reset_environment() {
    PUBLISHER_LOCALE=""
    PUBLISHER_WITHOUT_RUNNER_TEMP=false
    export ENVIRONMENT="aks-dev"
    export GITHUB_SHA="deadbeefcafe"
    export GITHUB_RUN_ID="4242"
    export APP_FILE="${LITERAL_APP_FILE}"
    unset RAD_GRAPH_SHOULD_FAIL RAD_APP_LIST_NAME
    unset RAD_RESOURCE_LIST_SHOULD_FAIL
    printf '{"outcome":"success","exitCode":0}\n' >"${RESULT_FILE}"
}

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------
extract_action_body
extract_step_facts
redirect_result_file
write_rad_stub

write_app_file "${LITERAL_APP_FILE}" "todolist"
write_app_file "${UNICODE_APP_FILE}" "$(printf 'caf\303\251')"
write_app_file "${MESSY_APP_FILE}" "My App/Prod"
printf 'param appName string\nresource app %s = {\n  name: appName\n}\n' \
    "'Radius.Core/applications'" >"${NO_LITERAL_APP_FILE}"

readonly EXPECTED_STATUS_FILES="deploy-activity.log
deploy-controlplane.log
deploy-graph.json
deploy-progress.json
deploy-state.txt"

# ---------------------------------------------------------------------------
# Happy path.
# ---------------------------------------------------------------------------
reset_environment
run_publisher
assert_exit_zero "happy path"
assert_step_outputs "radius-deploy-status-aks-dev-todolist" "true"
assert_status_dir_contains_exactly "${EXPECTED_STATUS_FILES}"

# The sibling status files must carry distinct signals. An earlier version
# copied rad-commands-result.json into all of them, publishing identical files
# that told the reader nothing about what actually happened.
assert_progress_contract
assert_run_state "succeeded"
progress_jq '.runId == 4242' || fail "expected .runId to come from GITHUB_RUN_ID"
progress_jq '.sequence == 1' ||
    fail "expected terminal sequence 1 when no live snapshot was published"
# shellcheck disable=SC2016
progress_jq --arg app "todolist" '.application == $app' ||
    fail "expected .application to be the resolved app name"
progress_jq '.resources | length == 3' ||
    fail "expected all three resources in deploy-progress.json"
# Succeeded/Failed map to terminal statuses; anything else must normalize to
# in_progress so a Radius state this action has never seen cannot paint a node
# red in the canvas.
assert_resource_status "web" "success"
assert_resource_status "db" "failed"
assert_resource_status "queue" "in_progress"
progress_jq 'any(.resources[];
    .name == "queue" and .provisioningState == "Updating")' ||
    fail "expected the raw provisioningState to be preserved alongside status"
progress_jq 'any(.resources[];
    .name == "web" and .message == "container ready")' ||
    fail "expected .message to carry properties.status.message"
progress_jq 'any(.resources[]; .name == "web"
    and .outputResourceIds == [
      "/planes/kubernetes/local/providers/apps/Deployment/web",
      "/planes/kubernetes/local/providers/core/Service/web"
    ])' || fail "expected web output resource IDs to be sorted and deduplicated"
progress_jq 'any(.resources[]; .name == "db"
    and .outputResourceIds == [
      "/subscriptions/test/providers/Microsoft.DBforMySQL/flexibleServers/db"
    ])' || fail "expected MySQL progress to carry its exact resolved server ID"
progress_jq 'any(.resources[]; .name == "queue"
    and .outputResourceIds == [])' ||
    fail "resources without resolved outputs need an empty ID array"

assert_status_file_contains "deploy-activity.log" '"outcome":"success"'
assert_status_file_contains "deploy-controlplane.log" "# rad version"
assert_status_file_contains "deploy-controlplane.log" "# rad env list --preview"
assert_status_file_contains "deploy-controlplane.log" "aks-dev"
assert_status_file_contains "deploy-state.txt" "state=success"
assert_status_file_contains "deploy-state.txt" "application=todolist"
assert_status_file_contains "deploy-state.txt" "sha=deadbeefcafe"
assert_status_file_contains "deploy-graph.json" "Radius.Compute/containers"
assert_status_file_contains "deploy-graph.json" \
    "Microsoft.DBforMySQL/flexibleServers"
jq -e 'all(.resources[]?; (.properties.containers // {}) |
    all(.[]?; has("env") | not))' "${STATUS_DIR}/deploy-graph.json" >/dev/null ||
    fail "deploy-graph.json must not contain container environment maps"
assert_status_files_differ "deploy-progress.json" "deploy-activity.log"
assert_status_files_differ "deploy-progress.json" "deploy-controlplane.log"
assert_status_files_differ "deploy-activity.log" "deploy-controlplane.log"

# A successful live publisher checkpoint hands the terminal publisher the next
# sequence, so the fixed-name artifact always wins over live ring snapshots.
reset_environment
mkdir -p "${RUNNER_TEMP}/radius-deploy-progress"
printf '7\n' >"${RUNNER_TEMP}/radius-deploy-progress/sequence"
run_publisher preserve
assert_exit_zero "live sequence handoff"
progress_jq '.sequence == 8' ||
    fail "expected terminal sequence to follow the last live sequence"

# ---------------------------------------------------------------------------
# A failed deploy must be reported as such at the run level.
# ---------------------------------------------------------------------------
reset_environment
printf '{"outcome":"failed","exitCode":1}\n' >"${RESULT_FILE}"
run_publisher
assert_exit_zero "failed deploy"
assert_step_outputs "radius-deploy-status-aks-dev-todolist" "true"
assert_progress_contract
assert_run_state "failed"
assert_status_file_contains "deploy-state.txt" "state=failed"
assert_status_file_contains "deploy-state.txt" "exitCode=1"

# ---------------------------------------------------------------------------
# A run killed mid-command by a step timeout is reported by run-rad-commands
# as its pessimistic seed. That outcome is not one this action names
# explicitly, so the `*)` arm must catch it as failed - reporting it as
# in_progress would repeat the bug the seed exists to prevent. (Cancellation
# kills the step the same way, but the workflows skip publishing entirely on
# `!cancelled()`, so it does not reach this mapping.)
# ---------------------------------------------------------------------------
reset_environment
printf '{"outcome":"interrupted","exitCode":1}\n' >"${RESULT_FILE}"
run_publisher
assert_exit_zero "interrupted deploy"
assert_progress_contract
assert_run_state "failed"
assert_status_file_contains "deploy-state.txt" "state=interrupted"

# ---------------------------------------------------------------------------
# An unreachable control plane must still produce a well-formed progress file
# rather than a malformed one the canvas cannot parse.
# ---------------------------------------------------------------------------
reset_environment
export RAD_RESOURCE_LIST_SHOULD_FAIL=true
run_publisher
assert_exit_zero "resource list failure"
assert_step_outputs "radius-deploy-status-aks-dev-todolist" "true"
assert_status_dir_contains_exactly "${EXPECTED_STATUS_FILES}"
assert_progress_contract
progress_jq '.resources == []' ||
    fail "expected an empty .resources array when rad resource list fails"
assert_output_contains "::warning::rad resource list --preview failed; deploy progress will contain no resources."
assert_output_contains "rad: could not reach the control plane"

# ---------------------------------------------------------------------------
# Non-ASCII application names must sanitize to a valid artifact name.
#
# Sanitization runs under LC_ALL=C so the [^a-z0-9._-] class is evaluated
# bytewise. In a UTF-8 locale GNU sed leaves multi-byte characters intact, which
# yields "radius-deploy-status-aks-dev-café" - not a valid artifact name, and a
# name the canvas would never derive. C.UTF-8 does not reproduce that, so the
# scenario needs a full UTF-8 locale to be a real regression test.
# ---------------------------------------------------------------------------
reset_environment
export APP_FILE="${UNICODE_APP_FILE}"
PUBLISHER_LOCALE="$(locale -a 2>/dev/null |
    grep -iE '\.utf-?8$' | grep -viE '^(C|POSIX)\.' | head -1 || true)"
if [[ -z "${PUBLISHER_LOCALE}" ]]; then
    echo "note: no full UTF-8 locale installed; name test runs byte-wise only"
fi
run_publisher
assert_exit_zero "non-ASCII app name"
assert_step_outputs "radius-deploy-status-aks-dev-caf" "true"

# Uppercase, spaces and slashes must collapse to single separators.
reset_environment
export APP_FILE="${MESSY_APP_FILE}"
run_publisher
assert_exit_zero "app name needing sanitization"
assert_step_outputs "radius-deploy-status-aks-dev-my-app-prod" "true"

# ---------------------------------------------------------------------------
# A nonliteral app name must disable graph publication even when restored state
# contains another application. Publishing that app would expose unrelated data.
# ---------------------------------------------------------------------------
reset_environment
export APP_FILE="${NO_LITERAL_APP_FILE}"
export RAD_APP_LIST_NAME="fallback-app"
run_publisher
assert_exit_zero "nonliteral application name"
assert_step_outputs "" "false"
assert_output_contains "::warning::Could not determine application name"
assert_status_dir_contains_exactly ""

# ---------------------------------------------------------------------------
# Publishing status is best-effort reporting: a failure warns, exits 0 and
# reports published=false so the upload step is skipped rather than failing an
# otherwise successful deployment.
# ---------------------------------------------------------------------------
reset_environment
export RAD_GRAPH_SHOULD_FAIL=true
run_publisher
assert_exit_zero "graph generation failure"
assert_step_outputs "" "false"
assert_output_contains "::warning::Failed to generate deployed graph"
# The graph command's stderr must be surfaced in the log but must not be left
# inside STATUS_DIR, where it would ship to users inside the artifact.
assert_output_contains "rad: application not found in environment"
assert_status_dir_contains_exactly "deploy-graph.json"

# ---------------------------------------------------------------------------
# A missing rad-commands-result.json must not break the publish.
# ---------------------------------------------------------------------------
reset_environment
rm -f "${RESULT_FILE}"
run_publisher
assert_exit_zero "missing rad-commands-result.json"
assert_step_outputs "radius-deploy-status-aks-dev-todolist" "true"
assert_status_dir_contains_exactly "${EXPECTED_STATUS_FILES}"
assert_status_file_contains "deploy-activity.log" \
    "rad-commands-result.json not found"
assert_status_file_contains "deploy-state.txt" "state=unknown"

# ---------------------------------------------------------------------------
# STATUS_DIR must be rebuilt from scratch so a previous run's files cannot be
# republished as if they belonged to this deploy.
# ---------------------------------------------------------------------------
reset_environment
mkdir -p "${STATUS_DIR}"
printf 'stale\n' >"${STATUS_DIR}/stale-from-previous-run.log"
run_publisher preserve
assert_exit_zero "stale status directory"
assert_status_dir_contains_exactly "${EXPECTED_STATUS_FILES}"

# ---------------------------------------------------------------------------
# The step must run outside an Actions runner, where RUNNER_TEMP is unset. That
# is the only reason the fallback exists, and without it `set -u` aborts the
# step with an unbound variable before anything is generated.
#
# This scenario deliberately writes to the real /tmp/radius-deploy-status and
# cannot be sandboxed: the fallback path is fixed by the action, so pointing
# RUNNER_TEMP anywhere would stop exercising the fallback and make this control
# vacuous - it would then pass even against a step that had lost the `:-`
# default. A static grep for the default has the same flaw. Leave it as is; the
# directory is the action's own scratch space, the action `rm -rf`s it at the
# start of every run, and the trap above removes it here.
# ---------------------------------------------------------------------------
reset_environment
rm -rf "${FALLBACK_STATUS_DIR}"
PUBLISHER_WITHOUT_RUNNER_TEMP=true
run_publisher
assert_exit_zero "RUNNER_TEMP unset"
assert_output_lacks "RUNNER_TEMP: unbound variable"
assert_step_outputs "radius-deploy-status-aks-dev-todolist" "true" \
    "${FALLBACK_STATUS_DIR}"
assert_status_dir_contains_exactly "${EXPECTED_STATUS_FILES}" \
    "${FALLBACK_STATUS_DIR}"
rm -rf "${FALLBACK_STATUS_DIR}"

# ---------------------------------------------------------------------------
# The wiring between the two composite steps. These three expressions are the
# only thing connecting the generate step's outputs to the upload, and no runtime
# scenario here can exercise them.
# ---------------------------------------------------------------------------
assert_fact "generate.id" "generate"
assert_fact "generate.shell" "bash"
assert_fact "upload.continue-on-error" "true"
assert_fact "upload.if" "steps.generate.outputs.published == 'true'"
# GitHub Actions expressions are literal text here, not shell expansions.
# shellcheck disable=SC2016
{
    assert_fact "upload.with.name" \
        '${{ steps.generate.outputs.artifact-name }}'
    assert_fact "upload.with.path" \
        '${{ steps.generate.outputs.status-dir }}'
    assert_fact "outputs.artifact-name.value" \
        '${{ steps.generate.outputs.artifact-name }}'
}

# ---------------------------------------------------------------------------
# Static guard for the one behaviour the runtime scenarios cannot always reach:
# where no full UTF-8 locale is installed the name test cannot distinguish
# byte-wise sanitization from the buggy character-wise form, so require the
# LC_ALL=C prefixes to stay on the sanitization pipeline.
# ---------------------------------------------------------------------------
grep -q "LC_ALL=C tr" "${PROGRESS_HELPER}" ||
    fail "artifact name sanitization must lowercase under LC_ALL=C"
grep -q "LC_ALL=C sed" "${PROGRESS_HELPER}" ||
    fail "artifact name sanitization must strip invalid bytes under LC_ALL=C"
# APP_NAME is intentionally literal because this checks the generated script.
# shellcheck disable=SC2016
grep -q 'rad resource list --preview -a "$APP_NAME" -o json' "${BODY_SCRIPT}" ||
    fail "deploy progress must list Radius.Core resources with --preview"

echo "publish-deploy-status tests passed"
