#!/usr/bin/env bash

# Provides retry_helm_pull for read-only Helm chart downloads.

retry_helm_pull() {
    local max_attempts="${RETRY_HELM_MAX_ATTEMPTS:-5}"
    local max_delay="${RETRY_HELM_MAX_DELAY_SECONDS:-15}"
    local max_seconds="${RETRY_HELM_MAX_SECONDS:-300}"
    local attempt_timeout="${RETRY_HELM_ATTEMPT_TIMEOUT_SECONDS:-60}"
    local deadline=$((SECONDS + max_seconds))
    local attempt=1
    local delay=1
    local wait_time
    local output
    local remaining
    local status

    while true; do
        remaining=$((deadline - SECONDS))
        if ((remaining <= 0)); then
            echo "helm pull exceeded the ${max_seconds}-second budget" >&2
            return 124
        fi
        if ((attempt_timeout > remaining)); then
            attempt_timeout="${remaining}"
        fi

        status=0
        output="$(retry_helm_run "${attempt_timeout}" "$@")" || status=$?
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
        remaining=$((deadline - SECONDS))
        if ((remaining <= wait_time)); then
            echo "helm pull exhausted its retry budget" >&2
            return "${status}"
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

retry_helm_run() {
    local timeout_seconds="$1"
    shift
    local output_file
    output_file="$(mktemp)"
    local status=0
    local pid
    local deadline=$((SECONDS + timeout_seconds))

    helm pull "$@" > "${output_file}" 2>&1 &
    pid=$!
    while kill -0 "${pid}" 2>/dev/null; do
        if ((SECONDS >= deadline)); then
            kill "${pid}" 2>/dev/null || true
            wait "${pid}" 2>/dev/null || true
            echo "helm pull timed out after ${timeout_seconds}s" \
                >> "${output_file}"
            cat "${output_file}"
            rm -f "${output_file}"
            return 124
        fi
        command sleep 1
    done
    wait "${pid}" || status=$?
    cat "${output_file}"
    rm -f "${output_file}"
    return "${status}"
}

retry_helm_is_transient() {
    local error
    error="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"

    [[ "${error}" =~ (^|[^[:alnum:]])http[/0-9.]*[[:space:]]+(408|429|500|502|503|504)([^0-9]|$) ]] &&
        return 0
    [[ "${error}" =~ (^|[^[:alnum:]])status([[:space:]]+code)?[^0-9]*(408|429|500|502|503|504)([^0-9]|$) ]] &&
        return 0
    [[ "${error}" =~ failed[[:space:]]to[[:space:]](fetch|download).*:[[:space:]]+(408|429|500|502|503|504)([^0-9]|$) ]] &&
        return 0
    [[ "${error}" =~ (timeout|timed[[:space:]]out) ]] && return 0
    [[ "${error}" =~ connection[[:space:]](reset|refused) ]] && return 0
    [[ "${error}" =~ (temporary|unexpected[[:space:]]eof) ]] && return 0
    [[ "${error}" =~ (^|[^[:alpha:]])eof([^[:alpha:]]|$) ]] && return 0
    return 1
}
