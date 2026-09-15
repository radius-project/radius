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

# Shared Radius release-version policy; source this file from release scripts.
#
# New release candidates use the dotted SemVer form X.Y.Z-rc.N. Historical
# X.Y.Z-rcN tags stay accepted because previous-tag selection, verification,
# and upgrades must still read them. The two forms are never mixed within one
# version: SemVer orders rc.2 before rc1, so a dotted RC that follows a legacy
# RC of the same version would be rejected by the upgrade preflight as a
# downgrade. The dotted form starts with the first RC of a version.
#
# The constants below are readonly, so guard against being sourced twice.
if [[ -n "${RADIUS_RELEASE_VERSION_POLICY_LOADED:-}" ]]; then
    return 0
fi
RADIUS_RELEASE_VERSION_POLICY_LOADED=1

readonly RADIUS_SEMVER_NUMBER='(0|[1-9][0-9]*)'
readonly RADIUS_RC_NUMBER='[1-9][0-9]*'
RADIUS_RELEASE_VERSION_PATTERN="^${RADIUS_SEMVER_NUMBER}\\.${RADIUS_SEMVER_NUMBER}"
RADIUS_RELEASE_VERSION_PATTERN+="\\.${RADIUS_SEMVER_NUMBER}"
RADIUS_RELEASE_VERSION_PATTERN+="(-rc(\\.${RADIUS_RC_NUMBER}|${RADIUS_RC_NUMBER}))?$"
readonly RADIUS_RELEASE_VERSION_PATTERN
readonly RADIUS_LEGACY_RC_PATTERN="-rc${RADIUS_RC_NUMBER}$"

is_radius_release_version() {
    local version="$1"

    [[ "${version}" =~ ${RADIUS_RELEASE_VERSION_PATTERN} ]]
}

is_legacy_rc_version() {
    local version="$1"

    [[ "${version}" =~ ${RADIUS_LEGACY_RC_PATTERN} ]]
}

canonical_radius_rc_version() {
    local version="$1"

    if is_legacy_rc_version "${version}"; then
        printf '%s\n' "${version/-rc/-rc.}"
    else
        printf '%s\n' "${version}"
    fi
}
