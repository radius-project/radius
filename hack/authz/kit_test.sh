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
# cluster: external tools are replaced by stubs that record their calls.
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

assert_output_not_contains() {
    if grep -Fq -- "$1" <<<"${OUTPUT}"; then
        fail_test "expected output not to contain '$1'. Output:"$'\n'"${OUTPUT}"
    else
        pass_test
    fi
}

assert_calls_contain() {
    if grep -Fq -- "$1" "${MOCK_CALLS}"; then
        pass_test
    else
        fail_test "expected a call containing '$1'. Calls:"$'\n'"$(cat "${MOCK_CALLS}")"
    fi
}

assert_calls_not_contain() {
    if grep -Fq -- "$1" "${MOCK_CALLS}"; then
        fail_test "expected no call containing '$1'. Calls:"$'\n'"$(cat "${MOCK_CALLS}")"
    else
        pass_test
    fi
}

assert_calls_ordered() {
    if awk -v first="$1" -v second="$2" '
        index($0, second) { if (seen) ordered = 1; else exit 1 }
        index($0, first) { seen = 1 }
        END { exit !ordered }
    ' "${MOCK_CALLS}"; then
        pass_test
    else
        fail_test "expected '$1' before '$2'. Calls:"$'\n'"$(cat "${MOCK_CALLS}")"
    fi
}

# A kubectl stub. Every call is appended to MOCK_CALLS.
#   MOCK_SECRET_KEYS     space-separated keys of the component Secret, or
#                        absent/timeout/forbidden/context for read errors.
#   MOCK_CONTEXT_EXISTS  "false" makes "config view --context" fail.
#   MOCK_POD_EXISTS      "false" returns no pod so the pod is created.
#   MOCK_KIT_MODE        the namespace's kit mode label, empty for no label,
#                        or absent/timeout/forbidden for namespace read errors.
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
        case "${MOCK_SECRET_KEYS}" in
            timeout | forbidden | context)
                echo "Error reading Secret: ${MOCK_SECRET_KEYS}" >&2
                exit 1
                ;;
        esac
        if [[ "${MOCK_SECRET_KEYS}" == "absent" ]]; then
            for arg in "$@"; do
                [[ "${arg}" != "--ignore-not-found" ]] || exit 0
            done
            echo "Error from server (NotFound): secrets \"$3\" not found" >&2
            exit 1
        fi
        for arg in "$@"; do
            if [[ "${arg}" == name ]]; then
                printf 'secret/%s\n' "$3"
            fi
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
        if [[ "${MOCK_POD_EXISTS:-true}" == "true" ]]; then
            echo "${MOCK_POD_IMAGE:-localhost:5001/rogue-client:authz-test}"
        fi
        ;;
    "get configmap")
        if [[ "${MOCK_IMAGE_MISSING:-false}" == true ]]; then
            echo "configmap not found" >&2
            exit 1
        fi
        echo "localhost:5001/rogue-client:authz-test"
        ;;
    "create configmap")
        echo "apiVersion: v1"
        ;;
    "get namespace")
        if [[ -n "${MOCK_KUBECONFIG_PATH:-}" ]]; then
            printf '%s\n' "${KUBECONFIG}" >"${MOCK_KUBECONFIG_PATH}"
            permissions="$(LC_ALL=C ls -l "${KUBECONFIG}")"
            [[ "${permissions:0:10}" == "-rw-------" ]] || {
                echo "kubeconfig must have mode 0600" >&2
                exit 1
            }
        fi
        case "${MOCK_KIT_MODE:-}" in
            timeout | forbidden)
                echo "Error from server: ${MOCK_KIT_MODE}" >&2
                exit 1
                ;;
        esac
        if [[ "${MOCK_KIT_MODE:-}" == "absent" ]]; then
            echo "Error from server (NotFound): namespaces \"$3\" not found" >&2
            exit 1
        fi
        printf '%s' "${MOCK_KIT_MODE:-}"
        ;;
    "label namespace")
        ;;
    "apply --filename")
        cat >/dev/null
        ;;
    "rollout status")
        ;;
    "logs deployment/"*)
        echo "authz mode=${MOCK_INSTALL_MODE}"
        ;;
    "apply --namespace")
        cat >"${MOCK_MANIFEST}"
        ;;
    "wait pod" | "delete pod")
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
        echo "radius-rogue-status:${MOCK_HTTP_STATUS:-200}"
        exit "${MOCK_REQUEST_STATUS:-0}"
        ;;
    *)
        echo "unexpected kubectl call: $*" >&2
        exit 1
        ;;
esac
MOCK
chmod +x "${TEST_ROOT}/bin/kubectl"
cat >"${TEST_ROOT}/bin/workflow-tool" <<'MOCK'
#!/bin/bash
set -euo pipefail
tool="${0##*/}"
echo "${tool} $*" >>"${MOCK_CALLS}"
case "${tool} $*" in
    "docker info")
        ;;
    "docker info --format {{.Architecture}}")
        if [[ "${MOCK_ARCHITECTURE}" == unsupported ]]; then
            echo "can't evaluate field Architecture" >&2
            exit 1
        fi
        echo "${MOCK_ARCHITECTURE}"
        ;;
    "uname -m")
        echo arm64
        ;;
    "make -C "*)
        exit "${MOCK_BUILD_STATUS:-0}"
        ;;
    "docker inspect --format {{.State.Running}} "*)
        echo true
        ;;
    "docker inspect --format {{json .NetworkSettings.Networks.kind}} "*)
        echo '{}'
        ;;
    "docker exec --interactive "*)
        cat >/dev/null
        ;;
    "docker exec "*)
        ;;
    "kind get clusters")
        echo radius-authz
        ;;
    "kind get nodes --name radius-authz")
        echo radius-authz-control-plane
        ;;
    "rad install kubernetes "*)
        if [[ "${MOCK_INSTALL_STATUS:-0}" != 0 ]]; then
            echo "Radius install failed" >&2
            exit "${MOCK_INSTALL_STATUS}"
        fi
        ;;
    *)
        echo "unexpected ${tool} call: $*" >&2
        exit 1
        ;;
esac
MOCK
chmod +x "${TEST_ROOT}/bin/workflow-tool"
for tool in docker kind make rad helm go uname; do
    ln -s workflow-tool "${TEST_ROOT}/bin/${tool}"
done
export MOCK_CALLS="${TEST_ROOT}/kubectl-calls.txt"
export MOCK_MANIFEST="${TEST_ROOT}/rogue-pod.yaml"
export MOCK_SECRET_KEYS="absent"
export MOCK_CONTEXT_EXISTS="true"
unset AUTHZ_COMPONENT_SECRET_FORMAT AUTHZ_KUBE_CONTEXT AUTHZ_CLUSTER_NAME \
    AUTHZ_RADIUS_NAMESPACE AUTHZ_ROGUE_NAMESPACE MOCK_KIT_MODE \
    MOCK_KUBECONFIG_PATH RAD AUTHZ_REGISTRY_NAME AUTHZ_REGISTRY_PORT \
    MOCK_INSTALL_STATUS MOCK_ARCHITECTURE MOCK_BUILD_STATUS \
    AUTHZ_ROGUE_IMAGE MOCK_POD_IMAGE MOCK_IMAGE_MISSING MOCK_REQUEST_STATUS \
    MOCK_HTTP_STATUS

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
de.tag=edge
dashboard.image=ghcr.io/radius-project/dashboard
dashboard.tag=edge
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

echo "Test: rogue.sh exec runs the shell-free request client"
reset_calls
run_kit rogue.sh exec -- -H 'X-Test: one' -H 'X-Test: two' --max-time 5s http://applications-rp.radius-system:5443/healthz
assert_status 0
assert_calls_contain "--context kind-radius-authz exec radius-authz-rogue --namespace default --container rogue -- /rogue-client request -H X-Test: one -H X-Test: two --max-time 5s http://applications-rp.radius-system:5443/healthz"
assert_output_contains "radius-rogue-status:200"

echo "Test: rogue.sh honors --namespace"
reset_calls
MOCK_POD_EXISTS=false run_kit rogue.sh --namespace team-a exec -- http://example
assert_status 0
assert_calls_contain "apply --namespace team-a"
assert_calls_contain "exec radius-authz-rogue --namespace team-a --container rogue -- /rogue-client request http://example"
if grep -Fq 'image: localhost:5001/rogue-client:authz-test' "${MOCK_MANIFEST}" &&
    ! grep -Fq AUTHZ_ROGUE_IMAGE "${MOCK_MANIFEST}"; then
    pass_test
else
    fail_test "rogue pod must use the recorded, rendered image"
fi

echo "Test: rogue.sh refuses stale pods and missing image settings"
reset_calls
MOCK_POD_IMAGE=old-image run_kit rogue.sh up
assert_status 2
assert_output_contains "down, then retry"
assert_calls_not_contain "wait pod"
MOCK_IMAGE_MISSING=true run_kit rogue.sh up
assert_status 2
assert_output_contains "run hack/authz/up.sh first"
AUTHZ_ROGUE_IMAGE='bad|image' run_kit rogue.sh up
assert_status 2
assert_output_contains "invalid rogue-client image"

echo "Test: request status and HTTP error responses propagate"
for code in 1 2; do
    MOCK_REQUEST_STATUS="${code}" run_kit rogue.sh exec -- http://example
    assert_status "${code}"
done
MOCK_HTTP_STATUS=403 run_kit rogue.sh exec -- http://example
assert_status 0
assert_output_contains "radius-rogue-status:403"

echo "Test: rogue.sh openssl runs in the openssl container"
reset_calls
run_kit rogue.sh openssl -- version
assert_status 0
assert_calls_contain "--container openssl -- openssl version"

echo "Test: rogue.sh down waits for deletion and tolerates an absent pod"
reset_calls
run_kit rogue.sh --namespace team-a down
assert_status 0
assert_calls_contain "delete pod radius-authz-rogue --namespace team-a --ignore-not-found --wait=true"

echo "Test: rogue.sh requires a command and arguments"
run_kit rogue.sh
assert_status 2
assert_output_contains "a command is required"
run_kit rogue.sh exec
assert_status 2
assert_output_contains "no request arguments"
run_kit rogue.sh bogus
assert_status 2

echo "Test: as-component.sh names the missing Secret"
reset_calls
export MOCK_SECRET_KEYS="absent"
run_kit as-component.sh dynamic-rp -- http://applications-rp.radius-system:5443/
assert_status 3
assert_output_contains "Component certificate Secret not found: radius-system/dynamic-rp-mtls"
assert_output_contains "AUTHZ_COMPONENT_SECRET_FORMAT"
assert_output_contains "Stack A"
assert_calls_not_contain " exec "

echo "Test: as-component.sh preserves Secret read errors"
for error in timeout forbidden context; do
    reset_calls
    MOCK_SECRET_KEYS="${error}" run_kit as-component.sh ucp -- http://x
    assert_status 1
    assert_output_contains "Error reading Secret: ${error}"
    assert_output_not_contains "Secret not found"
    assert_output_not_contains "Stack A"
    assert_calls_not_contain "get pod"
    assert_calls_not_contain " exec "
done

echo "Test: as-component.sh uses AUTHZ_COMPONENT_SECRET_FORMAT"
reset_calls
STATUS=0
OUTPUT="$(AUTHZ_COMPONENT_SECRET_FORMAT="radius-%s-cert" PATH="${TEST_ROOT}/bin:${PATH}" \
    bash "${KIT_DIR}/as-component.sh" ucp -- http://x 2>&1)" || STATUS=$?
assert_status 3
assert_output_contains "radius-system/radius-ucp-cert"

echo "Test: as-component.sh reports a Secret without a CA"
reset_calls
export MOCK_SECRET_KEYS="tls.crt tls.key"
run_kit as-component.sh ucp -- http://x
assert_status 3
assert_output_contains "The Secret has no 'ca.crt' key."
assert_calls_not_contain " exec "

echo "Test: as-component.sh streams through OpenSSL and puts cert flags before URL"
reset_calls
export MOCK_SECRET_KEYS="tls.crt tls.key ca.crt"
run_kit as-component.sh ucp -- https://applications-rp.radius-system:5443/
assert_status 0
assert_output_contains "Calling as ucp with radius-system/ucp-mtls"
assert_calls_contain "jsonpath={.data.tls\.key}"
assert_calls_contain "sh /work/ucp tls.key"
assert_calls_contain "--container openssl -- sh -c"
assert_calls_not_contain "--container rogue -- sh"
assert_calls_contain "--container rogue -- /rogue-client request --cert /work/ucp/tls.crt --key /work/ucp/tls.key --cacert /work/ucp/ca.crt https://applications-rp.radius-system:5443/"
export MOCK_SECRET_KEYS="absent"

echo "Test: as-component.sh validates its arguments"
run_kit as-component.sh
assert_status 2
assert_output_contains "a component is required"
run_kit as-component.sh ucp
assert_status 2
assert_output_contains "no request arguments"
run_kit as-component.sh Bad_Name -- http://x
assert_status 2
assert_output_contains "invalid component name"
run_kit as-component.sh ucp dynamic-rp -- http://x
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
assert_calls_contain "config view --raw --minify --flatten --context kind-radius-authz"

echo "Test: pinned kubeconfig is private and removed on success and failure"
for mode in "" timeout; do
    saved_umask="$(umask)"
    umask 000
    MOCK_KUBECONFIG_PATH="${TEST_ROOT}/kubeconfig-path" \
        MOCK_KIT_MODE="${mode}" run_kit would-deny.sh \
        --logs-dir "${TEST_ROOT}/logs/clean"
    umask "${saved_umask}"
    if [[ -z "${mode}" ]]; then
        assert_status 0
    else
        assert_status 2
        assert_output_contains "Error from server: timeout"
    fi
    config_path="$(cat "${TEST_ROOT}/kubeconfig-path")"
    if [[ ! -e "${config_path%/*}" ]]; then
        pass_test
    else
        fail_test "temporary kubeconfig directory was not removed"
    fi
done

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

echo "Test: up.sh records each mode after installation and before rollout"
for mode in off dryRun enforce; do
    reset_calls
    MOCK_INSTALL_MODE="${mode}" run_kit up.sh "${mode}" --skip-build --tag authz-test
    assert_status 0
    assert_calls_contain "rad install kubernetes --kubecontext kind-radius-authz --chart ${REPO_ROOT}/deploy/Chart --reinstall"
    assert_calls_contain "--set global.imageTag=authz-test"
    case "${mode}" in
        off) values="false false" ;;
        dryRun) values="false true" ;;
        enforce) values="true false" ;;
    esac
    assert_calls_contain "--set global.rbac.enabled=${values% *} --set global.rbac.dryRun=${values#* }"
    assert_calls_contain "label namespace radius-system authz-kit.radius.dev/mode=${mode} --overwrite"
    assert_calls_ordered "rad install kubernetes" "label namespace"
    assert_calls_ordered "label namespace" "rollout status"
    assert_calls_contain "--from-literal=rogue-image=localhost:5001/rogue-client:authz-test"
    assert_calls_not_contain "make "
done

echo "Test: up.sh does not record a mode or start rollout after install failure"
reset_calls
MOCK_INSTALL_STATUS=1 run_kit up.sh enforce --skip-build --tag authz-test
assert_status 1
assert_output_contains "Radius install failed"
assert_calls_not_contain "label namespace"
assert_calls_not_contain "rollout status"

echo "Test: build architecture detection supports Docker and Podman"
mkdir -p "${TEST_ROOT}/build-repo/hack/authz"
cp "${KIT_DIR}/up.sh" "${KIT_DIR}/lib.sh" "${TEST_ROOT}/build-repo/hack/authz/"
for architecture in x86_64 amd64 unsupported; do
    reset_calls
    STATUS=0
    OUTPUT="$(PATH="${TEST_ROOT}/bin:${PATH}" \
        MOCK_ARCHITECTURE="${architecture}" MOCK_INSTALL_MODE=dryRun \
        bash "${TEST_ROOT}/build-repo/hack/authz/up.sh" dryRun \
        --tag authz-test 2>&1)" || STATUS=$?
    assert_status 0
    assert_output_not_contains "can't evaluate field Architecture"
    assert_calls_ordered "make -C " "rad install kubernetes"
    assert_calls_contain "docker-build-rogue-client docker-push-rogue-client"
    if [[ "${architecture}" == unsupported ]]; then
        assert_calls_contain "uname -m"
        assert_output_contains "amd64 emulation"
    else
        assert_calls_not_contain "uname -m"
        assert_output_not_contains "amd64 emulation"
    fi
done

echo "Test: architecture fallback does not hide a build failure"
reset_calls
STATUS=0
OUTPUT="$(PATH="${TEST_ROOT}/bin:${PATH}" MOCK_ARCHITECTURE=unsupported \
    MOCK_BUILD_STATUS=1 bash "${TEST_ROOT}/build-repo/hack/authz/up.sh" \
    dryRun --tag authz-test 2>&1)" || STATUS=$?
assert_status 1
assert_calls_not_contain "rad install kubernetes"

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
for mode in off enforce ""; do
    reset_calls
    MOCK_KIT_MODE="${mode}" run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
    assert_status 0
    assert_calls_not_contain "get pods"
done
MOCK_KIT_MODE=enforce run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_output_contains "installed in authz mode enforce"
MOCK_KIT_MODE="" run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_output_contains "pass --require-dry-run"

echo "Test: would-deny.sh accepts an explicit --require-dry-run"
reset_calls
MOCK_KIT_MODE=dryRun run_kit would-deny.sh --require-dry-run --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_output_contains "could not list pods"
run_kit would-deny.sh --help
assert_output_contains "--require-dry-run"

echo "Test: namespace read errors cannot bypass live verification"
for mode in timeout forbidden absent; do
    reset_calls
    MOCK_KIT_MODE="${mode}" run_kit would-deny.sh \
        --logs-dir "${TEST_ROOT}/logs/clean"
    assert_status 2
    assert_output_contains "Error from server"
    assert_output_contains "could not read kit mode"
    assert_calls_not_contain "get pods"
done
reset_calls
MOCK_KIT_MODE=timeout run_kit would-deny.sh --current-context \
    --require-dry-run --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_output_contains "could not read kit mode"
assert_calls_not_contain "get pods"

echo "Test: flag text in a path cannot suppress live verification"
mkdir -p "${TEST_ROOT}/logs/a --require-dry-run b"
cp "${TEST_ROOT}/logs/clean/ucp-0.log" \
    "${TEST_ROOT}/logs/a --require-dry-run b/ucp-0.log"
reset_calls
MOCK_KIT_MODE=dryRun run_kit would-deny.sh \
    --logs-dir "${TEST_ROOT}/logs/a --require-dry-run b"
assert_status 2
assert_calls_contain "get pods"
assert_output_contains "could not list pods in namespace radius-system"

echo "Test: namespace overrides are rejected before contacting the cluster"
for arg in --namespace --namespace=another-namespace; do
    reset_calls
    run_kit would-deny.sh "${arg}" another-namespace
    assert_status 2
    assert_output_contains "use AUTHZ_RADIUS_NAMESPACE"
    assert_calls_not_contain "config view"
    assert_calls_not_contain "get namespace"
done

echo "Test: configured namespace is shared by mode detection and log collection"
reset_calls
AUTHZ_RADIUS_NAMESPACE=another-namespace MOCK_KIT_MODE=dryRun \
    run_kit would-deny.sh --logs-dir "${TEST_ROOT}/logs/clean"
assert_status 2
assert_calls_contain "get namespace another-namespace"
assert_calls_contain "get pods --namespace another-namespace"

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"
if [[ "${FAIL}" -gt 0 ]]; then
    exit 1
fi
