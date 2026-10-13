#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

export CALL_LOG="${TEST_DIR}/calls"
export INPUT_IMAGE="registry:2"
export INPUT_RETRIES=5
export INPUT_RETRY_WAIT_SECONDS=5
export INPUT_PULL_TIMEOUT_SECONDS=60
export FAILURES=0
export FAILURE_STATUS=125
export FAILURE_MESSAGE="connection reset by peer"
export CACHED=false
export TEST_CONTAINER_START=false

docker() {
    if [[ "$*" == "image inspect registry:2" ]]; then
        echo "inspect" >> "${CALL_LOG}"
        [[ "${CACHED}" == true ]]
        return
    fi
    if [[ "${TEST_CONTAINER_START}" == true && "$1" == run ]]; then
        [[ "$*" == *"--name radius-registry"* && "${!#}" == registry:2 ]] || return 99
        if [[ "${TEST_SECURE}" == true ]]; then
            [[ "$*" == *"REGISTRY_HTTP_TLS_CERTIFICATE=/certs/client.crt"* ]] || return 99
        else
            [[ "$*" != *"REGISTRY_HTTP_TLS_CERTIFICATE"* ]] || return 99
        fi
        echo "run ${TEST_SECURE}" >> "${CALL_LOG}"
        return "${RUN_STATUS}"
    fi
    [[ "$*" == "pull registry:2" ]] || return 99
    local count
    count="$(grep -c '^docker$' "${CALL_LOG}" || true)"
    echo "docker" >> "${CALL_LOG}"
    if [[ "${count}" -lt "${FAILURES}" ]]; then
        echo "${FAILURE_MESSAGE}" >&2
        return "${FAILURE_STATUS}"
    fi
}

sleep() {
    local count lower
    count="$(grep -c '^sleep ' "${CALL_LOG}" || true)"
    lower=$((INPUT_RETRY_WAIT_SECONDS * 2 ** count))
    if [[ "$1" -lt "${lower}" || "$1" -gt "$((lower * 2))" ]]; then
        echo "Sleep $1 is outside jitter bounds ${lower}..$((lower * 2))" >&2
        return 99
    fi
    echo "sleep ${lower}" >> "${CALL_LOG}"
}

timeout() {
    if [[ "$1" == --kill-after=5 && "$2" == 600 && "$3" == bash ]]; then
        shift 2
        "$@"
        return
    fi
    [[ "$1" == --kill-after=5 && "$2" == "${INPUT_PULL_TIMEOUT_SECONDS}" ]] || return 99
    shift 2
    "$@"
}

export -f docker sleep timeout

run_case() {
    local expected_status="$1"
    : > "${CALL_LOG}"
    local status=0
    bash "${SCRIPT_DIR}/pull-image.sh" > "${TEST_DIR}/output" 2>&1 || status=$?
    if [[ "${status}" -ne "${expected_status}" ]]; then
        cat "${TEST_DIR}/output"
        echo "Expected exit ${expected_status}, got ${status}" >&2
        exit 1
    fi
}

assert_output() {
    grep -F -- "$1" "${TEST_DIR}/output" > /dev/null
}

run_case 0
printf 'inspect\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

FAILURES=2
run_case 0
printf 'inspect\ndocker\nsleep 5\ndocker\nsleep 10\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::warning::docker pull registry:2 failed (exit 125, attempt 2/5); retrying in"

FAILURES=99
run_case 125
printf 'inspect\ndocker\nsleep 5\ndocker\nsleep 10\ndocker\nsleep 20\ndocker\nsleep 40\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "::error::docker pull registry:2 failed after 5 attempts"
assert_output "${FAILURE_MESSAGE}"

FAILURES=4
run_case 0
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

CACHED=true
FAILURES=99
run_case 0
printf 'inspect\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
assert_output "Using cached image registry:2"
CACHED=false

FAILURE_STATUS=1
for FAILURE_MESSAGE in \
    "unauthorized: incorrect username or password" \
    "authentication required" \
    "pull access denied for registry" \
    "denied: requested access to the resource is denied" \
    "manifest unknown" \
    "name unknown" \
    "invalid reference format" \
    "invalid argument" \
    "no matching manifest for linux/amd64" \
    "x509: certificate signed by unknown authority" \
    "checksum mismatch" \
    "digest mismatch" \
    "filesystem layer verification failed for digest sha256:123"; do
    run_case 1
    printf 'inspect\ndocker\n' > "${TEST_DIR}/expected"
    diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
    assert_output "${FAILURE_MESSAGE}"
    assert_output "failed without retry (exit 1)"
done

FAILURES=2
for FAILURE_MESSAGE in \
    "HTTP 503 Service Unavailable" \
    "toomanyrequests: HTTP 429" \
    "dial tcp: lookup registry-1.docker.io: no such host" \
    "unexpected EOF"; do
    run_case 0
    printf 'inspect\ndocker\nsleep 5\ndocker\nsleep 10\ndocker\n' > "${TEST_DIR}/expected"
    diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
    assert_output "${FAILURE_MESSAGE}"
done
FAILURES=99

FAILURE_MESSAGE="connection reset by peer"
printf 'inspect\ndocker\n' > "${TEST_DIR}/expected"
for FAILURE_STATUS in 127 130 137 143; do
    run_case "${FAILURE_STATUS}"
    diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
done

FAILURE_STATUS=124
run_case 124
printf 'inspect\ndocker\nsleep 5\ndocker\nsleep 10\ndocker\nsleep 20\ndocker\nsleep 40\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

INPUT_RETRIES=1
run_case 124
printf 'inspect\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

INPUT_RETRIES=2
INPUT_RETRY_WAIT_SECONDS=0
FAILURES=1
run_case 0
printf 'inspect\ndocker\nsleep 0\ndocker\n' > "${TEST_DIR}/expected"
diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
INPUT_RETRY_WAIT_SECONDS=5

for INPUT_RETRIES in 0 -1 invalid 01; do
    run_case 1
    [[ ! -s "${CALL_LOG}" ]]
    assert_output "::error::retries must be a positive integer"
done
INPUT_RETRIES=5

for INPUT_RETRY_WAIT_SECONDS in -1 invalid 01; do
    run_case 1
    [[ ! -s "${CALL_LOG}" ]]
    assert_output "::error::retry-wait-seconds must be a nonnegative integer"
done
INPUT_RETRY_WAIT_SECONDS=5

for INPUT_PULL_TIMEOUT_SECONDS in 0 -1 invalid 01; do
    run_case 1
    [[ ! -s "${CALL_LOG}" ]]
    assert_output "::error::pull-timeout-seconds must be a positive integer"
done
INPUT_PULL_TIMEOUT_SECONDS=60

# Execute the action's actual pull and startup commands with fake Docker.
export PULL_SCRIPT="${SCRIPT_DIR}/pull-image.sh"
export INPUT_REGISTRY_PORT=5000 INPUT_REGISTRY_NAME=radius-registry
export INPUT_REGISTRY_SERVER=localhost
export STEPS_CREATE_TEMP_CERT_DIR_OUTPUTS_TEMP_CERT_DIR="${TEST_DIR}"
export TEST_CONTAINER_START=true RUN_STATUS=0
INPUT_RETRIES=5
FAILURES=0
FAILURE_STATUS=125
for TEST_SECURE in true false; do
    export TEST_SECURE
    step="Create insecure Docker registry"
    [[ "${TEST_SECURE}" == false ]] || step="Create secure Docker registry"
    awk -v step="${step}" '
        /- name:/ {
            pull = index($0, "Pull registry image") > 0
            start = index($0, step) > 0
            block = 0
        }
        pull && /      run: / { sub(/^      run: /, ""); pull_run = $0 }
        pull && /^        INPUT_/ {
            sub(/^        /, "")
            sub(/: /, "=")
            inputs = inputs "export " $0 "\n"
        }
        start && /      run: \|/ {
            printf "%s%s\n", inputs, pull_run
            block = 1
            next
        }
        block && /^        / { sub(/^        /, ""); print; next }
        block { block = 0 }
    ' "${SCRIPT_DIR}/action.yaml" > "${TEST_DIR}/action.sh"
    grep -F 'timeout --kill-after=5 600 bash "${PULL_SCRIPT}"' "${TEST_DIR}/action.sh" > /dev/null
    grep -F 'docker run -d' "${TEST_DIR}/action.sh" > /dev/null

    : > "${CALL_LOG}"
    bash -e -o pipefail "${TEST_DIR}/action.sh" > "${TEST_DIR}/output" 2>&1
    printf 'inspect\ndocker\nrun %s\n' "${TEST_SECURE}" > "${TEST_DIR}/expected"
    diff -u "${TEST_DIR}/expected" "${CALL_LOG}"

    CACHED=true
    : > "${CALL_LOG}"
    bash -e -o pipefail "${TEST_DIR}/action.sh" > "${TEST_DIR}/output" 2>&1
    printf 'inspect\nrun %s\n' "${TEST_SECURE}" > "${TEST_DIR}/cached-expected"
    diff -u "${TEST_DIR}/cached-expected" "${CALL_LOG}"
    CACHED=false

    FAILURES=99
    status=0
    : > "${CALL_LOG}"
    bash -e -o pipefail "${TEST_DIR}/action.sh" > "${TEST_DIR}/output" 2>&1 || status=$?
    [[ "${status}" -eq 125 ]]
    [[ "$(grep -c '^docker$' "${CALL_LOG}")" -eq 5 ]]
    if grep -q '^run ' "${CALL_LOG}"; then
        echo "Container started after a failed pull" >&2
        exit 1
    fi
    FAILURES=0

    RUN_STATUS=125
    status=0
    : > "${CALL_LOG}"
    bash -e -o pipefail "${TEST_DIR}/action.sh" > "${TEST_DIR}/output" 2>&1 || status=$?
    [[ "${status}" -eq 125 ]]
    diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
    RUN_STATUS=0
done

mkdir -p "${TEST_DIR}/bin"
cat > "${TEST_DIR}/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == "image inspect registry:2" ]]; then
    exit 1
fi
[[ "$*" == "pull registry:2" ]] || exit 99
echo "simulated stalled pull" >&2
sleep 10
EOF
chmod +x "${TEST_DIR}/bin/docker"
(
    unset -f docker sleep timeout
    export PATH="${TEST_DIR}/bin:${PATH}"
    export INPUT_RETRIES=1 INPUT_PULL_TIMEOUT_SECONDS=1
    status=0
    bash "${PULL_SCRIPT}" > "${TEST_DIR}/timeout-output" 2>&1 || status=$?
    [[ "${status}" -eq 124 ]]
    grep -F "simulated stalled pull" "${TEST_DIR}/timeout-output" > /dev/null
    grep -F "failed after 1 attempts" "${TEST_DIR}/timeout-output" > /dev/null
)

echo "Local registry image pull tests passed"
