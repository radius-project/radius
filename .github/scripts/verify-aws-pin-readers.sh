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

git show "${commit}:.github/scripts/stage-release-bicep.mjs" |
    grep -Fq 'plan = await tagPlan({ github, context });' ||
    fail ".github/scripts/stage-release-bicep.mjs does not use the receipt-resolved plan"

git show "${commit}:.github/scripts/verify-release-publication.sh" |
    grep -Fq 'RELEASE_VERIFY_PLAN_FILE' ||
    fail ".github/scripts/verify-release-publication.sh does not read the receipt-resolved plan"

workflow="$(git show "${commit}:.github/workflows/build-release.yaml" | yq -o=json '.')" ||
    fail "cannot read .github/workflows/build-release.yaml"

# Each job resolves the receipt in one unconditional aws-pin step. Every named
# "step=ENV" consumer runs after it and reads its plan, as must any other plan
# input in the job.
require_consumers() {
    local job="$1"
    shift
    jq -e --arg job "${job}" --arg plan '${{ steps.aws-pin.outputs.plan-file }}' --args '
        [.jobs[$job].steps // [] | to_entries[]] as $steps |
        [$steps[] | select(.value.id == "aws-pin")] as $readers |
        ($readers | length == 1) and
        ($readers[0].value |
            ((.with.script // "") | contains("aws-pin-receipt.mjs")) and
            .if == null and (."continue-on-error" // false) == false) and
        ([$steps[].value.env // {} | to_entries[] |
            select(.key | endswith("PLAN_FILE")) | .value] | all(. == $plan)) and
        ($ARGS.positional | all(
            capture("^(?<name>.+)=(?<key>[A-Z_]+)$") as $use |
            [$steps[] | select(.value.name == $use.name)] |
            length == 1 and .[0].key > $readers[0].key and
            (.[0].value.env // {})[$use.key] == $plan))
    ' "$@" <<<"${workflow}" > /dev/null ||
        fail ".github/workflows/build-release.yaml:${job} does not pass the receipt-resolved plan to $*"
}

require_consumers verify-release \
    "Verify RC prerequisites for finals=INPUT_PLAN_FILE" \
    "Verify required outputs and installation=RELEASE_VERIFY_PLAN_FILE"
require_consumers finalize-release \
    "Recheck outputs after approval=RELEASE_VERIFY_PLAN_FILE" \
    "Recheck RC prerequisites after approval=INPUT_PLAN_FILE" \
    "Promote locked Bicep extensions=INPUT_PLAN_FILE"
require_consumers publish-sibling-releases \
    "Reconcile sibling release entries=INPUT_PLAN_FILE"
