#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# Verifies that every pull request commit has a GitHub Verified signature.
# Requires the GitHub CLI and jq.

set -euo pipefail

readonly COMMENT_MARKER="<!-- radius-commit-signature-verification -->"
readonly GUIDE_DIR="docs/contributing/contributing-code"
readonly GUIDE_SECTION="contributing-code-first-commit"
readonly GUIDE_PAGE="first-commit-06-creating-a-pr/index.md"
readonly GUIDE_PATH="${GUIDE_DIR}/${GUIDE_SECTION}/${GUIDE_PAGE}"
readonly GITHUB_ORIGIN="${GITHUB_SERVER_URL:-https://github.com}"
readonly GUIDE_REPO="${GITHUB_ORIGIN}/radius-project/radius"
readonly GUIDE_BLOB_URL="${GUIDE_REPO}/blob/main/${GUIDE_PATH}"
readonly GUIDE_URL="${GUIDE_BLOB_URL}#signing-your-commits"
# shellcheck disable=SC2016 # GraphQL variables must remain literal.
readonly GRAPHQL_QUERY='
query(
  $endCursor: String
  $number: Int!
  $owner: String!
  $repo: String!
) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      commits(first: 100, after: $endCursor) {
        nodes {
          commit {
            oid
            signature {
              isValid
              state
            }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}'

TEMP_DIR=""

cleanup() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}

trap cleanup EXIT

require_environment() {
    local name
    for name in GH_TOKEN GITHUB_REPOSITORY PR_AUTHOR PR_NUMBER; do
        if [[ -z "${!name:-}" ]]; then
            echo "Error: ${name} is required" >&2
            return 1
        fi
    done
}

require_commands() {
    local command
    for command in gh jq; do
        if ! command -v "${command}" > /dev/null; then
            echo "Error: ${command} is required" >&2
            return 1
        fi
    done
}

delete_comment() {
    local comment_id="$1"
    gh api --method DELETE \
        "/repos/${GITHUB_REPOSITORY}/issues/comments/${comment_id}"
}

main() {
    require_environment
    require_commands

    local owner
    local repo
    IFS="/" read -r owner repo <<< "${GITHUB_REPOSITORY}"
    if [[ -z "${owner}" || -z "${repo}" ]]; then
        echo "Error: GITHUB_REPOSITORY must use owner/repo format" >&2
        return 1
    fi

    TEMP_DIR="$(mktemp -d)"
    local commits_file="${TEMP_DIR}/commits.json"
    local unverified_file="${TEMP_DIR}/unverified.txt"
    local comments_file="${TEMP_DIR}/comments.json"
    local comment_ids_file="${TEMP_DIR}/comment-ids.txt"
    local comment_body_file="${TEMP_DIR}/comment.md"
    local issue_endpoint="/repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}"
    local comments_endpoint="/repos/${GITHUB_REPOSITORY}/issues/comments"

    gh api graphql --paginate --slurp \
        -f query="${GRAPHQL_QUERY}" \
        -F owner="${owner}" \
        -F repo="${repo}" \
        -F number="${PR_NUMBER}" > "${commits_file}"

    if ! jq -e \
        'all(.[]; .data.repository.pullRequest != null)' \
        "${commits_file}" > /dev/null; then
        echo "Error: pull request ${PR_NUMBER} was not found" >&2
        return 1
    fi

    jq -r '
      .[] |
      .data.repository.pullRequest.commits.nodes[] |
      select(
        .commit.signature == null or
        .commit.signature.isValid != true or
        .commit.signature.state != "VALID"
      ) |
      .commit.oid
    ' "${commits_file}" > "${unverified_file}"

    gh api --paginate --slurp \
        "${issue_endpoint}/comments?per_page=100" > "${comments_file}"

    jq -r --arg marker "${COMMENT_MARKER}" '
      .[][] |
      select(.user.login == "github-actions[bot]") |
      select(.body | contains($marker)) |
      .id
    ' "${comments_file}" > "${comment_ids_file}"

    local -a unverified_commits
    local -a comment_ids
    mapfile -t unverified_commits < "${unverified_file}"
    mapfile -t comment_ids < "${comment_ids_file}"

    if ((${#unverified_commits[@]} == 0)); then
        local comment_id
        for comment_id in "${comment_ids[@]}"; do
            delete_comment "${comment_id}"
        done
        echo "All pull request commits have Verified signatures."
        return
    fi

    {
        echo "Hi @${PR_AUTHOR}, thank you for contributing to Radius."
        echo
        echo "The following commits do not currently show a GitHub" \
            "**Verified** signature:"
        echo
        local commit
        for commit in "${unverified_commits[@]}"; do
            printf -- "- [\`%s\`](%s/%s/commit/%s)\n" \
                "${commit:0:7}" \
                "${GITHUB_ORIGIN}" \
                "${GITHUB_REPOSITORY}" \
                "${commit}"
        done
        echo
        echo "Please follow our [commit-signing guide](${GUIDE_URL}) to" \
            "configure GPG, SSH, or S/MIME signing. Then re-sign the affected" \
            "commits and force-push the rewritten branch with" \
            "\`--force-with-lease\`."
        echo
        echo "Cryptographic commit signing is separate from the DCO" \
            "\`Signed-off-by\` line; both are required."
        echo
        echo "${COMMENT_MARKER}"
    } > "${comment_body_file}"

    if ((${#comment_ids[@]} == 0)); then
        jq -n --rawfile body "${comment_body_file}" '{body: $body}' |
            gh api --method POST \
                "${issue_endpoint}/comments" \
                --input - > /dev/null
    else
        jq -n --rawfile body "${comment_body_file}" '{body: $body}' |
            gh api --method PATCH \
                "${comments_endpoint}/${comment_ids[0]}" \
                --input - > /dev/null

        local duplicate_id
        for duplicate_id in "${comment_ids[@]:1}"; do
            delete_comment "${duplicate_id}"
        done
    fi

    printf '%s%d%s\n' \
        "::warning title=Unverified commit signatures::" \
        "${#unverified_commits[@]}" \
        " commit(s) do not show a Verified signature."
}

main "$@"
