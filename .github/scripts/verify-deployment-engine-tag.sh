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
TAG="${1:-}"

fail() {
    echo "Error: $*" >&2
    exit 1
}

main() {
    local reference object_type object_sha verification recovery

    if [[ ! "${TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[1-9][0-9]*)?$ ]]; then
        fail "tag must use vX.Y.Z or vX.Y.Z-rc.N format"
    fi
    command -v "${GH}" > /dev/null || fail "required command not found: ${GH}"
    command -v jq > /dev/null || fail "required command not found: jq"

    recovery="git tag -s ${TAG} -m 'release tag ${TAG}'"
    recovery+=" && git push origin ${TAG}"
    if ! reference="$(
        "${GH}" api "repos/${REPOSITORY}/git/ref/tags/${TAG}"
    )"; then
        fail "Create the signed tag with: ${recovery}"
    fi
    object_type="$(jq -r '.object.type' <<< "${reference}")"
    object_sha="$(jq -r '.object.sha' <<< "${reference}")"
    if [[ "${object_type}" != "tag" ]]; then
        fail "Deployment Engine tag ${TAG} is lightweight, not signed"
    fi

    verification="$(
        "${GH}" api "repos/${REPOSITORY}/git/tags/${object_sha}" \
            --jq '.verification.verified'
    )"
    if [[ "${verification}" != "true" ]]; then
        fail "Deployment Engine tag ${TAG} has no valid signature"
    fi

    echo "Verified signed Deployment Engine tag ${TAG}."
}

main "$@"
