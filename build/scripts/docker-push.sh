#!/usr/bin/env bash

# Retry only the registry's "unknown blob" failure without rebuilding the image.
set -euo pipefail

if [[ "$#" -ne 1 || -z "$1" ]]; then
    echo "Usage: docker-push.sh IMAGE" >&2
    exit 2
fi

readonly IMAGE="$1"
readonly MAX_ATTEMPTS=3
output="$(mktemp)"
trap 'rm -f "${output}"' EXIT

attempt=1
wait_seconds=5
while true; do
    if docker push "${IMAGE}" 2>&1 | tee "${output}"; then
        exit 0
    else
        statuses=("${PIPESTATUS[@]}")
    fi
    if [[ "${statuses[1]}" -ne 0 ]]; then
        echo "ERROR: Could not record docker push output" >&2
        exit "${statuses[1]}"
    fi
    status="${statuses[0]}"
    error="$(tail -n 1 "${output}")"
    error="${error%$'\r'}"

    # Docker uses exit 1 for both registry and permanent configuration errors.
    if [[ "${status}" -ne 1 || "${error}" != "unknown blob" ||
        "${attempt}" -ge "${MAX_ATTEMPTS}" ]]; then
        echo "ERROR: docker push ${IMAGE} failed" \
            "(exit ${status}, attempt ${attempt}/${MAX_ATTEMPTS})" >&2
        exit "${status}"
    fi

    echo "WARNING: docker push ${IMAGE} returned unknown blob" \
        "(attempt ${attempt}/${MAX_ATTEMPTS}); retrying in ${wait_seconds}s" >&2
    sleep "${wait_seconds}"
    attempt=$((attempt + 1))
    wait_seconds=$((wait_seconds * 2))
done
