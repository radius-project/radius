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

# ============================================================================
# Render the build job summary table shared by the trigger-scoped build
# workflows, and fail when any job did not succeed.
#
# Usage:
#   build-summary.sh "<job>:<result>" ["<job>:<result>" ...]
#
# A result of "success" or "skipped" passes; anything else fails the step.
# The table is appended to $GITHUB_STEP_SUMMARY when set, otherwise stdout.
# ============================================================================

set -euo pipefail

usage() {
    echo "Usage: $0 \"<job>:<result>\" [\"<job>:<result>\" ...]" >&2
}

main() {
    if [[ $# -eq 0 ]]; then
        echo "Error: at least one \"<job>:<result>\" argument is required" >&2
        usage
        exit 1
    fi

    local summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"
    local failed=0
    local entry job result icon

    for entry in "$@"; do
        job="${entry%%:*}"
        result="${entry##*:}"
        if [[ -z "${job}" || -z "${result}" || "${job}" == "${entry}" ]]; then
            echo "Error: malformed argument '${entry}', expected \"<job>:<result>\"" >&2
            exit 1
        fi
    done

    {
        echo "## Build Results Summary"
        echo
        echo "| Job | Status |"
        echo "|-----|--------|"

        for entry in "$@"; do
            job="${entry%%:*}"
            result="${entry##*:}"

            if [[ "${result}" == "success" || "${result}" == "skipped" ]]; then
                icon="✅"
            else
                icon="❌"
                failed=1
            fi

            echo "| ${job} | ${icon} ${result} |"
        done

        echo
        if ((failed == 1)); then
            echo "**Result: ❌ BUILD FAILED**"
        else
            echo "**Result: ✅ ALL BUILDS PASSED**"
        fi
    } >>"${summary_file}"

    if ((failed == 1)); then
        echo "::error::One or more build jobs failed or were cancelled" >&2
        exit 1
    fi

    echo "All build jobs completed successfully"
}

main "$@"
