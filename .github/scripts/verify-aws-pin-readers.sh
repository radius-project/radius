#!/bin/bash
# Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
set -euo pipefail

# Tags execute the selected product's workflows, not the controller's copy.
commit="$(git rev-parse --verify "${1:?expected product commit}^{commit}")"
fail() {
    echo "Error: ${commit} cannot publish from an AWS pin receipt: $*. Backport the receipt readers before preparing this channel." >&2
    exit 1
}

git cat-file -e "${commit}:.github/scripts/aws-pin-receipt.mjs" ||
    fail "missing the signed receipt reader"

require_reader() {
    local file=".github/workflows/$1" job="$2"
    git show "${commit}:${file}" | yq -o=json '.' | jq -e --arg job "${job}" '
        [.jobs[$job].steps[] | select(
            ((.with.script // "") | contains("aws-pin-receipt.mjs")) and
            (.if == null) and (."continue-on-error" // false) == false
        )] | length == 1' > /dev/null ||
        fail "${file}:${job} does not require the AWS receipt"
}

require_reader __build-bicep-types.yaml build-and-push-aws-bicep-types
require_reader build-release.yaml verify-release
require_reader build-release.yaml finalize-release
require_reader build-release.yaml publish-sibling-releases
