#!/usr/bin/env bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.

# Exercises the workflow's actual scripts with local Git repos and a fake gh.
# Requires Bash, Git, jq, and Mike Farah's yq; no GitHub access or Go build.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly WORKFLOW="${SCRIPT_DIR}/../workflows/update-resource-types.yaml"

for tool in git jq yq; do
    command -v "${tool}" > /dev/null || {
        echo "ERROR: ${tool} is required." >&2
        exit 1
    }
done

ROOT="$(mktemp -d "${TMPDIR:-/tmp}/update-resource-types-test.XXXXXX")"
readonly ROOT
trap 'rm -rf "${ROOT}"' EXIT
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null

assert_equal() {
    if [[ "$1" != "$2" ]]; then
        echo "FAIL: $3: expected '$1', got '$2'." >&2
        exit 1
    fi
}

assert_json() {
    assert_equal \
        "$(jq -cS 'sort_by(.name)' <<< "$1" | tr -d '\r\n')" \
        "$(jq -cS 'sort_by(.name)' <<< "$2" | tr -d '\r\n')" "$3"
}

steps="$(yq -o=json '.jobs."open-update-pr".steps' "${WORKFLOW}")"
for step in \
    "Prepare cumulative update branch" \
    "Update resource types and recipe packs" \
    "Generate derived artifacts"; do
    jq -er --arg name "${step}" \
        '.[] | select(.name == $name) | .run' <<< "${steps}" \
        | tr -d '\r' > "${ROOT}/${step}.sh"
done

export PR_BRANCH GITHUB_REPOSITORY GITHUB_REF GITHUB_SHA GH_TOKEN
PR_BRANCH="$(yq -r '.env.PR_BRANCH' "${WORKFLOW}")"
GITHUB_REPOSITORY=radius-project/radius
GITHUB_REF=refs/heads/main
GH_TOKEN=unused-test-token
export GH_CALLS="${ROOT}/gh-calls" GITHUB_OUTPUT="${ROOT}/output"
export PR_STATE=open
export PAYLOAD_PINS='[{"name":"Radius.Storage","ref":"Radius.Storage/v0.2.3"}]'
export PAYLOAD_PACK_PINS='[{"name":"azure-aks","ref":"recipe-pack/azure-aks/v0.1.0"}]'

gh() {
    printf '%s\n' "$*" >> "${GH_CALLS}"
    local query="repos/${GITHUB_REPOSITORY}/pulls?state=open"
    query="${query}&head=${GITHUB_REPOSITORY%%/*}:${PR_BRANCH}"
    if [[ "$*" != "api ${query} --jq .[].number" ]]; then
        echo "ERROR: unexpected gh invocation: $*" >&2
        return 1
    fi
    case "${PR_STATE}" in
        open) echo 123 ;;
        closed) ;;
        error)
            echo "ERROR: simulated GitHub API failure." >&2
            return 1
            ;;
        *) return 1 ;;
    esac
}
export -f gh

commit_fixture() {
    git -C "${ROOT}/origin" add .
    git -C "${ROOT}/origin" -c user.name="Workflow test" \
        -c user.email=workflow-test@example.invalid \
        -c commit.gpgsign=false commit --quiet -m "$1"
}

write_tools() {
    printf 'module example.invalid/snapshot\n\ngo %s\n' "$1" \
        > "${ROOT}/origin/go.mod"
    printf 'module-checksum-%s\n' "$1" > "${ROOT}/origin/go.sum"
    printf '%s\n' "$2" > "${ROOT}/origin/.node-version"
    printf 'version: %s\n' "$3" > "${ROOT}/origin/build/tools.yaml"
}

git init --quiet --initial-branch=main "${ROOT}/origin"
mkdir -p "${ROOT}/origin/deploy/manifest" "${ROOT}/origin/build"
cat > "${ROOT}/origin/deploy/manifest/defaults.yaml" << 'YAML'
resourceTypes:
  - name: Radius.Data
    ref: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  - name: Radius.Messaging
    ref: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
  - name: Radius.Storage
    ref: cccccccccccccccccccccccccccccccccccccccc
    tag: Radius.Storage/v0.2.1
recipePacks:
  - name: kubernetes
    ref: dddddddddddddddddddddddddddddddddddddddd
    tag: recipe-pack/kubernetes/v0.3.0
  - name: azure-aks
    ref: eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
  - name: azure-aci
    ref: ffffffffffffffffffffffffffffffffffffffff
    tag: recipe-pack/azure-aci/v0.1.0
YAML
write_tools 1.27.2 24.1.0 v0.45.0
commit_fixture "Queued dispatch"
GITHUB_SHA="$(git -C "${ROOT}/origin" rev-parse HEAD)"

write_tools 1.27.3 24.2.0 v0.46.0
commit_fixture "Main advances before checkout"
main_sha="$(git -C "${ROOT}/origin" rev-parse HEAD)"

git -C "${ROOT}/origin" checkout --quiet -b "${PR_BRANCH}" "${GITHUB_SHA}"
yq -i '
    .resourceTypes[0].tag = "Radius.Data/v0.3.1" |
    .resourceTypes[1].ref = "1111111111111111111111111111111111111111" |
    .resourceTypes[2].tag = "Radius.Storage/v0.2.2" |
    .recipePacks[0].ref = "2222222222222222222222222222222222222222" |
    .recipePacks[0].tag = "recipe-pack/kubernetes/v0.3.1"
' "${ROOT}/origin/deploy/manifest/defaults.yaml"
commit_fixture "Pending namespace and recipe pack pins"
git -C "${ROOT}/origin" checkout --quiet main

# Model checkout's explicit ref, or its event-SHA default, without running
# the hosted action. A second main advance catches an initial-ref-only fix.
checkout_ref="$(jq -r '
    .[] | select(.uses // "" | startswith("actions/checkout@")) |
    .with.ref // ""
' <<< "${steps}" | tr -d '\r')"
git clone --quiet --no-local "${ROOT}/origin" "${ROOT}/checkout"
cd "${ROOT}/checkout"
if [[ -n "${checkout_ref}" ]]; then
    git checkout --quiet "${checkout_ref}"
else
    git checkout --quiet --detach "${GITHUB_SHA}"
fi
export SETUP_SHA
SETUP_SHA="$(git rev-parse HEAD)"
export SETUP_FILES
SETUP_FILES="$(git hash-object go.mod go.sum .node-version build/tools.yaml)"

write_tools 1.27.4 24.3.0 v0.47.0
yq -i '
    .recipePacks[0].ref = "2222222222222222222222222222222222222222" |
    .recipePacks[0].tag = "recipe-pack/kubernetes/v0.3.1"
' "${ROOT}/origin/deploy/manifest/defaults.yaml"
commit_fixture "Main advances again after tool setup"

assert_snapshot() {
    assert_equal "${SETUP_SHA}" "$(git rev-parse HEAD)" \
        "HEAD must stay at the tool-setup snapshot"
    assert_equal "${SETUP_FILES}" \
        "$(git hash-object go.mod go.sum .node-version build/tools.yaml)" \
        "Go, module cache, Node, and tool pins must match setup"
    assert_equal "" "$(git status --porcelain)" \
        "branch preparation must leave the checked-out source unchanged"
}

prepare() {
    : > "${GITHUB_OUTPUT}"
    : > "${GH_CALLS}"
    bash "${ROOT}/Prepare cumulative update branch.sh"
    assert_snapshot
}

output_pins() {
    sed -n "/^$1<</{n;p;}" "${GITHUB_OUTPUT}" | tr -d '\r'
}

export expected_pins='[
    {"name":"Radius.Data","ref":"Radius.Data/v0.3.1"},
    {"name":"Radius.Messaging","ref":"1111111111111111111111111111111111111111"},
    {"name":"Radius.Storage","ref":"Radius.Storage/v0.2.3"}
]'
export expected_packs='[
    {"name":"kubernetes","ref":"recipe-pack/kubernetes/v0.3.1"},
    {"name":"azure-aks","ref":"recipe-pack/azure-aks/v0.1.0"}
]'

prepare
assert_equal "${main_sha}" "${SETUP_SHA}" \
    "checkout must select current main, not the queued event SHA"
assert_equal main "$(git branch --show-current)" \
    "create-pull-request needs the selected main branch"
assert_json "${expected_pins}" "$(output_pins pins)" \
    "pending tags and bare SHAs survive, with payload pins taking precedence"
assert_json "${expected_packs}" "$(output_pins pack_pins)" \
    "pending packs are compared to the setup snapshot, not newer main"

# Execute the update/generate step bodies, checking their inputs at the make
# boundary instead of downloading contrib content or running code generation.
make() {
    assert_snapshot
    case "$*" in
        update-resource-types-and-recipe-packs)
            assert_json "${expected_pins}" "${RESOURCE_TYPES_PINS}" \
                "resource type inputs to make"
            assert_json "${expected_packs}" "${RECIPE_PACKS_PINS}" \
                "recipe pack inputs to make"
            ;;
        generate) ;;
        *)
            echo "ERROR: unexpected make invocation: $*" >&2
            return 1
            ;;
    esac
    printf '%s\n' "$*" >> "${MAKE_CALLS}"
}
export -f make assert_snapshot assert_equal assert_json
export MAKE_CALLS="${ROOT}/make-calls"
export RESOURCE_TYPES_PINS RECIPE_PACKS_PINS
RESOURCE_TYPES_PINS="$(output_pins pins)"
RECIPE_PACKS_PINS="$(output_pins pack_pins)"
bash "${ROOT}/Update resource types and recipe packs.sh"
bash "${ROOT}/Generate derived artifacts.sh"
assert_equal $'update-resource-types-and-recipe-packs\ngenerate' \
    "$(cat "${MAKE_CALLS}")" "both generation steps use the setup snapshot"
echo "PASS: queued dispatch and moving main preserve one source/tool snapshot."

PR_STATE=closed
prepare
assert_json "${PAYLOAD_PINS}" "$(output_pins pins)" \
    "a closed PR must not carry stale namespace pins"
assert_json "${PAYLOAD_PACK_PINS}" "$(output_pins pack_pins)" \
    "a closed PR must not carry stale recipe pack pins"
echo "PASS: a leftover bot branch without an open PR contributes no pins."

PR_STATE=error
: > "${GITHUB_OUTPUT}"
if bash "${ROOT}/Prepare cumulative update branch.sh" \
    > "${ROOT}/api-error.log" 2>&1; then
    echo "FAIL: GitHub API failure was ignored." >&2
    exit 1
fi
grep -q "ERROR: failed to query the pending" "${ROOT}/api-error.log"
assert_equal "" "$(cat "${GITHUB_OUTPUT}")" "API failure emits no pins"
assert_snapshot
echo "PASS: an open-PR lookup failure stops preparation."

PR_STATE=open
git -C "${ROOT}/origin" checkout --quiet "${PR_BRANCH}"
yq -o=json '.' "${ROOT}/origin/deploy/manifest/defaults.yaml" \
    | jq '
        .sources = (.resourceTypes | map(.namespace = .name | del(.name))) |
        del(.resourceTypes)
    ' > "${ROOT}/legacy.json"
cp "${ROOT}/legacy.json" "${ROOT}/origin/deploy/manifest/defaults.yaml"
commit_fixture "Legacy pending namespace schema"
git -C "${ROOT}/origin" checkout --quiet main
prepare
assert_json "${expected_pins}" "$(output_pins pins)" \
    "legacy sources and namespace fields remain supported"
assert_json "${expected_packs}" "$(output_pins pack_pins)" \
    "legacy namespace schema does not discard recipe pack pins"
echo "PASS: pending pins remain readable across the namespace schema rename."

git -C "${ROOT}/origin" branch --delete --force "${PR_BRANCH}" > /dev/null
prepare
assert_equal "" "$(cat "${GH_CALLS}")" "no bot branch means no PR lookup"
assert_json "${PAYLOAD_PINS}" "$(output_pins pins)" \
    "a missing bot branch contributes no namespace pins"
assert_json "${PAYLOAD_PACK_PINS}" "$(output_pins pack_pins)" \
    "a missing bot branch contributes no recipe pack pins"
echo "PASS: a missing bot branch uses only the dispatch payload."

jq -e '
    map(.name) as $names |
    ($names | index("Checkout")) as $checkout |
    ($names | index("Prepare cumulative update branch")) as $prepare |
    [ .[] | select(.uses // "" | startswith("actions/checkout@")) ] as $clones |
    ($clones | length) == 1 and $clones[0].with.ref == "main" and
    $checkout != null and $prepare != null and
    all("Setup Go", "Download Go modules", "Setup NodeJS",
        "Install yq", "Install Bicep";
        . as $name | ($names | index($name)) as $index |
        $index != null and $checkout < $index and $index < $prepare) and
    $prepare < ($names | index("Update resource types and recipe packs")) and
    ($names | index("Update resource types and recipe packs")) <
        ($names | index("Generate derived artifacts")) and
    ($names | index("Generate derived artifacts")) <
        ($names | index("Create or update pull request"))
' <<< "${steps}" > /dev/null
assert_equal max "$(yq -r '.concurrency.queue' "${WORKFLOW}")" \
    "dispatches must remain queued rather than evicted"
assert_equal update-resource-types \
    "$(yq -r '.concurrency.group' "${WORKFLOW}")" \
    "all dispatches must share one concurrency group"
echo "PASS: one main checkout precedes all revision-dependent setup."
