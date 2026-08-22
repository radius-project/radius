#!/bin/bash

# Refreshes the GitHub OIDC assertion mounted into Radius Azure consumers.

set -euo pipefail

readonly MODE="${1:-}"
readonly MAX_REFRESHES="${2:-0}"
readonly AUDIENCE="api://AzureADTokenExchange"
readonly NAMESPACE="radius-system"
readonly SECRET_NAME="azure-oidc-token"
readonly TOKEN_KEY="azure-identity-token"
readonly CONSUMERS=(applications-rp dynamic-rp bicep-de)
# GitHub Actions OIDC tokens expire after 5 minutes, so refresh well inside that
# window and leave room for kubelet to propagate the updated Secret volume.
readonly REFRESH_INTERVAL_SECONDS="${AZURE_OIDC_REFRESH_INTERVAL_SECONDS:-120}"
readonly CONNECT_TIMEOUT_SECONDS=5
readonly REQUEST_TIMEOUT_SECONDS=20
SLEEP_PID=""

# The watcher is idle between refreshes and bash defers traps until the current
# foreground command returns, so cancel the sleep instead of waiting it out.
stop_watcher() {
    if [[ -n "${SLEEP_PID}" ]]; then
        kill "${SLEEP_PID}" 2>/dev/null || true
    fi
    exit 0
}

for variable in ACTIONS_ID_TOKEN_REQUEST_TOKEN ACTIONS_ID_TOKEN_REQUEST_URL; do
    if [[ -z "${!variable:-}" ]]; then
        echo "Missing required environment variable: ${variable}" >&2
        exit 1
    fi
done

if [[ ! "${REFRESH_INTERVAL_SECONDS}" =~ ^[1-9][0-9]*$ ]]; then
    echo "AZURE_OIDC_REFRESH_INTERVAL_SECONDS must be a positive integer." >&2
    exit 1
fi

if [[ ! "${MAX_REFRESHES}" =~ ^[0-9]+$ ]]; then
    echo "Refresh count must be a non-negative integer." >&2
    exit 1
fi

refresh_token() {
    local token

    token="$(curl -fsS \
        --connect-timeout "${CONNECT_TIMEOUT_SECONDS}" \
        --max-time "${REQUEST_TIMEOUT_SECONDS}" \
        -H "Authorization: bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" \
        "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=${AUDIENCE}" |
        jq -er '.value | strings | select(length > 0)')" || return 1

    kubectl create secret generic "${SECRET_NAME}" \
        --namespace "${NAMESPACE}" \
        --from-literal="${TOKEN_KEY}=${token}" \
        --dry-run=client -o yaml |
        kubectl apply --request-timeout="${REQUEST_TIMEOUT_SECONDS}s" -f - >/dev/null
}

restart_consumers() {
    local deployment

    for deployment in "${CONSUMERS[@]}"; do
        kubectl rollout restart "deployment/${deployment}" \
            --namespace "${NAMESPACE}" >/dev/null
    done
    for deployment in "${CONSUMERS[@]}"; do
        kubectl rollout status "deployment/${deployment}" \
            --namespace "${NAMESPACE}" --timeout=300s
    done
}

case "${MODE}" in
    --prepare)
        refresh_token
        restart_consumers
        echo "Azure OIDC token refreshed and consumers restarted."
        ;;
    --watch)
        trap stop_watcher TERM INT
        refreshes=0
        while true; do
            sleep "${REFRESH_INTERVAL_SECONDS}" &
            SLEEP_PID=$!
            wait "${SLEEP_PID}" || true
            SLEEP_PID=""
            if refresh_token; then
                echo "Azure OIDC token refreshed."
            else
                echo "::warning::Azure OIDC token refresh failed; retrying." >&2
            fi

            ((refreshes += 1))
            if ((MAX_REFRESHES > 0 && refreshes >= MAX_REFRESHES)); then
                break
            fi
        done
        ;;
    *)
        echo "Usage: $0 --prepare | --watch [refresh-count]" >&2
        exit 2
        ;;
esac
