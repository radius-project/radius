#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# Tests for the manual authorization test kit in hack/authz. Runs without a
# cluster: kubectl is replaced by a stub that records its calls.
# ============================================================================

set -euo pipefail

KIT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly KIT_DIR
REPO_ROOT="$(cd "${KIT_DIR}/../.." && pwd)"
readonly REPO_ROOT
readonly SCRIPTS=(up.sh rogue.sh as-component.sh records.sh would-deny.sh)

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
PASS=0
FAIL=0

cleanup() {
    rm -rf "${TEST_ROOT}"
}
trap cleanup EXIT

fail_test() {
    echo "  ASSERT FAILED: $1"
    ((++FAIL))
}

pass_test() {
    ((++PASS))
}

# Runs a kit script with the kubectl stub and sets STATUS and OUTPUT.
run_kit() {
    local script="$1"
    shift
    STATUS=0
    OUTPUT="$(PATH="${TEST_ROOT}/bin:${PATH}" bash "${KIT_DIR}/${script}" "$@" 2>&1)" || STATUS=$?
}

# Calls a lib.sh function in a subshell and sets STATUS and OUTPUT.
run_lib() {
    STATUS=0
    OUTPUT="$(
        # shellcheck source=hack/authz/lib.sh
        source "${KIT_DIR}/lib.sh"
        "$@" 2>&1
    )" || STATUS=$?
}

assert_status() {
    if [[ "${STATUS}" -eq "$1" ]]; then
        pass_test
    else
        fail_test "expected exit status $1, got ${STATUS}. Output:"$'\n'"${OUTPUT}"
    fi
}

assert_output() {
    if [[ "${OUTPUT}" == "$1" ]]; then
        pass_test
    else
        fail_test "expected output:"$'\n'"$1"$'\n'"got:"$'\n'"${OUTPUT}"
    fi
}

assert_output_contains() {
    if grep -Fq -- "$1" <<<"${OUTPUT}"; then
        pass_test
    else
        fail_test "expected output to contain '$1'. Output:"$'\n'"${OUTPUT}"
    fi
}

assert_calls_contain() {
    if grep -Fq -- "$1" "${MOCK_CALLS}"; then
        pass_test
    else
        fail_test "expected a kubectl call containing '$1'. Calls:"$'\n'"$(cat "${MOCK_CALLS}")"
    fi
}

assert_calls_not_contain() {
    if grep -Fq -- "$1" "${MOCK_CALLS}"; then
        fail_test "expected no kubectl call containing '$1'. Calls:"$'\n'"$(cat "${MOCK_CALLS}")"
    else
        pass_test
    fi
}

# A kubectl stub. Every call is appended to MOCK_CALLS.
#   MOCK_SECRET_KEYS     space-separated keys of the component Secret, or
#                        "absent" when the Secret does not exist.
#   MOCK_CONTEXT_EXISTS  "false" makes "config view --context" fail.
#   MOCK_POD_EXISTS      "false" makes "get pod" fail so the pod is created.
#   MOCK_KIT_MODE        the namespace's kit mode label, empty for no label,
#                        or "absent" when the namespace does not exist.
mkdir -p "${TEST_ROOT}/bin"
cat >"${TEST_ROOT}/bin/kubectl" <<'MOCK'
#!/bin/bash
set -euo pipefail
echo "$*" >>"${MOCK_CALLS}"
if [[ "$1" == "--context" ]]; then
    shift 2
fi
case "$1 $2" in
    "get secret")
        if [[ "${MOCK_SECRET_KEYS}" == "absent" ]]; then
            echo "Error from server (NotFound): secrets \"$3\" not found" >&2
            exit 1
        fi
        for arg in "$@"; do
            if [[ "${arg}" == jsonpath=* ]]; then
                key="${arg#jsonpath=\{.data.}"
                key="${key%\}}"
                key="${key//\\/}"
                if [[ " ${MOCK_SECRET_KEYS} " == *" ${key} "* ]]; then
                    printf '%s' "ZmFrZQ=="
                fi
            fi
        done
        ;;
    "get pod")
        [[ "${MOCK_POD_EXISTS:-true}" == "true" ]] || exit 1
        ;;
    "get namespace")
        if [[ "${MOCK_KIT_MODE:-}" == "absent" ]]; then
            echo "Error from server (NotFound): namespaces \"$3\" not found" >&2
            exit 1
        fi
        printf '%s' "${MOCK_KIT_MODE:-}"
        ;;
    "label namespace")
        ;;
    "wait pod" | "apply --namespace" | "delete pod")
        ;;
    "config view")
        if [[ "${MOCK_CONTEXT_EXISTS}" == "false" ]]; then
            echo "error: context not found" >&2
            exit 1
        fi
        echo "apiVersion: v1"
        ;;
    exec*)
        [[ -t 0 ]] || cat >/dev/null
        echo "exec ok"
        ;;
    *)
        echo "unexpected kubectl call: $*" >&2
        exit 1
        ;;
esac
MOCK
chmod +x "${TEST_ROOT}/bin/kubectl"
export MOCK_CALLS="${TEST_ROOT}/kubectl-calls.txt"
export MOCK_SECRET_KEYS="absent"
export MOCK_CONTEXT_EXISTS="true"
unset AUTHZ_COMPONENT_SECRET_FORMAT AUTHZ_KUBE_CONTEXT AUTHZ_CLUSTER_NAME \
    AUTHZ_RADIUS_NAMESPACE AUTHZ_ROGUE_NAMESPACE MOCK_KIT_MODE

reset_calls() {
    : >"${MOCK_CALLS}"
}

echo "Test: scripts parse with bash -n"
for script in lib.sh kit_test.sh "${SCRIPTS[@]}"; do
    if bash -n "${KIT_DIR}/${script}"; then
        pass_test
    else
        fail_test "bash -n failed for ${script}"
    fi
done

echo "Test: scripts pass shellcheck"
if command -v shellcheck >/dev/null; then
    files=()
    for script in lib.sh kit_test.sh "${SCRIPTS[@]}"; do
        files+=("${KIT_DIR}/${script}")
    done
    if (cd "${REPO_ROOT}" && shellcheck --rcfile .github/linters/.shellcheckrc "${files[@]}"); then
        pass_test
    else
        fail_test "shellcheck reported issues"
    fi
else
    echo "  skipped: shellcheck is not installed (make install-shellcheck)"
fi

echo "Test: every script prints usage for --help"
for script in "${SCRIPTS[@]}"; do
    run_kit "${script}" --help
    assert_status 0
    assert_output_contains "Usage: ${script}"
done

echo "Test: modes map to Helm values"
run_lib authz_mode_helm_values off
assert_status 0
assert_output $'global.rbac.enabled=false\nglobal.rbac.dryRun=false'
run_lib authz_mode_helm_values dryRun
assert_status 0
assert_output $'global.rbac.enabled=false\nglobal.rbac.dryRun=true'
run_lib authz_mode_helm_values enforce
assert_status 0
assert_output $'global.rbac.enabled=true\nglobal.rbac.dryRun=false'

echo "Test: unknown modes are rejected"
for mode in "" dryrun Enforce on; do
    run_lib authz_mode_helm_values "${mode}"
    assert_status 2
    assert_output_contains "expected off, dryRun, or enforce"
done

echo "Test: install values select the registry, tag, pinned images, and mode"
run_lib authz_install_set_values enforce localhost:5001 authz-abc
assert_status 0
assert_output "global.imageRegistry=localhost:5001
global.imageTag=authz-abc
de.image=ghcr.io/radius-project/deployment-engine
de.tag=latest
dashboard.image=ghcr.io/radius-project/dashboard
dashboard.tag=latest
global.rbac.enabled=true
global.rbac.dryRun=false"

echo "Test: install values require a valid mode, registry, and tag"
run_lib authz_install_set_values bogus localhost:5001 authz-abc
assert_status 2
run_lib authz_install_set_values dryRun "" authz-abc
assert_status 2
run_lib authz_install_set_values dryRun localhost:5001 ""
assert_status 2

echo "Test: component Secret names use the configurable format"
run_lib authz_component_secret_name dynamic-rp
assert_status 0
assert_output "dynamic-rp-mtls"
STATUS=0
OUTPUT="$(AUTHZ_COMPONENT_SECRET_FORMAT="radius-%s-cert" bash -c \
    'source "$1" && authz_component_secret_name ucp' _ "${KIT_DIR}/lib.sh" 2>&1)" || STATUS=$?
assert_status 0
assert_output "radius-ucp-cert"

echo "Test: invalid component names and formats are rejected"
for component in "" Bad_Name -ucp "ucp/../x"; do
    run_lib authz_component_secret_name "${component}"
    assert_status 2
    assert_output_contains "invalid component name"
done
STATUS=0
OUTPUT="$(AUTHZ_COMPONENT_SECRET_FORMAT="fixed-name" bash -c \
    'source "$1" && authz_component_secret_name ucp' _ "${KIT_DIR}/lib.sh" 2>&1)" || STATUS=$?
assert_status 2
assert_output_contains "must contain %s"

echo "Test: up.sh rejects unknown modes before touching the cluster"
reset_calls
run_kit up.sh bogus
assert_status 2
assert_output_contains "expected off, dryRun, or enforce"
assert_calls_not_contain "kubectl"

echo "Test: rogue.sh exec runs curl in the rogue pod and drops a leading curl"
reset_calls
run_kit rogue.sh exec -- curl -s http://applications-rp.radius-system:5443/healthz
assert_status 0
assert_calls_contain "--context kind-radius-authz exec radius-authz-rogue --namespace default --container curl -- curl -s http://applications-rp.radius-system:5443/healthz"

echo "Test: rogue.sh honors --namespace"
reset_calls
MOCK_POD_EXISTS=false run_kit rogue.sh --namespace team-a exec -- -s http://example
assert_status 0
assert_calls_contain "apply --namespace team-a"
assert_calls_contain "exec radius-authz-rogue --namespace team-a --container curl -- curl -s http://example"

echo "Test: rogue.sh openssl runs in the openssl container"
reset_calls
run_kit rogue.sh openssl -- version
assert_status 0
assert_calls_contain "--container openssl -- openssl version"

echo "Test: rogue.sh requires a command and arguments"
run_kit rogue.sh
assert_status 2
assert_output_contains "a command is required"
run_kit rogue.sh exec
assert_status 2
assert_output_contains "no curl arguments"
run_kit rogue.sh bogus
assert_status 2

echo "Test: as-component.sh names the missing Secret"
reset_calls
export MOCK_SECRET_KEYS="absent"
run_kit as-component.sh dynamic-rp -- -s http://applications-rp.radius-system:5443/
assert_status 3
assert_output_contains "Component certificate Secret not found: radius-system/dynamic-rp-mtls"
assert_output_contains "AUTHZ_COMPONENT_SECRET_FORMAT"
assert_output_contains "Stack A"
assert_calls_not_contain " exec "

echo "Test: as-component.sh uses AUTHZ_COMPONENT_SECRET_FORMAT"
reset_calls
STATUS=0
OUTPUT="$(AUTHZ_COMPONENT_SECRET_FORMAT="radius-%s-cert" PATH="${TEST_ROOT}/bin:${PATH}" \
    bash "${KIT_DIR}/as-component.sh" ucp -- -s http://x 2>&1)" || STATUS=$?
assert_status 3
assert_output_contains "radius-system/radius-ucp-cert"

echo "Test: as-component.sh reports a Secret without a CA"
reset_calls
export MOCK_SECRET_KEYS="tls.crt tls.key"
run_kit as-component.sh ucp -- -s http://x
assert_status 3
assert_output_contains "The Secret has no 'ca.crt' key."
assert_calls_not_contain " exec "

echo "Test: as-component.sh copies the certificate and calls curl with it"
reset_calls
export MOCK_SECRET_KEYS="tls.crt tls.key ca.crt"
run_kit as-component.sh ucp -- curl -s https://applications-rp.radius-system:5443/
assert_status 0
assert_output_contains "Calling as ucp with radius-system/ucp-mtls"
assert_calls_contain "jsonpath={.data.tls\.key}"
assert_calls_contain "sh /work/ucp tls.key"
assert_calls_contain "--container curl -- curl -s https://applications-rp.radius-system:5443/ --cert /work/ucp/tls.crt --key /work/ucp/tls.key --cacert /work/ucp/ca.crt"
export MOCK_SECRET_KEYS="absent"

echo "Test: as-component.sh validates its arguments"
run_kit as-component.sh
assert_status 2
assert_output_contains "a component is required"
run_kit as-component.sh ucp
assert_status 2
assert_output_contains "no curl arguments"
run_kit as-component.sh Bad_Name -- -s http://x
assert_status 2
assert_output_contains "invalid component name"
run_kit as-component.sh ucp dynamic-rp -- -s http://x
assert_status 2

echo "Test: records.sh is a placeholder until Stack B"
run_kit records.sh
assert_status 0
assert_output "execution records are added in Stack B"
run_kit records.sh rec-123
assert_status 0
run_kit records.sh a b
assert_status 2

echo "Test: would-deny.sh delegates to the dry-run would-deny check"
mkdir -p "${TEST_ROOT}/logs/clean" "${TEST_ROOT}/logs/would-deny"
echo '{"msg":"request","authzWouldDeny":false}' >"${TEST_ROOT}/logs/clean/ucp-0.log"
echo '{"msg":"request","authzWouldDeny":true,"authzCode":"AuthorizationFailed"}' \
    >"${TEST_ROOT}/logs/would-deny/ucp-0.log"
reset_calls
run_kit would-deny.sh --current-context --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 0
assert_output_contains "No authorization would-deny log lines found"
assert_calls_not_contain "config view"
run_kit would-deny.sh --current-context --logs-dir "${TEST_ROOT}/logs/would-deny"
assert_status 1
assert_output_contains "would-deny: ucp-0"

echo "Test: would-deny.sh pins the kit context"
reset_calls
run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 0
assert_calls_contain "config view --minify --flatten --context kind-radius-authz"

echo "Test: would-deny.sh reports a missing context"
export MOCK_CONTEXT_EXISTS="false"
run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_output_contains "kubeconfig context 'kind-radius-authz' not found"
export MOCK_CONTEXT_EXISTS="true"

echo "Test: authz_would_deny_mode_args requires dryRun only for a dryRun install"
run_lib authz_would_deny_mode_args dryRun
assert_status 0
assert_output "--require-dry-run"
for mode in off enforce ""; do
    run_lib authz_would_deny_mode_args "${mode}"
    assert_status 0
    assert_output ""
done

echo "Test: authz_record_mode labels the Radius namespace with the installed mode"
reset_calls
PATH="${TEST_ROOT}/bin:${PATH}" run_lib authz_record_mode dryRun
assert_status 0
assert_calls_contain "--context kind-radius-authz label namespace radius-system authz-kit.radius.dev/mode=dryRun --overwrite"
PATH="${TEST_ROOT}/bin:${PATH}" run_lib authz_record_mode bogus
assert_status 2
assert_calls_not_contain "mode=bogus"

echo "Test: up.sh records the installed mode"
if grep -Eq '^[[:space:]]+authz_record_mode "\$\{MODE\}"' "${KIT_DIR}/up.sh"; then
    pass_test
else
    fail_test "up.sh does not call authz_record_mode after installing"
fi

echo "Test: would-deny.sh requires dry-run startup logs for a dryRun install"
reset_calls
MOCK_KIT_MODE=dryRun run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_calls_contain "get namespace radius-system"
assert_calls_contain "get pods"
assert_output_contains "could not list pods in namespace radius-system"
reset_calls
MOCK_KIT_MODE=dryRun run_kit would-deny.sh --current-context --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_calls_contain "get pods"

echo "Test: would-deny.sh does not require dry-run for other or unknown modes"
for mode in enforce "" absent; do
    reset_calls
    MOCK_KIT_MODE="${mode}" run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
    assert_status 0
    assert_calls_not_contain "get pods"
done
MOCK_KIT_MODE=enforce run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_output_contains "installed in authz mode enforce"
MOCK_KIT_MODE="" run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_output_contains "pass --require-dry-run"

echo "Test: would-deny.sh forwards an explicit --require-dry-run once"
reset_calls
MOCK_KIT_MODE=dryRun run_kit would-deny.sh --require-dry-run --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_output_contains "could not list pods"
run_kit would-deny.sh --help
assert_output_contains "--require-dry-run"

echo "Test: README describes how would-deny.sh differs from the Make target"
# shellcheck disable=SC2016 # Backticks are literal Markdown.
readme_row="$(grep -F '| `would-deny.sh`' "${KIT_DIR}/README.md" || true)"
# shellcheck disable=SC2016 # Backticks are literal Markdown.
if [[ "${readme_row}" == *"--require-dry-run"* && "${readme_row}" != *'(`make authz-would-deny-check`)'* ]]; then
    pass_test
else
    fail_test "README would-deny.sh row must explain --require-dry-run and not claim it equals make authz-would-deny-check"
fi

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"
if [[ "${FAIL}" -gt 0 ]]; then
    exit 1
fi
