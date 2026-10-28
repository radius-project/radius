#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
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

mkdir -p "${WORK_DIR}/bin"
cat >"${WORK_DIR}/bin/kubectl" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo "$*" >>"${MOCK_CALLS}"
if [[ "${MOCK_FAILURE:-}" == "$1 $2" ]]; then
    echo "mock API unavailable" >&2
    exit 1
fi
case "$1 $2" in
    "get pods")
        if [[ "${MOCK_FAILURE:-}" == malformed ]]; then
            echo '{"items":'
        elif [[ "${MOCK_FAILURE:-}" == empty ]]; then
            echo '{"items":[]}'
        else
            cat <<'JSON'
{"items":[{"metadata":{"name":"rp-pod","uid":"uid"},
 "spec":{"nodeName":"kind","containers":[
  {"name":"applications-rp","image":"rp:test",
   "resources":{"requests":{"memory":"160Mi"},"limits":{"memory":"500Mi"}},
   "env":[{"name":"PASSWORD","value":"SHOULD_NOT_LOG"}]},
  {"name":"sidecar","image":"sidecar:test"}]},
 "status":{"containerStatuses":[{"name":"applications-rp","restartCount":1,
  "state":{"running":{"startedAt":"2026-10-14T13:00:00Z"}},
  "lastState":{"terminated":{"reason":"OOMKilled","exitCode":137,"signal":9,
   "startedAt":"2026-10-14T12:00:00Z","finishedAt":"2026-10-14T12:59:00Z",
   "message":"SHOULD_NOT_LOG"}}}]}},
 {"metadata":{"name":"dynamic-pod","uid":"dynamic-uid"},
  "spec":{"containers":[{"name":"dynamic-rp","image":"dynamic:test"}]}}]}
JSON
        fi
        ;;
    "get nodes")
        echo '{"items":[{"metadata":{"name":"kind"},"status":{"capacity":{"memory":"8Gi"},"allocatable":{"memory":"7Gi"},"conditions":[{"type":"MemoryPressure","status":"False","reason":"SufficientMemory","message":"SHOULD_NOT_LOG"}]}}]}'
        ;;
    "get events")
        echo '{"items":[{"type":"Warning","reason":"BackOff","message":"SHOULD_NOT_LOG","involvedObject":{"kind":"Pod","name":"rp-pod","namespace":"radius-system"}}]}'
        ;;
    "exec rp-pod"|"exec dynamic-pod")
        sh -n -c "${!#}"
        echo "oom_kill 1"
        echo "memory.peak 524288000"
        ;;
    "logs rp-pod"|"logs dynamic-pod")
        echo '{"message":"Failed to load plugin schemas: GetProviderSchema; SHOULD_NOT_LOG"}'
        echo '{"message":"Plugin did not\nrespond; SHOULD_NOT_LOG"}'
        echo '{"error":"timeout while waiting for plugin to start; SHOULD_NOT_LOG"}'
        echo 'not JSON; SHOULD_NOT_LOG'
        ;;
    *) echo "unexpected kubectl call: $*" >&2; exit 1 ;;
esac
MOCK
cat >"${WORK_DIR}/bin/free" <<'MOCK'
#!/bin/bash
echo "Mem: 8192 4096 4096"
MOCK
chmod +x "${WORK_DIR}/bin/"*
export PATH="${WORK_DIR}/bin:${PATH}"
export MOCK_CALLS="${WORK_DIR}/calls"

for failure in none "get nodes" "get events" "exec rp-pod" \
    "logs rp-pod" "get pods" malformed empty; do
    export MOCK_FAILURE="${failure}"
    : >"${MOCK_CALLS}"
    output="${WORK_DIR}/${failure// /-}.log"
    bash "${SCRIPT_DIR}/collect-terraform-diagnostics.sh" >"${output}" 2>&1
    assert_contains "${output}" "Post-test snapshot"
    assert_contains "${output}" "Mem: 8192"
    assert_contains "${output}" "Terraform container state"
    if grep -Fq SHOULD_NOT_LOG "${output}"; then
        fail "diagnostics exposed a raw log, pod environment, or status message"
    fi
    case "${failure}" in
        none)
            for value in OOMKilled MemoryPressure BackOff 500Mi oom_kill \
                memory.peak runningSince previousTermination; do
                assert_contains "${output}" "${value}"
            done
            for field in schemaErrorRecords startupTimeoutRecords \
                pluginNoResponseRecords nonJSONRecords; do
                assert_contains "${output}" "\"${field}\": 1"
            done
            assert_contains "${MOCK_CALLS}" "--previous"
            assert_contains "${MOCK_CALLS}" "--since=40m"
            assert_contains "${MOCK_CALLS}" "--limit-bytes=1048576"
            assert_contains "${MOCK_CALLS}" "--request-timeout=10s"
            if grep -Eq 'sidecar|logs dynamic-pod .*--previous' \
                "${MOCK_CALLS}"; then
                fail "collected an unrelated container or nonexistent history"
            fi
            ;;
        empty) assert_contains "${output}" "NO_CONTAINERS:" ;;
        *) assert_contains "${output}" "FAILED:" ;;
    esac
done

assert_contains "${SCRIPT_DIR}/../workflows/functional-test-cloud.yaml" \
    "DIAGNOSTICS_TERRAFORM: \${{ matrix.name == 'corerp-cloud' && job.status != 'success' && 'true' || 'false' }}"
echo "Terraform diagnostics tests passed"
