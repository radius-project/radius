#!/usr/bin/env bash
# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
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

# diff-resource-types.sh generates a markdown diff report comparing default-
# registered resource type manifests between two versions of the
# resource-types-contrib Go module.
#
# Required environment variables:
#   OLD_VERSION - The old module version (empty if new dependency)
#   NEW_VERSION - The new module version
#
# Output:
#   /tmp/diff-report.md - Markdown report suitable for a PR comment

set -euo pipefail

readonly MODULE="github.com/radius-project/resource-types-contrib"
readonly REPORT_FILE="/tmp/diff-report.md"

# Cleanup temp files on exit.
cleanup() {
    rm -f /tmp/diff-report-truncated.md
}
trap cleanup EXIT

# Download both versions and get their local paths.
OLD_DIR=""
if [[ -n "${OLD_VERSION:-}" ]]; then
    OLD_DIR=$(go mod download -json "${MODULE}@${OLD_VERSION}" | jq -r '.Dir')
fi
NEW_DIR=$(go mod download -json "${MODULE}@${NEW_VERSION}" | jq -r '.Dir')

# Read the list of default resource types from defaults.yaml in both versions.
# We diff only files listed in defaults.yaml (the authoritative list of what
# gets embedded into the Radius binary), not the entire module.
OLD_FILES=""
NEW_FILES=""
if [[ -n "${OLD_DIR}" ]] && [[ -f "${OLD_DIR}/defaults.yaml" ]]; then
    OLD_FILES=$(yq '.defaultRegistration[]' "${OLD_DIR}/defaults.yaml" 2>/dev/null || echo "")
fi
if [[ -f "${NEW_DIR}/defaults.yaml" ]]; then
    NEW_FILES=$(yq '.defaultRegistration[]' "${NEW_DIR}/defaults.yaml" 2>/dev/null || echo "")
fi

ALL_FILES=$(echo -e "${OLD_FILES}\n${NEW_FILES}" | sort -u | grep -v '^$' || true)

if [[ -z "${ALL_FILES}" ]]; then
    echo "No default manifests found in defaults.yaml"
    exit 0
fi

# Build the diff report.
DIFF_OUTPUT=""

# Check if defaults.yaml itself changed.
if [[ -n "${OLD_DIR}" ]] && [[ -f "${OLD_DIR}/defaults.yaml" ]] && [[ -f "${NEW_DIR}/defaults.yaml" ]]; then
    DEFAULTS_DIFF=$(diff -u "${OLD_DIR}/defaults.yaml" "${NEW_DIR}/defaults.yaml" || true)
    if [[ -n "${DEFAULTS_DIFF}" ]]; then
        DIFF_OUTPUT+="### Changed: \`defaults.yaml\`"$'\n'
        DIFF_OUTPUT+='```diff'$'\n'
        DIFF_OUTPUT+="${DEFAULTS_DIFF}"$'\n'
        DIFF_OUTPUT+='```'$'\n\n'
    fi
elif [[ -z "${OLD_DIR}" || ! -f "${OLD_DIR}/defaults.yaml" ]] && [[ -f "${NEW_DIR}/defaults.yaml" ]]; then
    DIFF_OUTPUT+="### Added: \`defaults.yaml\`"$'\n'
    DIFF_OUTPUT+='```yaml'$'\n'
    DIFF_OUTPUT+=$(cat "${NEW_DIR}/defaults.yaml")$'\n'
    DIFF_OUTPUT+='```'$'\n\n'
fi

# Diff each default-registered manifest file.
while IFS= read -r entry; do
    [[ -z "${entry}" ]] && continue

    # Resolve resource type name to file path.
    # Format: Radius.<Namespace>/<typeName> -> <Namespace>/<typeName>/<typeName>.yaml
    NAMESPACE_SUFFIX=$(echo "${entry}" | cut -d'/' -f1 | sed 's/^Radius\.//')
    TYPE_NAME=$(echo "${entry}" | cut -d'/' -f2)
    file="${NAMESPACE_SUFFIX}/${TYPE_NAME}/${TYPE_NAME}.yaml"

    OLD_FILE=""
    if [[ -n "${OLD_DIR}" ]]; then
        OLD_FILE="${OLD_DIR}/${file}"
    fi
    NEW_FILE="${NEW_DIR}/${file}"

    if [[ -z "${OLD_FILE}" ]] || [[ ! -f "${OLD_FILE}" ]]; then
        if [[ -f "${NEW_FILE}" ]]; then
            DIFF_OUTPUT+="### Added: \`${file}\`"$'\n'
            DIFF_OUTPUT+='```yaml'$'\n'
            DIFF_OUTPUT+=$(head -c 10000 "${NEW_FILE}")$'\n'
            DIFF_OUTPUT+='```'$'\n\n'
        fi
    elif [[ -f "${OLD_FILE}" ]] && [[ ! -f "${NEW_FILE}" ]]; then
        DIFF_OUTPUT+="### Removed: \`${file}\`"$'\n\n'
    elif [[ -f "${OLD_FILE}" ]] && [[ -f "${NEW_FILE}" ]]; then
        FILE_DIFF=$(diff -u "${OLD_FILE}" "${NEW_FILE}" || true)
        if [[ -n "${FILE_DIFF}" ]]; then
            DIFF_OUTPUT+="### Changed: \`${file}\`"$'\n'
            DIFF_OUTPUT+='```diff'$'\n'
            DIFF_OUTPUT+=$(echo "${FILE_DIFF}" | head -200)$'\n'
            DIFF_OUTPUT+='```'$'\n\n'
        fi
    fi
done <<< "${ALL_FILES}"

if [[ -z "${DIFF_OUTPUT}" ]]; then
    DIFF_OUTPUT="No changes to default-registered manifest files."
fi

# Write the full report.
{
    echo "## Resource Types Diff Report"
    echo ""
    echo "Changes in default-registered manifests between \`${OLD_VERSION:-*(new dependency)*}\` and \`${NEW_VERSION}\`:"
    echo ""
    echo "${DIFF_OUTPUT}"
    echo "---"
    echo "<sub>This report shows only files listed in <code>defaults.yaml</code>. Generated automatically by the resource-types-diff workflow.</sub>"
} > "${REPORT_FILE}"

# Truncate if too large for PR comment (GitHub limit is 65536 chars).
# Close any open code fence before appending the truncation footer.
if [[ $(wc -c < "${REPORT_FILE}") -gt 60000 ]]; then
    head -c 59000 "${REPORT_FILE}" > /tmp/diff-report-truncated.md
    echo "" >> /tmp/diff-report-truncated.md
    echo '```' >> /tmp/diff-report-truncated.md
    echo "" >> /tmp/diff-report-truncated.md
    echo "---" >> /tmp/diff-report-truncated.md
    echo "*Report truncated. See [resource-types-contrib](https://github.com/radius-project/resource-types-contrib) for the full diff.*" >> /tmp/diff-report-truncated.md
    mv /tmp/diff-report-truncated.md "${REPORT_FILE}"
fi

echo "Diff report written to ${REPORT_FILE}"
