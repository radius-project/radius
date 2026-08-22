#!/bin/bash

# Tests refresh-azure-oidc-token.sh without cloud or cluster access.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
readonly REPO_ROOT
readonly SCRIPT="${SCRIPT_DIR}/refresh-azure-oidc-token.sh"
readonly COMMAND_ACTION="${REPO_ROOT}/.github/extension/actions/run-rad-commands/action.yml"
readonly DELETE_ACTION="${REPO_ROOT}/.github/extension/actions/delete-resource/action.yml"
readonly AZURE_COMMAND_WORKFLOW="${REPO_ROOT}/.github/extension/run-rad-commands-azure.yml"
readonly AZURE_DELETE_WORKFLOW="${REPO_ROOT}/.github/extension/delete-azure.yml"
TEMP_DIR="$(mktemp -d)"
readonly TEMP_DIR
trap 'rm -rf "${TEMP_DIR}"' EXIT

mkdir -p "${TEMP_DIR}/bin"

cat >"${TEMP_DIR}/bin/curl" <<'EOF'
#!/bin/bash
set -euo pipefail
count=0
if [[ -f "${CURL_COUNT_FILE}" ]]; then
    count="$(cat "${CURL_COUNT_FILE}")"
fi
((count += 1))
printf '%s' "${count}" >"${CURL_COUNT_FILE}"
printf '%s\n' "$*" >>"${CURL_ARGS_FILE}"
printf '{"value":"token-%s"}\n' "${count}"
EOF

cat >"${TEMP_DIR}/bin/kubectl" <<'EOF'
#!/bin/bash
set -euo pipefail
if [[ "${1:-}" == "create" ]]; then
    for argument in "$@"; do
        case "${argument}" in
            --from-literal=*)
                printf '%s\n' "${argument#*=}"
                exit 0
                ;;
        esac
    done
    exit 1
fi

if [[ "${1:-}" == "apply" ]]; then
    printf '%s\n' "$*" >>"${KUBECTL_ARGS_FILE}"
    cat >>"${KUBECTL_APPLY_FILE}"
    exit 0
fi

if [[ "${1:-}" == "rollout" ]]; then
    printf '%s\n' "$*" >>"${KUBECTL_ROLLOUT_FILE}"
    exit 0
fi

exit 1
EOF

cat >"${TEMP_DIR}/bin/sleep" <<'EOF'
#!/bin/bash
exit 0
EOF

chmod +x "${TEMP_DIR}/bin/curl" \
    "${TEMP_DIR}/bin/kubectl" \
    "${TEMP_DIR}/bin/sleep"

ORIGINAL_PATH="${PATH}"
readonly ORIGINAL_PATH
export PATH="${TEMP_DIR}/bin:${PATH}"
export ACTIONS_ID_TOKEN_REQUEST_TOKEN="request-token"
export ACTIONS_ID_TOKEN_REQUEST_URL="https://oidc.example/token?job=1"
export AZURE_OIDC_REFRESH_INTERVAL_SECONDS=1
export CURL_COUNT_FILE="${TEMP_DIR}/curl-count"
export CURL_ARGS_FILE="${TEMP_DIR}/curl-args"
export KUBECTL_APPLY_FILE="${TEMP_DIR}/kubectl-apply"
export KUBECTL_ARGS_FILE="${TEMP_DIR}/kubectl-args"
export KUBECTL_ROLLOUT_FILE="${TEMP_DIR}/kubectl-rollout"

output="$(bash "${SCRIPT}" --prepare)"
output+="$(bash "${SCRIPT}" --watch 2)"

if [[ "$(cat "${CURL_COUNT_FILE}")" != "3" ]]; then
    echo "Expected three OIDC token requests." >&2
    exit 1
fi

if [[ "$(grep -c 'audience=api://AzureADTokenExchange' \
    "${CURL_ARGS_FILE}")" != "3" ]]; then
    echo "Expected every request to use the Azure token-exchange audience." >&2
    exit 1
fi
if [[ "$(grep -c -- '--connect-timeout 5 --max-time 20' \
    "${CURL_ARGS_FILE}")" != "3" ]]; then
    echo "Expected every OIDC request to have finite network timeouts." >&2
    exit 1
fi
if [[ "$(grep -c -- '--request-timeout=20s' \
    "${KUBECTL_ARGS_FILE}")" != "3" ]]; then
    echo "Expected every Secret update to have a finite request timeout." >&2
    exit 1
fi

expected_updates=$'azure-identity-token=token-1\n'
expected_updates+=$'azure-identity-token=token-2\n'
expected_updates+=$'azure-identity-token=token-3'
if [[ "$(cat "${KUBECTL_APPLY_FILE}")" != "${expected_updates}" ]]; then
    echo "Expected each refreshed token to update the Kubernetes Secret." >&2
    exit 1
fi

if [[ "${output}" == *"token-"* || "${output}" == *"request-token"* ]]; then
    echo "Token value leaked to standard output." >&2
    exit 1
fi

if [[ "$(grep -c '^rollout restart deployment/' \
    "${KUBECTL_ROLLOUT_FILE}")" != "3" ]]; then
    echo "Expected all Azure token consumers to restart after initial refresh." >&2
    exit 1
fi
if [[ "$(grep -c '^rollout status deployment/' \
    "${KUBECTL_ROLLOUT_FILE}")" != "3" ]]; then
    echo "Expected the initial refresh to wait for all Azure token consumers." >&2
    exit 1
fi

prepare_invocation="\"\$TOKEN_REFRESH_SCRIPT\" --prepare"
watch_invocation="\"\$TOKEN_REFRESH_SCRIPT\" --watch &"
stop_invocation="kill \"\$OIDC_REFRESH_PID\""
for action in "${COMMAND_ACTION}" "${DELETE_ACTION}"; do
    grep -Fq 'azure-oidc-token-refresh:' "${action}" || {
        echo "Expected $(basename "$(dirname "${action}")") to declare the refresh input." >&2
        exit 1
    }
    grep -Fq "${prepare_invocation}" "${action}" || {
        echo "Expected $(basename "$(dirname "${action}")") to prepare token consumers." >&2
        exit 1
    }
    grep -Fq "${watch_invocation}" "${action}" || {
        echo "Expected $(basename "$(dirname "${action}")") to start the token watcher." >&2
        exit 1
    }
    grep -Fq "${stop_invocation}" "${action}" || {
        echo "Expected $(basename "$(dirname "${action}")") to stop the token watcher." >&2
        exit 1
    }
done

for workflow in "${AZURE_COMMAND_WORKFLOW}" "${AZURE_DELETE_WORKFLOW}"; do
    grep -Fq 'azure-oidc-token-refresh:' "${workflow}" || {
        echo "Expected $(basename "${workflow}") to enable token refresh." >&2
        exit 1
    }
done

validation_pattern="if ! is_allowed \"\$cmd\""
validation_line="$(grep -n "${validation_pattern}" "${COMMAND_ACTION}" | cut -d: -f1)"
prepare_line="$(grep -n 'if ! start_azure_oidc_refresh; then' \
    "${COMMAND_ACTION}" | head -1 | cut -d: -f1)"
if [[ -z "${validation_line}" || -z "${prepare_line}" ||
    "${prepare_line}" -le "${validation_line}" ]]; then
    echo "Expected command validation before Azure token preparation." >&2
    exit 1
fi

# A cancelled watcher must stop immediately instead of waiting out its refresh
# interval, otherwise the action's cleanup cannot finalize the result artifact.
idle_bin="${TEMP_DIR}/idle-bin"
mkdir -p "${idle_bin}"
cp "${TEMP_DIR}/bin/curl" "${TEMP_DIR}/bin/kubectl" "${idle_bin}/"
AZURE_OIDC_REFRESH_INTERVAL_SECONDS=300 PATH="${idle_bin}:${ORIGINAL_PATH}" \
    bash "${SCRIPT}" --watch &
watcher_pid=$!
/bin/sleep 0.5
kill "${watcher_pid}"

stopped=0
for _ in $(seq 1 60); do
    if ! kill -0 "${watcher_pid}" 2>/dev/null; then
        stopped=1
        break
    fi
    /bin/sleep 0.05
done
wait "${watcher_pid}" 2>/dev/null || true

if [[ "${stopped}" != "1" ]]; then
    echo "Expected a cancelled watcher to stop without waiting for its interval." >&2
    exit 1
fi

echo "refresh-azure-oidc-token tests passed"
