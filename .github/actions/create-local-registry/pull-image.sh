#!/usr/bin/env bash

set -euo pipefail

if [[ ! "${INPUT_RETRIES}" =~ ^[1-9][0-9]*$ ]]; then
    echo "::error::retries must be a positive integer"
    exit 1
fi
if [[ ! "${INPUT_RETRY_WAIT_SECONDS}" =~ ^(0|[1-9][0-9]*)$ ]]; then
    echo "::error::retry-wait-seconds must be a nonnegative integer"
    exit 1
fi
if [[ ! "${INPUT_PULL_TIMEOUT_SECONDS}" =~ ^[1-9][0-9]*$ ]]; then
    echo "::error::pull-timeout-seconds must be a positive integer"
    exit 1
fi

if docker image inspect "${INPUT_IMAGE}" > /dev/null 2>&1; then
    echo "Using cached image ${INPUT_IMAGE}"
    exit 0
fi

output="$(mktemp)"
trap 'rm -f "${output}"' EXIT

attempt=1
wait_seconds="${INPUT_RETRY_WAIT_SECONDS}"
while true; do
    if timeout --kill-after=5 "${INPUT_PULL_TIMEOUT_SECONDS}" \
        docker pull "${INPUT_IMAGE}" > "${output}" 2>&1; then
        cat "${output}"
        exit 0
    else
        status=$?
    fi
    cat "${output}"

    # Docker uses the same exit code for transport and permanent errors.
    if [[ "${status}" -eq 127 || "${status}" -eq 130 ||
        "${status}" -eq 137 || "${status}" -eq 143 ]] ||
        grep -Eiq 'unauthorized|authentication required|pull access denied|(^|[[:space:]])denied:|manifest unknown|name unknown|invalid reference format|invalid argument|no matching manifest|x509:|checksum mismatch|digest mismatch|filesystem layer verification failed' "${output}"; then
        echo "::error::docker pull ${INPUT_IMAGE} failed without retry (exit ${status})"
        exit "${status}"
    fi
    if [[ "${attempt}" -ge "${INPUT_RETRIES}" ]]; then
        echo "::error::docker pull ${INPUT_IMAGE} failed after ${INPUT_RETRIES} attempts"
        exit "${status}"
    fi
    delay=$((wait_seconds + RANDOM % (wait_seconds + 1)))
    echo "::warning::docker pull ${INPUT_IMAGE} failed (exit ${status}, attempt ${attempt}/${INPUT_RETRIES}); retrying in ${delay}s"
    sleep "${delay}"
    attempt=$((attempt + 1))
    wait_seconds=$((wait_seconds * 2))
done
