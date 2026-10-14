#!/usr/bin/env bash

# Test push recovery without a Docker daemon or a live registry.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

export CALL_LOG="${TEST_DIR}/calls"
export FAILURES=0 FAILURE_STATUS=1 FAILURE_MESSAGE="unknown blob"
export TEE_STATUS=0
readonly IMAGE="ghcr.io/radius-project/dev/magpiego:test"

mkdir "${TEST_DIR}/bin"
cat > "${TEST_DIR}/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -eq 2 && "$1" == push ]] || exit 99
count="$(grep -c '^push ' "${CALL_LOG}" || true)"
printf '%s\n' "$*" >> "${CALL_LOG}"
echo "The push refers to repository [$2]"
if [[ "${count}" -lt "${FAILURES}" ]]; then
    printf '%s\n' "${FAILURE_MESSAGE}" >&2
    exit "${FAILURE_STATUS}"
fi
echo "test: digest: sha256:123 size: 123"
EOF

cat > "${TEST_DIR}/bin/sleep" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'sleep %s\n' "$1" >> "${CALL_LOG}"
EOF
chmod +x "${TEST_DIR}/bin/docker" "${TEST_DIR}/bin/sleep"
export PATH="${TEST_DIR}/bin:${PATH}"

tee() {
    if [[ "${TEE_STATUS}" -ne 0 ]]; then
        cat > /dev/null
        return "${TEE_STATUS}"
    fi
    command tee "$@"
}

export -f tee

run_case() {
    local expected_status="$1"
    shift
    local status=0
    : > "${CALL_LOG}"
    "$@" > "${TEST_DIR}/output" 2>&1 || status=$?
    if [[ "${status}" -ne "${expected_status}" ]]; then
        cat "${TEST_DIR}/output"
        echo "Expected exit ${expected_status}, got ${status}" >&2
        exit 1
    fi
}

assert_calls() {
    printf '%s\n' "$@" > "${TEST_DIR}/expected"
    diff -u "${TEST_DIR}/expected" "${CALL_LOG}"
}

assert_output() {
    grep -F -- "$1" "${TEST_DIR}/output" > /dev/null
}

run_case 0 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
assert_calls "push ${IMAGE}"
assert_output "test: digest: sha256:123"

FAILURES=2
run_case 0 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
assert_calls "push ${IMAGE}" "sleep 5" "push ${IMAGE}" \
    "sleep 10" "push ${IMAGE}"
assert_output "unknown blob"
assert_output "(attempt 2/3); retrying in 10s"
assert_output "test: digest: sha256:123"

FAILURES=99
run_case 1 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
assert_calls "push ${IMAGE}" "sleep 5" "push ${IMAGE}" \
    "sleep 10" "push ${IMAGE}"
assert_output "(exit 1, attempt 3/3)"
[[ "$(grep -c '^unknown blob$' "${TEST_DIR}/output")" -eq 3 ]]

for FAILURE_MESSAGE in \
    "unauthorized: authentication required" \
    "denied: permission_denied: write_package" \
    "invalid reference format" \
    "tag does not exist" \
    "x509: certificate signed by unknown authority" \
    "digest mismatch" \
    "Cannot connect to the Docker daemon" \
    "unknown error" \
    "unexpected EOF" \
    "HTTP 503 Service Unavailable" \
    "repository name contains unknown blob" \
    $'unknown blob\ndenied: requested access is denied'; do
    run_case 1 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
    assert_calls "push ${IMAGE}"
    assert_output "${FAILURE_MESSAGE}"
    assert_output "(exit 1, attempt 1/3)"
done

FAILURE_MESSAGE="unknown blob"
for FAILURE_STATUS in 2 125 126 127 130 137 143; do
    run_case "${FAILURE_STATUS}" \
        bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
    assert_calls "push ${IMAGE}"
    assert_output "(exit ${FAILURE_STATUS}, attempt 1/3)"
done
FAILURE_STATUS=1

FAILURES=1
FAILURE_MESSAGE=$'unknown blob\r'
run_case 0 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
assert_calls "push ${IMAGE}" "sleep 5" "push ${IMAGE}"
FAILURE_MESSAGE="unknown blob"

FAILURES=0
TEE_STATUS=42
run_case 42 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}"
assert_calls "push ${IMAGE}"
assert_output "ERROR: Could not record docker push output"
TEE_STATUS=0

run_case 2 bash "${ROOT}/build/scripts/docker-push.sh"
[[ ! -s "${CALL_LOG}" ]]
assert_output "Usage: docker-push.sh IMAGE"
run_case 2 bash "${ROOT}/build/scripts/docker-push.sh" ""
[[ ! -s "${CALL_LOG}" ]]
run_case 2 bash "${ROOT}/build/scripts/docker-push.sh" "${IMAGE}" extra
[[ ! -s "${CALL_LOG}" ]]

# Run the actual Make recipes to check image arguments and prevent rebuilds.
cd "${ROOT}"
FAILURES=2
run_case 0 make --no-print-directory -s -f build/docker.mk \
    docker-push-magpiego DOCKER_REGISTRY=ghcr.io/radius-project/dev \
    DOCKER_TAG_VERSION=test
assert_calls "push ${IMAGE}" "sleep 5" "push ${IMAGE}" \
    "sleep 10" "push ${IMAGE}"

FAILURES=99
run_case 2 make --no-print-directory -s -f build/docker.mk \
    docker-push-magpiego DOCKER_REGISTRY=ghcr.io/radius-project/dev \
    DOCKER_TAG_VERSION=test
assert_calls "push ${IMAGE}" "sleep 5" "push ${IMAGE}" \
    "sleep 10" "push ${IMAGE}"
assert_output "(exit 1, attempt 3/3)"

FAILURES=0
run_case 0 make -j1 --no-print-directory -s -f build/docker.mk docker-push \
    DOCKER_REGISTRY=ghcr.io/radius-project/dev DOCKER_TAG_VERSION=test
assert_calls \
    "push ghcr.io/radius-project/dev/ucpd:test" \
    "push ghcr.io/radius-project/dev/applications-rp:test" \
    "push ghcr.io/radius-project/dev/dynamic-rp:test" \
    "push ghcr.io/radius-project/dev/controller:test" \
    "push ghcr.io/radius-project/dev/testrp:test" \
    "push ${IMAGE}" \
    "push ghcr.io/radius-project/dev/bicep:test" \
    "push ghcr.io/radius-project/dev/pre-upgrade:test"

echo "Docker push tests passed"
