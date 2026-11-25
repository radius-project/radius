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

require_reader build-release.yaml verify-release
require_reader build-release.yaml finalize-release
require_reader build-release.yaml publish-sibling-releases

git show "${commit}:.github/scripts/stage-release-bicep.mjs" |
    grep -Fq 'plan = await tagPlan({ github, context });' ||
    fail ".github/scripts/stage-release-bicep.mjs does not use the receipt-resolved plan"

git show "${commit}:.github/workflows/build-release.yaml" |
    yq -o=json '.' |
    jq -e '
        any(.jobs["verify-release"].steps[]?;
            .env.INPUT_PLAN_FILE == "${{ steps.aws-pin.outputs.plan-file }}") and
        any(.jobs["verify-release"].steps[]?;
            .env.RELEASE_VERIFY_PLAN_FILE == "${{ steps.aws-pin.outputs.plan-file }}") and
        any(.jobs["finalize-release"].steps[]?;
            .env.INPUT_PLAN_FILE == "${{ steps.aws-pin.outputs.plan-file }}") and
        any(.jobs["publish-sibling-releases"].steps[]?;
            .env.INPUT_PLAN_FILE == "${{ steps.aws-pin.outputs.plan-file }}")
    ' > /dev/null ||
    fail ".github/workflows/build-release.yaml does not pass the receipt-resolved plan to consumers"
