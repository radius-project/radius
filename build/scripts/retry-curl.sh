#!/usr/bin/env bash

# Provides retry_curl, a curl-compatible wrapper for one HTTPS request.

retry_curl() {
    local -a curl_args=()
    local url="" output="" write_out=""
    local temporary_output=false
    local argument

    while (($#)); do
        argument="$1"
        case "${argument}" in
            -o | --output)
                output="${2:-}"
                shift 2
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
    if [[ -n "${write_out}" && "${write_out}" != '%{url_effective}' ]]; then
        echo "retry_curl only supports %{url_effective} for --write-out" >&2
        return 2
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
            rm -rf "${temporary_directory}"
            return 28
        fi
        timeout=60
        if ((remaining < timeout)); then
            timeout="${remaining}"
        fi

        : > "${headers}"
        status=0
        result="$(
            curl --proto '=https' --tlsv1.2 --fail --silent --show-error \
                --location --connect-timeout 15 --max-time "${timeout}" \
                "${curl_args[@]}" --output "${output}" \
                --dump-header "${headers}" \
                --write-out '%{http_code} %{url_effective}' "${url}"
        )" || status=$?
        http_code="${result%% *}"
        effective_url="${result#* }"

        if ((status == 0)); then
            if [[ -n "${write_out}" ]]; then
                printf '%s' "${effective_url}"
            elif [[ "${temporary_output}" == true ]]; then
                cat "${output}"
            fi
            rm -rf "${temporary_directory}"
            return 0
        fi

        if ! retry_curl_is_transient "${status}" "${http_code}"; then
            rm -rf "${temporary_directory}"
            return "${status}"
        fi

        wait_time="$(retry_curl_wait_seconds "${headers}" "${delay}")"
        remaining=$((deadline - SECONDS))
        if ((attempt >= max_attempts || remaining <= wait_time)); then
            echo "curl request exhausted its retry budget" >&2
            rm -rf "${temporary_directory}"
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

retry_curl_is_transient() {
    local status="$1"
    local http_code="$2"

    case "${status}:${http_code}" in
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

retry_curl_wait_seconds() {
    local headers="$1"
    local delay="$2"
    local retry_after
    local retry_time=0
    local wait_time="${delay}"
    local now

    retry_after="$(
        awk '
            /^HTTP\// { value = "" }
            tolower($1) == "retry-after:" {
                sub(/^[^:]*:[ \t]*/, "")
                sub(/[ \t\r]*$/, "")
                value = $0
            }
            END { print value }
        ' "${headers}"
    )"
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
