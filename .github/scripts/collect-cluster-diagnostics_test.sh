#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
for tool in jq timeout; do
    command -v "${tool}" >/dev/null 2>&1 || {
        echo "${tool} is required for diagnostics tests (GNU coreutils timeout)." >&2
        exit 1
    }
done
WORK_DIR="$(mktemp -d)"
readonly WORK_DIR
trap 'rm -rf "${WORK_DIR}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    grep -Fq -- "$2" "$1" || fail "expected $1 to contain '$2'"
}

assert_inventory_row() {
    local inventory="$1"
    shift
    local expected
    expected="$(printf '%s\t' "$@")"
    expected="${expected%$'\t'}"
    grep -Fxq -- "${expected}" "${inventory}" \
        || fail "missing exact inventory row: ${expected}"
}

assert_stopped() {
    local pid="$1"
    for _ in {1..30}; do
        if ! kill -0 "${pid}" 2>/dev/null; then
            return
        fi
        sleep 0.1
    done
    fail "process ${pid} survived diagnostics cleanup"
}

mkdir -p "${WORK_DIR}/bin"
cat >"${WORK_DIR}/bin/kubectl" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo "$*" >>"${MOCK_CALLS}"
if [[ "${MOCK_FAILURE:-}" == all ]]; then
    echo "mock API server unavailable" >&2
    exit 1
fi
if [[ "$*" == "get pods -n "*" -o json" ]]; then
    case "${MOCK_FAILURE:-}" in
        inventory) echo "mock inventory forbidden" >&2; exit 1 ;;
        malformed) echo '{"items":'; exit 0 ;;
        empty) echo '{"items":[]}'; exit 0 ;;
    esac
    if [[ "${MOCK_LARGE:-}" == true ]]; then
        jq -n '{items: [range(12) | {
            metadata: {name: ("pod-" + tostring), uid: "uid"},
            spec: {containers: [{name: "ucp", image: "ucp:release"}]}
        }]}'
    else
        cat <<'JSON'
{"items":[
  {"metadata":{"name":"ucp-pod","uid":"uid-ucp"},
   "spec":{"containers":[{"name":"ucp","image":"ucp:release",
       "env":[{"name":"PASSWORD","value":"SHOULD_NOT_PERSIST"}]}],
       "initContainers":[{"name":"init","image":"init:release"}]},
   "status":{"containerStatuses":[{"name":"ucp","restartCount":1,
       "containerID":"containerd://current",
       "state":{"running":{"startedAt":"2026-09-17T10:00:00Z"}},
       "lastState":{"terminated":{"containerID":"containerd://previous",
           "startedAt":"2026-09-17T09:58:00Z",
           "finishedAt":"2026-09-17T09:59:00Z"}}}]}},
  {"metadata":{"name":"bicep-pod","uid":"uid-bicep"},
   "spec":{"containers":[{"name":"de","image":"bicep:release"}]},
   "status":{"containerStatuses":[{"name":"de","restartCount":0}]}},
  {"metadata":{"name":"terminated-pod","uid":"uid-terminated"},
   "spec":{"containers":[{"name":"rp","image":"rp:release"}]},
   "status":{"containerStatuses":[{"name":"rp","restartCount":2,
       "containerID":"containerd://terminated-current",
       "state":{"terminated":{"startedAt":"2026-09-17T10:00:00Z",
           "finishedAt":"2026-09-17T10:05:00Z"}},
       "lastState":{"terminated":{
           "containerID":"containerd://terminated-previous",
           "startedAt":"2026-09-17T08:59:00Z",
           "finishedAt":"2026-09-17T09:00:00Z"}}}]}},
  {"metadata":{"name":"waiting-pod","uid":"uid-waiting"},
   "spec":{"containers":[{"name":"rp","image":"rp:release"}]},
   "status":{"containerStatuses":[{"name":"rp","restartCount":1,
       "state":{"waiting":{"reason":"CrashLoopBackOff"}},
       "lastState":{"terminated":{
           "containerID":"containerd://waiting-previous",
           "startedAt":"2026-09-17T09:00:00Z",
           "finishedAt":"2026-09-17T09:05:00Z"}}}]}}
]}
JSON
    fi
elif [[ "$1" == logs ]]; then
    if [[ "${MOCK_FAILURE:-}" == slow ]]; then
        sleep 10
    fi
    if [[ "$*" == *--previous* ]]; then
        echo "mock previous logs unavailable" >&2
        exit 1
    fi
    if [[ "${MOCK_LARGE:-}" == true ]]; then
        head -c 12000000 /dev/zero | tr '\0' x
    else
        echo "2026-09-17T10:40:00Z retained log after a quiet interval"
    fi
elif [[ "$1 $2" == "get events" ]]; then
    if [[ "${MOCK_FAILURE:-}" == events ]]; then
        echo "mock event access denied" >&2
        exit 1
    fi
    if [[ "${MOCK_LARGE:-}" == events ]]; then
        head -c 24000000 /dev/zero
        exit 0
    fi
    echo '{"items":[{"lastTimestamp":"2026-09-17T10:39:00Z","reason":"BackOff"}]}'
else
    echo "mock cluster snapshot"
fi
MOCK
chmod +x "${WORK_DIR}/bin/kubectl"
export PATH="${WORK_DIR}/bin:${PATH}"
export DIAGNOSTICS_NAME=all

new_case() {
    export RADIUS_CONTAINER_LOG_PATH="${WORK_DIR}/$1"
    export MOCK_CALLS="${WORK_DIR}/$1-calls"
    unset MOCK_FAILURE MOCK_LARGE DIAGNOSTICS_LOG_NAMESPACES
    unset DIAGNOSTICS_SINCE_TIME DIAGNOSTICS_COLLECTION_SECONDS
    unset DIAGNOSTICS_SAMPLE_INTERVAL KIND_CLUSTER_NAME
    mkdir -p "${RADIUS_CONTAINER_LOG_PATH}"
    : >"${MOCK_CALLS}"
}

collect() {
    bash "${SCRIPT_DIR}/collect-cluster-diagnostics.sh" \
        >"${RADIUS_CONTAINER_LOG_PATH}/collector-output.log" 2>&1
}

new_case legacy
collect
[[ "$(wc -l <"${MOCK_CALLS}")" -eq 5 ]] || fail "legacy capture changed"
[[ ! -d "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics" ]] \
    || fail "legacy callers unexpectedly collect container logs"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-tests-pod-states.log" \
    "kubectl get events -A"

new_case no-window
export DIAGNOSTICS_LOG_NAMESPACES=radius-system
collect
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
    "SKIPPED: no test window"
[[ "$(wc -l <"${MOCK_CALLS}")" -eq 5 ]] || fail "collected unbounded history"

new_case window
export DIAGNOSTICS_LOG_NAMESPACES=radius-system
diag="${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics"
mkdir -p "${diag}"
echo "2026-09-17T10:00:00Z" >"${diag}/window-start.txt"
echo "legacy stream" >"${RADIUS_CONTAINER_LOG_PATH}/ucp-pod.ucp.log"
collect
assert_contains "${MOCK_CALLS}" "--since-time=2026-09-17T10:00:00Z"
assert_contains "${MOCK_CALLS}" "--timestamps"
assert_contains "${MOCK_CALLS}" "--request-timeout=10s"
assert_contains "${diag}/radius-system/ucp-pod.ucp.current.log" "retained log"
assert_contains "${diag}/radius-system/ucp-pod.init.current.log" "retained log"
assert_contains "${diag}/radius-system/ucp-pod.ucp.previous.log" \
    "mock previous logs unavailable"
assert_contains "${diag}/radius-system/bicep-pod.de.current.log" "retained log"
assert_contains "${diag}/radius-system/containers.tsv" "uid-ucp"
assert_contains "${diag}/radius-system/containers.tsv" "containerd://current"
assert_inventory_row "${diag}/radius-system/containers.tsv" \
    ucp-pod uid-ucp ucp 1 ucp:release containerd://current \
    2026-09-17T10:00:00Z - containerd://previous \
    2026-09-17T09:58:00Z 2026-09-17T09:59:00Z
assert_inventory_row "${diag}/radius-system/containers.tsv" \
    terminated-pod uid-terminated rp 2 rp:release \
    containerd://terminated-current \
    2026-09-17T10:00:00Z 2026-09-17T10:05:00Z \
    containerd://terminated-previous \
    2026-09-17T08:59:00Z 2026-09-17T09:00:00Z
assert_inventory_row "${diag}/radius-system/containers.tsv" \
    waiting-pod uid-waiting rp 1 rp:release - - - \
    containerd://waiting-previous 2026-09-17T09:00:00Z 2026-09-17T09:05:00Z
assert_inventory_row "${diag}/radius-system/containers.tsv" \
    bicep-pod uid-bicep de 0 bicep:release - - - - - -
assert_inventory_row "${diag}/radius-system/containers.tsv" \
    ucp-pod uid-ucp init 0 init:release - - - - - -
assert_contains "${diag}/collection.log" \
    "currentStarted currentFinished previousContainerID previousStarted previousFinished"
assert_contains "${diag}/collection.log" "exit=1"
assert_contains "${diag}/collection.log" "FAILED:"
assert_contains "${diag}/collection.log" "Collection finished:"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/ucp-pod.ucp.log" "legacy stream"
[[ ! -e "${diag}/radius-system/bicep-pod.de.previous.log" ]] \
    || fail "previous logs requested without a restart"
if grep -RqE 'SHOULD_NOT_PERSIST|TRUNCATED-BY-ROTATION' "${diag}"; then
    fail "leaked podspec or falsely inferred rotation"
fi

new_case explicit-window
export DIAGNOSTICS_LOG_NAMESPACES=radius-system
export DIAGNOSTICS_SINCE_TIME=2026-09-17T10:15:00Z
mkdir -p "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics"
echo "2026-09-17T10:00:00Z" \
    >"${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/window-start.txt"
collect
assert_contains "${MOCK_CALLS}" "--since-time=2026-09-17T10:15:00Z"

for failure in inventory malformed all; do
    new_case "${failure}"
    export DIAGNOSTICS_LOG_NAMESPACES=radius-system
    export DIAGNOSTICS_SINCE_TIME=2026-09-17T10:00:00Z
    export MOCK_FAILURE="${failure}"
    collect
    assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
        "FAILED: inventory"
    assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-tests-pod-states.log" \
        "kubectl get events -A"
done

new_case invalid
export DIAGNOSTICS_LOG_NAMESPACES=radius-system
export DIAGNOSTICS_SINCE_TIME=not-a-timestamp
collect
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
    "FAILED: invalid DIAGNOSTICS_SINCE_TIME"

new_case empty
export DIAGNOSTICS_LOG_NAMESPACES=radius-system MOCK_FAILURE=empty
export DIAGNOSTICS_SINCE_TIME=2026-09-17T10:00:00Z
collect
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
    "NO_CONTAINERS:"

new_case limits
export DIAGNOSTICS_LOG_NAMESPACES=radius-system MOCK_LARGE=true
export DIAGNOSTICS_SINCE_TIME=2026-09-17T10:00:00Z
collect
total=0
count=0
for file in "${RADIUS_CONTAINER_LOG_PATH}"/all-diagnostics/radius-system/*.log; do
    size="$(wc -c <"${file}")"
    [[ "${size}" -eq 10485760 ]] || fail "per-file cap not enforced: ${size}"
    total=$((total + size))
    count=$((count + 1))
done
[[ "${total}" -eq 104857600 && "${count}" -eq 10 ]] \
    || fail "total cap not enforced: ${total} bytes in ${count} files"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
    "LIMIT_REACHED: time or total log budget exhausted."

new_case timeout
export DIAGNOSTICS_LOG_NAMESPACES=radius-system MOCK_FAILURE=slow
export DIAGNOSTICS_SINCE_TIME=2026-09-17T10:00:00Z
export DIAGNOSTICS_COLLECTION_SECONDS=2
started="${SECONDS}"
collect
((SECONDS - started < 6)) || fail "container sweep exceeded its time budget"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
    "exit=124"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/collection.log" \
    "LIMIT_REACHED:"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-tests-pod-states.log" \
    "kubectl get events -A"

new_case sample-success
export DIAGNOSTICS_SAMPLE_INTERVAL=1
bash "${SCRIPT_DIR}/run-with-cluster-diagnostics.sh" bash -c \
    'printf "%s\n" "$1"; sleep 3' _ 'argument with spaces' \
    >"${RADIUS_CONTAINER_LOG_PATH}/command-output.log" 2>&1
diag="${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/command-output.log" \
    "argument with spaces"
assert_contains "${diag}/events-1.log" "Sample started:"
assert_contains "${diag}/events-2.log" "BackOff"
assert_contains "${diag}/sampler.log" "exit=0"
assert_contains "${diag}/sampler.log" "Sampler stopped:"
[[ -s "${diag}/window-start.txt" ]] || fail "missing diagnostic window"
pid="$(sed -n 's/^Sampler started: pid=\([0-9]*\).*/\1/p' "${diag}/sampler.log")"
assert_stopped "${pid}"
export DIAGNOSTICS_LOG_NAMESPACES=radius-system
collect
assert_contains "${MOCK_CALLS}" "--since-time=$(<"${diag}/window-start.txt")"

new_case sample-failure
export MOCK_FAILURE=events
status=0
bash "${SCRIPT_DIR}/run-with-cluster-diagnostics.sh" bash -c 'sleep 1; exit 23' \
    >"${RADIUS_CONTAINER_LOG_PATH}/command-output.log" 2>&1 || status=$?
[[ "${status}" -eq 23 ]] || fail "diagnostics replaced test failure: ${status}"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/events-1.log" \
    "FAILED: get events"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/sampler.log" \
    "exit=23"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/sampler.log" \
    "Sampler stopped:"

new_case event-limit
export MOCK_LARGE=events
bash "${SCRIPT_DIR}/run-with-cluster-diagnostics.sh" bash -c \
    'sleep 2; echo "command completed"' \
    >"${RADIUS_CONTAINER_LOG_PATH}/command-output.log" 2>&1
diag="${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics"
[[ "$(wc -c <"${diag}/events-1.log")" -eq 20971520 ]] \
    || fail "event output cap not enforced"
[[ ! -e "${diag}/events-2.log" ]] || fail "sampling continued past byte budget"
assert_contains "${diag}/sampler.log" "LIMIT_REACHED: 20 MiB"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/command-output.log" \
    "command completed"

new_case unwritable-status
mkdir -p "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/sampler.log"
status=0
bash "${SCRIPT_DIR}/run-with-cluster-diagnostics.sh" bash -c 'exit 25' \
    >"${RADIUS_CONTAINER_LOG_PATH}/command-output.log" 2>&1 || status=$?
[[ "${status}" -eq 25 ]] || fail "status write failure prevented test command"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/command-output.log" \
    "FAILED: cannot write sampler status"

new_case invalid-sampler
export DIAGNOSTICS_SAMPLE_INTERVAL=0
status=0
bash "${SCRIPT_DIR}/run-with-cluster-diagnostics.sh" bash -c 'exit 24' \
    >"${RADIUS_CONTAINER_LOG_PATH}/command-output.log" 2>&1 || status=$?
[[ "${status}" -eq 24 ]] || fail "invalid diagnostics prevented test command"
assert_contains "${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics/sampler.log" \
    "FAILED: DIAGNOSTICS_SAMPLE_INTERVAL"

new_case cancelled
bash "${SCRIPT_DIR}/run-with-cluster-diagnostics.sh" bash -c \
    'echo "$$" >"$RADIUS_CONTAINER_LOG_PATH/command.pid"; exec sleep 60' \
    >"${RADIUS_CONTAINER_LOG_PATH}/command-output.log" 2>&1 &
wrapper_pid=$!
for _ in {1..100}; do
    [[ -s "${RADIUS_CONTAINER_LOG_PATH}/command.pid" ]] && break
    sleep 0.1
done
[[ -s "${RADIUS_CONTAINER_LOG_PATH}/command.pid" ]] \
    || fail "wrapped command did not start"
kill -TERM "${wrapper_pid}"
status=0
wait "${wrapper_pid}" || status=$?
[[ "${status}" -eq 143 ]] || fail "cancellation status lost: ${status}"
diag="${RADIUS_CONTAINER_LOG_PATH}/all-diagnostics"
pid="$(sed -n 's/^Sampler started: pid=\([0-9]*\).*/\1/p' "${diag}/sampler.log")"
assert_stopped "${pid}"
assert_stopped "$(<"${RADIUS_CONTAINER_LOG_PATH}/command.pid")"
assert_contains "${diag}/sampler.log" "Sampler stopped:"

workflow="${SCRIPT_DIR}/../workflows/long-running-azure.yaml"
assert_contains "${workflow}" \
    "bash \"\$GITHUB_WORKSPACE/.github/scripts/run-with-cluster-diagnostics.sh\""
assert_contains "${workflow}" \
    "bash -e -c 'make test-functional-all-cloud; make test-functional-all-noncloud'"
assert_contains "${workflow}" 'DIAGNOSTICS_LOG_NAMESPACES: radius-system'

assert_contains "${SCRIPT_DIR}/../workflows/unit-tests.yaml" \
    'run: make test-cluster-diagnostics'
status=0
make -f "${SCRIPT_DIR}/../../build/test.mk" -qp \
    >"${WORK_DIR}/make-database" || status=$?
# Query mode returns 1 for an out-of-date phony target; nothing is executed.
((status <= 1)) || fail "could not inspect Make prerequisites"
grep -q '^test: ' "${WORK_DIR}/make-database" \
    || fail "make test target not found"
if grep -Eq '^test:.* test-cluster-diagnostics([[:space:]]|$)' \
    "${WORK_DIR}/make-database"; then
    fail "make test must not require diagnostics-only tools"
fi

echo "cluster diagnostics tests passed"
