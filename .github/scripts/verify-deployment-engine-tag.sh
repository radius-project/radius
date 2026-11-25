#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

set -euo pipefail

GH="${GH:-gh}"
readonly REPOSITORY="radius-project/deployment-engine"
TAG=""
# Optional commit that the tag, or an absent tag's base ref, must resolve to:
# the approved commit, or for a final release the commit its RC locked.
EXPECTED_COMMIT=""
# Ref whose commit the release controller tags after approval when the tag
# is absent.
ABSENT_BASE=""
REQUIRE_PUBLISHER=false
LOOKUP_ERROR=""

cleanup() {
    if [[ -n "${LOOKUP_ERROR}" ]]; then
        rm -f "${LOOKUP_ERROR}"
    fi
}
trap cleanup EXIT

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: verify-deployment-engine-tag.sh [--require-publisher] [--allow-absent <base-ref>] <tag> \
    [expected-commit]

Writes the tag commit and its state (signed, unsigned, or absent) to
GITHUB_OUTPUT. A signed tag needs a signature that GitHub verifies. An
unsigned tag, such as one the release controller created, must reference a
GitHub-verified commit on main. With --allow-absent, a tag that GitHub
reports missing (HTTP 404) reports the commit of <base-ref> (main or a
release tag) that the controller will tag after approval instead of failing.
With expected-commit, the tag, or <base-ref> for an absent tag, must resolve
to that commit.
EOF
}

# An unsigned tag has no maintainer attestation, so it may only name a
# GitHub-verified commit that is already on the protected main branch.
require_trusted_main_commit() {
    local commit="$1"
    local comparison status

    comparison="$(
        "${GH}" api "repos/${REPOSITORY}/compare/${commit}...main?per_page=1"
    )" || fail "could not compare ${commit} with Deployment Engine main"
    status="$(jq -r '.status' <<<"${comparison}")"
    case "${status}" in
        ahead | identical) ;;
        *)
            fail "Deployment Engine commit ${commit} is not on main (${status}); push a signed tag to release another commit"
            ;;
    esac
    if [[ "$(jq -r '.base_commit.commit.verification.verified' \
        <<<"${comparison}")" != "true" ]]; then
        fail "Deployment Engine commit ${commit} does not have a GitHub-verified signature; push a signed tag to release it"
    fi
}

read_publisher_file() {
    local commit="$1" path="$2" media_type="$3"
    if ! "${GH}" api "repos/${REPOSITORY}/contents/${path}?ref=${commit}" \
        -H "Accept: ${media_type}" 2>"${LOOKUP_ERROR}"; then
        cat "${LOOKUP_ERROR}" >&2
        if grep -Fq '(HTTP 404)' "${LOOKUP_ERROR}"; then
            fail "${path} is missing at ${commit}; select a reviewed release source before tagging"
        fi
        fail "could not read ${path} at ${commit}; check Contents access and retry the same commit once the API error clears"
    fi
}

write_outputs() {
    local commit="$1"
    local state="$2"

    if [[ "${REQUIRE_PUBLISHER}" == true ]]; then
        local workflow publish_prefix helper=".github/scripts/require-unused-tag.sh" metadata
        # Match executable control flow through the push, not a command that
        # could be unreachable. The producer may report its digest afterward.
        publish_prefix="$(cat <<'BASH'
if [[ "$GITHUB_REF" == refs/tags/* ]]; then
  bash .github/scripts/require-unused-tag.sh "$TAG"
fi
export BUILDX_METADATA_FILE="$RUNNER_TEMP/build-metadata.json"
make docker-push REPO="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY" COMMIT_SHA="$GITHUB_SHA" COMMIT_REF="$GITHUB_REF"
BASH
)"
        workflow="$(read_publisher_file "${commit}" .github/workflows/build-and-push.yaml application/vnd.github.raw+json)" || exit 1
        yq -o=json '.' <<<"${workflow}" |
            jq -e \
                --arg build "github.repository == '${REPOSITORY}' && !github.event.deleted" \
                --arg container "github.repository == '${REPOSITORY}' && (github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/v'))" \
                --arg parse '.github/scripts/parse-version.sh --ref "$GITHUB_REF" --sha "$GITHUB_SHA" --out "$GITHUB_OUTPUT"' \
                --arg push "$publish_prefix" '
                def mandatory: .if == null and (."continue-on-error" // false) == false;
                def can_read: if type == "object" then .contents == "read" or .contents == "write"
                              else . == "read-all" or . == "write-all" end;
                def checks_out_source($defaults):
                    ((.permissions // $defaults) | can_read) and
                    (.steps | to_entries |
                     map(select(.value.uses // "" | startswith("actions/checkout@"))) as $c |
                     length > 0 and ($c | length) == 1 and
                     ($c[0].value | mandatory and
                      (.uses | test("^actions/checkout@[0-9a-f]{40}$")) and
                      ((.with // {} | keys) - ["fetch-depth","persist-credentials","ref"] == []) and
                      (.with.ref == null or .with.ref == "${{ github.sha }}")) and
                     all(.[] | select(.value.run != null or (.value.uses // "" | startswith("./")));
                         .key > $c[0].key));
                .permissions as $defaults |
                .jobs.container as $job |
                .on.push.tags == ["v*"] and
                .env.REGISTRY == "ghcr.io/radius-project" and
                .jobs.build.needs == null and .jobs.build.if == $build and
                (.jobs.build | checks_out_source($defaults)) and ($job | checks_out_source($defaults)) and
                $job.needs == "build" and $job.if == $container and
                (($job.permissions // $defaults) |
                    if type == "object" then .packages == "write" else . == "write-all" end) and
                $job.env.REGISTRY == null and
                ([$job.steps | to_entries[] | select(.value.id == "version")] as $v |
                 [$job.steps | to_entries[] | select(.value.name == "Publish image")] as $p |
                 ($v | length) == 1 and ($p | length) == 1 and $v[0].key < $p[0].key and
                 any($job.steps | to_entries[];
                     .key < $p[0].key and (.value | mandatory and
                         (.uses // "" | test("^docker/login-action@[0-9a-f]{40}$")) and
                         .with.registry == "ghcr.io" and .with.username == "${{ github.actor }}" and
                         .with.password == "${{ github.token }}")) and
                 ($v[0].value | mandatory and (.run | rtrimstr("\n")) == $parse) and
                 ($p[0].value | mandatory and .env.TAG == "${{ steps.version.outputs.ghcr-tag }}" and
                    .env.REGISTRY == null and (.run == $push or (.run | startswith($push + "\n")))))
            ' >/dev/null ||
            fail "${commit} lacks the active direct GHCR tag publisher; select a reviewed release source before tagging"
        metadata="$(read_publisher_file "${commit}" "${helper}" application/vnd.github+json)" || exit 1
        jq -e '.type == "file" and (.size | type == "number" and . > 0)' \
            <<<"${metadata}" >/dev/null ||
            fail "${helper} is not a nonempty file at ${commit}; select a reviewed release source before tagging"
        local tree parser=".github/scripts/parse-version.sh"
        tree="$("${GH}" api "repos/${REPOSITORY}/git/trees/${commit}?recursive=1")" ||
            fail "could not read the Git tree at ${commit}; retry the same commit after checking API access"
        jq -e '.truncated == false and (.tree | type == "array")' \
            <<<"${tree}" >/dev/null || fail "incomplete Git tree at ${commit}; cannot verify ${parser}"
        jq -e --arg path "${parser}" \
            'any(.tree[]; .path == $path and .type == "blob" and .mode == "100755")' \
            <<<"${tree}" >/dev/null ||
            fail "${parser} is missing or not executable at ${commit}; select a reviewed release source before tagging"
    fi

    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        {
            echo "commit=${commit}"
            echo "state=${state}"
        } >>"${GITHUB_OUTPUT}"
    fi
}

plan_absent_tag() {
    local recovery="$1"
    local base base_commit

    if [[ -z "${ABSENT_BASE}" ]]; then
        if [[ -n "${EXPECTED_COMMIT}" ]]; then
            fail "Missing ${REPOSITORY} ref refs/tags/${TAG}; expected commit ${EXPECTED_COMMIT}. Stop recovery and investigate the missing ref."
        fi
        fail "Deployment Engine tag ${TAG} does not exist. The release controller creates it after approval; to release another commit, create the signed tag with: ${recovery}"
    fi
    base="$("${GH}" api "repos/${REPOSITORY}/commits/${ABSENT_BASE}")" ||
        fail "could not resolve Deployment Engine ${ABSENT_BASE}"
    base_commit="$(jq -r '.sha' <<<"${base}")"
    [[ "${base_commit}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "Deployment Engine ${ABSENT_BASE} does not resolve to a commit"
    if [[ -n "${EXPECTED_COMMIT}" && "${base_commit}" != "${EXPECTED_COMMIT}" ]]; then
        fail "Deployment Engine ${ABSENT_BASE} resolves to ${base_commit}, not the expected ${EXPECTED_COMMIT}; restore ${ABSENT_BASE} or push a signed ${TAG} tag at ${EXPECTED_COMMIT}"
    fi
    require_trusted_main_commit "${base_commit}"

    write_outputs "${base_commit}" absent
    echo "Deployment Engine tag ${TAG} is absent; the release controller will create it at ${base_commit} (${ABSENT_BASE}) after approval."
}

main() {
    local reference object_type object_sha tag_object verified reason
    local signed_tag_name target_type target_sha recovery state
    local tag_pattern base_pattern

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --require-publisher)
                REQUIRE_PUBLISHER=true
                shift
                ;;
            --allow-absent)
                ABSENT_BASE="${2:-}"
                [[ -n "${ABSENT_BASE}" ]] ||
                    fail "--allow-absent requires a base ref"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            -*) fail "unknown option: $1" ;;
            *) break ;;
        esac
    done
    (($# >= 1 && $# <= 2)) || {
        usage >&2
        exit 1
    }
    TAG="$1"
    EXPECTED_COMMIT="${2:-}"

    # Numeric identifiers reject leading zeros, matching the semver policy in
    # .github/scripts/release-version.sh.
    tag_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
    tag_pattern+='(-rc\.[1-9][0-9]*)?$'
    if [[ ! "${TAG}" =~ ${tag_pattern} ]]; then
        fail "tag must use vX.Y.Z or vX.Y.Z-rc.N format"
    fi
    if [[ -n "${EXPECTED_COMMIT}" &&
        ! "${EXPECTED_COMMIT}" =~ ^[0-9a-f]{40}$ ]]; then
        fail "expected commit must be a full commit SHA"
    fi
    # A final release tags its last RC's commit, which may use the historical
    # -rcN form.
    base_pattern='^(main|v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
    base_pattern+='(-rc\.?[1-9][0-9]*)?)$'
    if [[ -n "${ABSENT_BASE}" && ! "${ABSENT_BASE}" =~ ${base_pattern} ]]; then
        fail "base ref must be main or a release tag"
    fi
    command -v "${GH}" >/dev/null || fail "required command not found: ${GH}"
    command -v jq >/dev/null || fail "required command not found: jq"

    if [[ "$("${GH}" api "repos/${REPOSITORY}" --jq '.full_name')" != "${REPOSITORY}" ]]; then
        fail "cannot read ${REPOSITORY} at its canonical location"
    fi

    recovery="git tag -s ${TAG} -m 'release tag ${TAG}'"
    recovery+=" && git push origin ${TAG}"
    # Only GitHub's 404 for a readable repository proves the tag is missing.
    # Any other failure, such as 403, 429, 5xx, or a transport error, must not
    # become a plan to create the tag.
    LOOKUP_ERROR="$(mktemp)"
    if ! reference="$(
        "${GH}" api "repos/${REPOSITORY}/git/ref/tags/${TAG}" \
            2>"${LOOKUP_ERROR}"
    )"; then
        if grep -Fq '(HTTP 404)' "${LOOKUP_ERROR}"; then
            plan_absent_tag "${recovery}"
            return
        fi
        cat "${LOOKUP_ERROR}" >&2
        fail "could not look up Deployment Engine tag ${TAG}; rerun once the GitHub API error above clears"
    fi
    object_type="$(jq -r '.object.type' <<<"${reference}")"
    object_sha="$(jq -r '.object.sha' <<<"${reference}")"
    case "${object_type}" in
        tag)
            tag_object="$("${GH}" api "repos/${REPOSITORY}/git/tags/${object_sha}")"
            verified="$(jq -r '.verification.verified' <<<"${tag_object}")"
            reason="$(jq -r '.verification.reason // ""' <<<"${tag_object}")"
            signed_tag_name="$(jq -r '.tag' <<<"${tag_object}")"
            if [[ "${signed_tag_name}" != "${TAG}" ]]; then
                fail "Deployment Engine tag object names ${signed_tag_name}, expected ${TAG}"
            fi
            target_type="$(jq -r '.object.type' <<<"${tag_object}")"
            target_sha="$(jq -r '.object.sha' <<<"${tag_object}")"
            if [[ "${verified}" == "true" ]]; then
                state=signed
            elif [[ "${reason}" == "unsigned" ]]; then
                state=unsigned
            else
                # A signature that GitHub cannot verify is a failed signing
                # attempt, not an unsigned tag.
                fail "Deployment Engine tag ${TAG} has an unverified signature (${reason:-unknown})"
            fi
            ;;
        commit)
            target_type=commit
            target_sha="${object_sha}"
            state=unsigned
            ;;
        *)
            target_type="${object_type}"
            target_sha="${object_sha}"
            state=unsigned
            ;;
    esac
    if [[ "${target_type}" != "commit" ||
        ! "${target_sha}" =~ ^[0-9a-f]{40}$ ]]; then
        fail "Deployment Engine tag ${TAG} does not target a commit"
    fi
    if [[ "${state}" == "unsigned" ]]; then
        require_trusted_main_commit "${target_sha}"
    fi
    if [[ -n "${EXPECTED_COMMIT}" && "${target_sha}" != "${EXPECTED_COMMIT}" ]]; then
        fail "Deployment Engine tag ${TAG} is at ${target_sha}, not the expected ${EXPECTED_COMMIT}"
    fi

    write_outputs "${target_sha}" "${state}"
    echo "Verified ${state} Deployment Engine tag ${TAG} at ${target_sha}."
}

main "$@"