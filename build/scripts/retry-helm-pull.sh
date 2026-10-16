#!/usr/bin/env bash

# Provides retry_helm_pull for read-only Helm chart downloads.

retry_helm_pull() {
    local max_attempts="${RETRY_HELM_MAX_ATTEMPTS:-5}"
    local max_delay="${RETRY_HELM_MAX_DELAY_SECONDS:-15}"
    local attempt=1
    local delay=1
    local wait_time
    local output
    local status

    while true; do
        status=0
        output="$(helm pull "$@" 2>&1)" || status=$?
        if ((status == 0)); then
            [[ -z "${output}" ]] || printf '%s\n' "${output}"
            return 0
        fi
        printf '%s\n' "${output}" >&2
        if ! retry_helm_is_transient "${output}"; then
            return "${status}"
        fi
        if ((attempt >= max_attempts)); then
            echo "helm pull failed after ${max_attempts} attempts" >&2
            return "${status}"
        fi
        wait_time="${delay}"
        if [[ "${RETRY_HELM_NO_JITTER:-}" != "true" ]]; then
            wait_time=$((wait_time + RANDOM % (wait_time + 1)))
        fi
        echo "helm pull failed; retrying in ${wait_time}s" >&2
        sleep "${wait_time}"
        attempt=$((attempt + 1))
        delay=$((delay * 2))
        if ((delay > max_delay)); then
            delay="${max_delay}"
        fi
    done
}

retry_helm_is_transient() {
    local error
    error="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"

    [[ "${error}" =~ (^|[^[:alnum:]])http[/0-9.]*[[:space:]]+(408|429|500|502|503|504)([^0-9]|$) ]] &&
        return 0
    [[ "${error}" =~ (^|[^[:alnum:]])status([[:space:]]+code)?[^0-9]*(408|429|500|502|503|504)([^0-9]|$) ]] &&
        return 0
    [[ "${error}" =~ (timeout|timed[[:space:]]out) ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|unexpected[[:space:]]eof) ]] && return 0
    [[ "${error}" =~ (^|[^[:alpha:]])eof([^[:alpha:]]|$) ]] && return 0
    return 1
}
