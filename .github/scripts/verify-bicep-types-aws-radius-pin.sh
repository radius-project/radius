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

# bicep-types-aws generates Bicep types from the Radius schemas at the commit
# its radius submodule pins, so its release must pin the commit this release
# ships. Commits from before the submodule existed have nothing to check.
# Release plan validation and the release controller both run this check.

set -euo pipefail

readonly PIN_DOC='https://github.com/radius-project/radius/blob/main/docs/contributing/contributing-releases/README.md#pin-radius-in-bicep-types-aws'

fail() {
    echo "Error: $*" >&2
    exit 1
}

usage() {
    cat << 'EOF'
Usage: verify-bicep-types-aws-radius-pin.sh <bicep-types-aws-dir> \
    <bicep-types-aws-commit> <radius-product-commit>
EOF
}

main() {
    local repository="${1:-}"
    local source_commit="${2:-}"
    local product_commit="${3:-}"
    local entry history mode object_type pinned fix

    if [[ $# -ne 3 ]]; then
        usage >&2
        exit 1
    fi
    [[ -d "${repository}" ]] \
        || fail "bicep-types-aws checkout not found: ${repository}"
    [[ "${source_commit}" =~ ^[0-9a-f]{40}$ ]] \
        || fail "bicep-types-aws commit must be a full commit SHA"
    [[ "${product_commit}" =~ ^[0-9a-f]{40}$ ]] \
        || fail "Radius product commit must be a full commit SHA"

    fix="See ${PIN_DOC}"
    entry="$(git -C "${repository}" ls-tree "${source_commit}" -- radius)" \
        || fail "could not read bicep-types-aws commit ${source_commit}"
    if [[ -z "${entry}" ]]; then
        history="$(git -C "${repository}" log -1 --format=%H \
            "${source_commit}" -- radius)" \
            || fail "could not read bicep-types-aws history at ${source_commit}"
        if [[ -n "${history}" ]]; then
            fail "bicep-types-aws commit ${source_commit} removed its radius submodule. ${fix}"
        fi
        echo "::notice::bicep-types-aws commit ${source_commit} has no radius submodule, so there is no Radius pin to check."
        return
    fi
    read -r mode object_type pinned _ <<< "${entry}"
    if [[ "${mode}" != "160000" || "${object_type}" != "commit" ]]; then
        fail "bicep-types-aws commit ${source_commit} has a radius path that is not a submodule. ${fix}"
    fi
    if [[ "${pinned}" != "${product_commit}" ]]; then
        fail "bicep-types-aws commit ${source_commit} pins Radius ${pinned}, but this release ships Radius ${product_commit}. ${fix}"
    fi
}

main "$@"
