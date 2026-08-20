// Copyright 2026 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// @ts-check

const COMMENT_MARKER =
  "<!-- Sticky Pull Request Commentcommit-signature-verification -->";

const COMMITS_QUERY = `
  query(
    $owner: String!
    $repo: String!
    $number: Int!
    $cursor: String
  ) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        commits(first: 100, after: $cursor) {
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
  }
`;

/**
 * @param {{ oid: string, signature?: { isValid: boolean, state: string } | null }} commit
 * @returns {boolean}
 */
export function hasVerifiedSignature(commit) {
  if (!/^[0-9a-f]{40}$/.test(commit.oid)) {
    throw new Error("GitHub returned an invalid commit OID");
  }

  return (
    commit.signature?.isValid === true && commit.signature.state === "VALID"
  );
}

/**
 * @param {import('@actions/github-script').AsyncFunctionArguments['github']} github
 * @param {{ owner: string, repo: string }} repository
 * @param {number} number
 * @returns {Promise<Array<{ oid: string, signature?: { isValid: boolean, state: string } | null }>>}
 */
export async function listPullRequestCommits(github, repository, number) {
  const commits = [];
  let cursor = null;

  do {
    const result = await github.graphql(COMMITS_QUERY, {
      ...repository,
      number,
      cursor,
    });
    const pullRequest = result.repository.pullRequest;
    if (!pullRequest) {
      throw new Error(`Pull request #${number} was not found`);
    }

    const page = pullRequest.commits;
    commits.push(...page.nodes.map((node) => node.commit));
    if (page.pageInfo.hasNextPage && !page.pageInfo.endCursor) {
      throw new Error("GitHub returned an invalid pagination cursor");
    }
    cursor = page.pageInfo.hasNextPage ? page.pageInfo.endCursor : null;
  } while (cursor);

  return commits;
}

/**
 * @param {string[]} unverified
 * @param {{ owner: string, repo: string }} repository
 * @param {string} serverUrl
 * @returns {string}
 */
export function formatGuidance(unverified, repository, serverUrl) {
  const guideUrl =
    `${serverUrl}/radius-project/radius/blob/main/` +
    "docs/contributing/contributing-code/" +
    "contributing-code-first-commit/" +
    "first-commit-06-creating-a-pr/index.md#signing-your-commits";
  const commitLines = unverified.map(
    (oid) =>
      `- [\`${oid.slice(0, 7)}\`](` +
      `${serverUrl}/${repository.owner}/` +
      `${repository.repo}/commit/${oid})`,
  );

  return [
    "Thank you for contributing to Radius.",
    "",
    "The following commits do not currently show a GitHub **Verified** signature:",
    "",
    ...commitLines,
    "",
    `Please follow our [commit-signing guide](${guideUrl}) to configure ` +
      "GPG, SSH, or S/MIME signing. Then re-sign the affected commits " +
      "and force-push the rewritten branch with `--force-with-lease`.",
    "",
    "Cryptographic commit signing is separate from the DCO " +
      "`Signed-off-by` line; both are required.",
    "",
    COMMENT_MARKER,
  ].join("\n");
}

/** @param {import('@actions/github-script').AsyncFunctionArguments} AsyncFunctionArguments */
export default async ({ github, context, core }) => {
  const prNumber = Number(process.env.PR_NUMBER);
  if (!Number.isSafeInteger(prNumber) || prNumber <= 0) {
    throw new Error("PR_NUMBER must be a positive integer");
  }

  const commits = await listPullRequestCommits(github, context.repo, prNumber);
  const unverified = commits
    .filter((commit) => !hasVerifiedSignature(commit))
    .map((commit) => commit.oid);

  if (unverified.length === 0) {
    core.info(
      "All pull request commits have Verified signatures. " +
        "Existing guidance comments are retained.",
    );
    return;
  }

  core.warning(
    `${unverified.length} commit(s) do not have Verified signatures.`,
  );
  for (const oid of unverified) {
    core.info(`- ${oid.slice(0, 7)}`);
  }

  const serverUrl = process.env.GITHUB_SERVER_URL || "https://github.com";
  const body = formatGuidance(unverified, context.repo, serverUrl);
  const comments = await github.paginate(github.rest.issues.listComments, {
    ...context.repo,
    issue_number: prNumber,
    per_page: 100,
  });
  const existing = comments.find(
    (comment) =>
      comment.user?.login === "github-actions[bot]" &&
      comment.body?.includes(COMMENT_MARKER),
  );

  if (!existing) {
    await github.rest.issues.createComment({
      ...context.repo,
      issue_number: prNumber,
      body,
    });
    core.info("Posted the signature guidance comment.");
  } else if (existing.body !== body) {
    await github.rest.issues.updateComment({
      ...context.repo,
      comment_id: existing.id,
      body,
    });
    core.info("Updated the signature guidance comment.");
  } else {
    core.info("The signature guidance comment is already current.");
  }
};
