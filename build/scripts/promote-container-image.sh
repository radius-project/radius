#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <source-image> <target-image>" >&2
    exit 2
fi

readonly SOURCE_IMAGE="$1"
readonly TARGET_IMAGE="$2"

inspect_output=""
if inspect_output="$(docker buildx imagetools inspect "${SOURCE_IMAGE}" 2>&1)"; then
    echo "Promoting ${SOURCE_IMAGE} to ${TARGET_IMAGE}"
    docker buildx imagetools create \
        --tag "${TARGET_IMAGE}" \
        "${SOURCE_IMAGE}"
    exit 0
fi

if grep -Eqi 'manifest unknown|(^|: )not found($|[[:space:]])' \
    <<< "${inspect_output}"; then
    echo "Source image ${SOURCE_IMAGE} is not published; leaving ${TARGET_IMAGE} unchanged"
    exit 0
fi

echo "${inspect_output}" >&2
exit 1
