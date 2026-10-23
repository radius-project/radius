#!/usr/bin/env bash

# Provides retry_curl, a curl-compatible wrapper for one HTTPS request.

retry_curl() {
    local -a curl_args=()
    local url="" output="" write_out="" headers_output=""
    local fail_flag="--fail"
    local temporary_output=false
    local argument

    while (($#)); do
        argument="$1"
        case "${argument}" in
            -o | --output)
                output="${2:-}"
                shift 2
                ;;
            -D | --dump-header)
                headers_output="${2:-}"
                shift 2
                ;;
            --fail-with-body)
                fail_flag="--fail-with-body"
                shift
                ;;
            -w | --write-out)
                write_out="${2:-}"
                shift 2
                ;;
            http://* | https://*)
                if [[ -n "${url}" ]]; then
                    echo "retry_curl supports one URL per call" >&2
                    return 2
                fi
                url="${argument}"
                shift
                ;;
            --retry | --retry-all-errors | --retry-connrefused | \
                --retry-delay | --retry-max-time)
                echo "retry_curl owns the retry policy" >&2
                return 2
                ;;
            *)
                curl_args+=("${argument}")
                shift
                ;;
        esac
    done

    if [[ -z "${url}" ]]; then
        echo "retry_curl requires an HTTPS URL" >&2
        return 2
    fi
    if [[ "${url}" != https://* ]]; then
        echo "retry_curl only supports HTTPS URLs" >&2
        return 2
    fi
    if [[ -n "${write_out}" ]]; then
        case "${write_out}" in
            '%{http_code}' | '%{url_effective}') ;;
            *)
                echo "retry_curl only supports %{http_code} or" \
                    "%{url_effective} for --write-out" >&2
                return 2
                ;;
        esac
    fi

    local temporary_directory
    temporary_directory="$(mktemp -d)"
    local headers="${temporary_directory}/headers"
    if [[ -z "${output}" ]]; then
        output="${temporary_directory}/output"
        temporary_output=true
    fi

    local max_seconds="${RETRY_CURL_MAX_SECONDS:-300}"
    local max_attempts="${RETRY_CURL_MAX_ATTEMPTS:-11}"
    local max_delay="${RETRY_CURL_MAX_DELAY_SECONDS:-60}"
    local deadline=$((SECONDS + max_seconds))
    local attempt=1
    local delay=1
    local status=0
    local result=""
    local http_code=""
    local effective_url=""
    local remaining
    local timeout
    local wait_time

    while true; do
        remaining=$((deadline - SECONDS))
        if ((remaining <= 0)); then
            echo "curl request exceeded the ${max_seconds}-second budget" >&2
            status=28
            if [[ "${fail_flag}" == --fail-with-body &&
                "${temporary_output}" == true && -f "${output}" ]]; then
                cat "${output}" || status=23
            fi
            retry_curl_write_out \
                "${write_out}" "${http_code}" "${effective_url}"
            retry_curl_cleanup "${temporary_directory}" "${headers_output}" ||
                return "$?"
            return "${status}"
        fi
        timeout="${remaining}"

        : > "${headers}"
        status=0
        if ((${#curl_args[@]})); then
            result="$(
                curl --proto '=https' --tlsv1.2 "${fail_flag}" \
                    --silent --show-error \
                    --location --connect-timeout 15 --max-time "${timeout}" \
                    --speed-limit 1024 --speed-time 30 \
                    "${curl_args[@]}" --output "${output}" \
                    --dump-header "${headers}" \
                    --write-out '%{http_code} %{url_effective}' "${url}"
            )" || status=$?
        else
            result="$(
                curl --proto '=https' --tlsv1.2 "${fail_flag}" \
                    --silent --show-error \
                    --location --connect-timeout 15 --max-time "${timeout}" \
                    --speed-limit 1024 --speed-time 30 \
                    --output "${output}" --dump-header "${headers}" \
                    --write-out '%{http_code} %{url_effective}' "${url}"
            )" || status=$?
        fi
        http_code="${result%% *}"
        effective_url="${result#* }"

        if ((status == 0)); then
            if [[ -n "${write_out}" ]]; then
                retry_curl_write_out \
                    "${write_out}" "${http_code}" "${effective_url}"
            elif [[ "${temporary_output}" == true ]]; then
                cat "${output}"
            fi
            retry_curl_cleanup "${temporary_directory}" "${headers_output}" ||
                return "$?"
            return 0
        fi

        if ! retry_curl_is_transient \
            "${status}" "${http_code}" "${headers}"; then
            if [[ "${fail_flag}" == --fail-with-body &&
                "${temporary_output}" == true && -f "${output}" ]]; then
                cat "${output}" || status=23
            fi
            retry_curl_write_out \
                "${write_out}" "${http_code}" "${effective_url}"
            retry_curl_cleanup "${temporary_directory}" "${headers_output}" ||
                return "$?"
            return "${status}"
        fi

        wait_time="$(retry_curl_wait_seconds "${headers}" "${delay}")"
        remaining=$((deadline - SECONDS))
        if ((attempt >= max_attempts || remaining <= wait_time)); then
            echo "curl request exhausted its retry budget" >&2
            if [[ "${fail_flag}" == --fail-with-body &&
                "${temporary_output}" == true && -f "${output}" ]]; then
                cat "${output}" || status=23
            fi
            retry_curl_write_out \
                "${write_out}" "${http_code}" "${effective_url}"
            retry_curl_cleanup "${temporary_directory}" "${headers_output}" ||
                return "$?"
            return "${status}"
        fi

        echo "curl failed (status ${status}, HTTP ${http_code});" \
            "retrying in ${wait_time}s" >&2
        sleep "${wait_time}"
        attempt=$((attempt + 1))
        delay=$((delay * 2))
        if ((delay > max_delay)); then
            delay="${max_delay}"
        fi
    done
}

retry_curl_cleanup() {
    local temporary_directory="$1"
    local headers_output="$2"
    local status=0

    if [[ -n "${headers_output}" ]]; then
        cp "${temporary_directory}/headers" "${headers_output}" || status=23
    fi
    rm -rf "${temporary_directory}" || return "$?"
    return "${status}"
}

retry_curl_write_out() {
    local write_out="$1"
    local http_code="$2"
    local effective_url="$3"

    case "${write_out}" in
        '%{http_code}') printf '%s' "${http_code}" ;;
        '%{url_effective}') printf '%s' "${effective_url}" ;;
    esac
}

retry_curl_is_transient() {
    local status="$1"
    local http_code="$2"
    local headers="${3:-}"

    case "${status}:${http_code}" in
        22:403)
            [[ -n "${headers}" &&
                "$(retry_curl_header_value \
                    "${headers}" x-ratelimit-remaining)" != 0 &&
                -n "$(retry_curl_header_value "${headers}" retry-after)" ]]
            return
            ;;
        22:408 | 22:429 | 22:500 | 22:502 | 22:503 | 22:504)
            return 0
            ;;
        5:* | 6:* | 7:* | 18:* | 28:* | 52:* | 55:* | 56:* | 92:*)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

retry_curl_header_value() {
    local headers="$1"
    local name="$2"

    [[ -f "${headers}" ]] || return 0
    awk -v name="${name}" '
        /^HTTP\// { value = "" }
        tolower($1) == name ":" {
            sub(/^[^:]*:[ \t]*/, "")
            sub(/[ \t\r]*$/, "")
            value = $0
        }
        END { print value }
    ' "${headers}"
}

retry_curl_wait_seconds() {
    local headers="$1"
    local delay="$2"
    local retry_after
    local retry_time=0
    local wait_time="${delay}"
    local now

    retry_after="$(retry_curl_header_value "${headers}" retry-after)"
    if [[ -n "${retry_after}" ]]; then
        case "${retry_after}" in
            *[!0-9]*)
                now="$(LC_ALL=C date -u +%s)"
                if retry_time="$(
                    LC_ALL=C date -u -d "${retry_after}" +%s 2>/dev/null
                )" || retry_time="$(
                    LC_ALL=C date -j -u \
                        -f '%a, %d %b %Y %H:%M:%S GMT' \
                        "${retry_after}" +%s 2>/dev/null
                )"; then
                    retry_time=$((retry_time - now))
                else
                    retry_time=0
                fi
                ;;
            *)
                while [[ "${retry_after}" == 0* ]]; do
                    retry_after="${retry_after#0}"
                done
                if ((${#retry_after} > 3)); then
                    retry_time=300
                else
                    retry_time=$((10#${retry_after:-0}))
                fi
                ;;
        esac
    fi
    if ((retry_time > wait_time)); then
        wait_time="${retry_time}"
    fi
    if [[ "${RETRY_CURL_NO_JITTER:-}" != "true" ]] &&
        ((wait_time == delay)); then
        wait_time=$((wait_time + RANDOM % (wait_time + 1)))
    fi
    printf '%s\n' "${wait_time}"
}
