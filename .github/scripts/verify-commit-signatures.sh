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

# Lists pull request commits that do not have a GitHub Verified signature.
# Requires the GitHub CLI and jq.

set -euo pipefail

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

require_environment() {
    local name
    for name in GH_TOKEN GITHUB_REPOSITORY PR_NUMBER; do
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

    local response
    response="$(
        gh api graphql --paginate --slurp \
            -f query="${GRAPHQL_QUERY}" \
            -F owner="${owner}" \
            -F repo="${repo}" \
            -F number="${PR_NUMBER}"
    )"

    if ! jq -e \
        'all(.[]; .data.repository.pullRequest != null)' \
        <<< "${response}" > /dev/null; then
        echo "Error: pull request ${PR_NUMBER} was not found" >&2
        return 1
    fi

    jq -c '
      [
        .[] |
        .data.repository.pullRequest.commits.nodes[] |
        select(
          .commit.signature == null or
          .commit.signature.isValid != true or
          .commit.signature.state != "VALID"
        ) |
        .commit.oid
      ]
    ' <<< "${response}"
}

main "$@"
