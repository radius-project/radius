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

attempt=1
wait_seconds="${INPUT_RETRY_WAIT_SECONDS}"
until docker pull "${INPUT_IMAGE}"; do
    if [[ "${attempt}" -ge "${INPUT_RETRIES}" ]]; then
        echo "::error::docker pull ${INPUT_IMAGE} failed after ${INPUT_RETRIES} attempts"
        exit 1
    fi
    echo "::warning::docker pull ${INPUT_IMAGE} failed (attempt ${attempt}/${INPUT_RETRIES}); retrying in ${wait_seconds}s"
    sleep "${wait_seconds}"
    attempt=$((attempt + 1))
    wait_seconds=$((wait_seconds * 2))
done
