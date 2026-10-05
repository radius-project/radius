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

modules=()
while IFS= read -r directory || [[ -n "${directory}" ]]; do
    directory="${directory%$'\r'}"
    [[ -n "${directory}" ]] || continue
    if [[ ! -f "${directory}/go.mod" ]]; then
        echo "::error::No go.mod found in ${directory}"
        exit 1
    fi
    modules+=("${directory}")
done <<< "${INPUT_MODULE_DIRECTORIES}"

if [[ "${#modules[@]}" -eq 0 ]]; then
    echo "::error::module-directories must contain at least one directory"
    exit 1
fi

for directory in "${modules[@]}"; do
    (
        cd "${directory}"
        attempt=1
        wait_seconds="${INPUT_RETRY_WAIT_SECONDS}"
        until go mod download; do
            if [[ "${attempt}" -ge "${INPUT_RETRIES}" ]]; then
                echo "::error::go mod download in ${directory} failed after ${INPUT_RETRIES} attempts"
                exit 1
            fi
            echo "::warning::go mod download in ${directory} failed (attempt ${attempt}/${INPUT_RETRIES}); retrying in ${wait_seconds}s"
            sleep "${wait_seconds}"
            attempt=$((attempt + 1))
            wait_seconds=$((wait_seconds * 2))
        done
    )
done
