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
readonly REPOSITORY="azure-octo/deployment-engine"
TAG=""
# Optional commit recorded by an earlier verification of the same release run.
EXPECTED_COMMIT=""
# Ref whose commit the release controller tags after approval when the tag
# is absent.
ABSENT_BASE=""

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: verify-deployment-engine-tag.sh [--allow-absent <base-ref>] <tag> \
    [expected-commit]

Writes the tag commit and its state (signed, unsigned, or absent) to
GITHUB_OUTPUT. A signed tag needs a signature that GitHub verifies. An
unsigned tag, such as one the release controller created, must reference a
commit on main. With --allow-absent, a missing tag reports the commit of
<base-ref> (main or a release tag) that the controller will tag after
approval instead of failing.
EOF
}

# An unsigned tag has no maintainer attestation, so it may only name a commit
# that is already on the protected main branch.
require_on_main() {
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
}

write_outputs() {
    local commit="$1"
    local state="$2"

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
        fail "Deployment Engine tag ${TAG} does not exist. The release controller creates it after approval; to release another commit, create the signed tag with: ${recovery}"
    fi
    base="$("${GH}" api "repos/${REPOSITORY}/commits/${ABSENT_BASE}")" ||
        fail "could not resolve Deployment Engine ${ABSENT_BASE}"
    base_commit="$(jq -r '.sha' <<<"${base}")"
    [[ "${base_commit}" =~ ^[0-9a-f]{40}$ ]] ||
        fail "Deployment Engine ${ABSENT_BASE} does not resolve to a commit"
    require_on_main "${base_commit}"
    if [[ -n "${EXPECTED_COMMIT}" && "${base_commit}" != "${EXPECTED_COMMIT}" ]]; then
        fail "Deployment Engine ${ABSENT_BASE} moved from approved commit ${EXPECTED_COMMIT} to ${base_commit}"
    fi

    write_outputs "${base_commit}" absent
    echo "Deployment Engine tag ${TAG} is absent; the release controller will create it at ${base_commit} (${ABSENT_BASE}) after approval."
}

main() {
    local reference object_type object_sha tag_object verified reason
    local signed_tag_name target_type target_sha recovery state
    local tag_pattern base_pattern

    while [[ $# -gt 0 ]]; do
        case "$1" in
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

    # The repository is private, so a token without access receives the same
    # 404 as a missing tag. Prove access first instead of asking for a tag
    # that may already exist.
    if ! "${GH}" api "repos/${REPOSITORY}" --jq '.full_name' > /dev/null; then
        fail "the token cannot read ${REPOSITORY}; verify ${TAG} with a publisher App token minted for that repository with contents: read"
    fi

    recovery="git tag -s ${TAG} -m 'release tag ${TAG}'"
    recovery+=" && git push origin ${TAG}"
    if ! reference="$(
        "${GH}" api "repos/${REPOSITORY}/git/ref/tags/${TAG}"
    )"; then
        plan_absent_tag "${recovery}"
        return
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
        require_on_main "${target_sha}"
    fi
    if [[ -n "${EXPECTED_COMMIT}" && "${target_sha}" != "${EXPECTED_COMMIT}" ]]; then
        fail "Deployment Engine tag ${TAG} moved from approved commit ${EXPECTED_COMMIT} to ${target_sha}"
    fi

    write_outputs "${target_sha}" "${state}"
    echo "Verified ${state} Deployment Engine tag ${TAG} at ${target_sha}."
}

main "$@"