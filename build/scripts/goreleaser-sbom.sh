#!/bin/bash

set -euo pipefail

args=("$@")
if [[ "${GORELEASER_SNAPSHOT:-false}" == "true" ]]; then
    export SYFT_CHECK_FOR_APP_UPDATE=false
    export SYFT_GOLANG_SEARCH_REMOTE_LICENSES=false
else
    args+=(--enrich golang)
fi

exec syft "${args[@]}"
