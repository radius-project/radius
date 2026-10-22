// ------------------------------------------------------------
// Copyright 2026 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

// @ts-check

import { errorStatus, getRef } from "./reconcile-release-controller-lock.mjs";
import { readFile } from "node:fs/promises";
import path from "node:path";

const shaPattern = /^[0-9a-f]{40}$/;
const versionPattern = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.0-rc\.1$/;
const releaseAuthor = "radius-release[bot]";

function verifyAppToken(core) {
  if (core.getInput("APP_SLUG", { required: true }) !== "radius-release") {
    throw new Error(
      "Only the radius-release App may write frozen release metadata"
    );
  }
}

function metadataFiles(version) {
  return [
    "versions.yaml",
    "CHANGELOG.md",
    `docs/release-notes/${version}.md`,
    `.github/release-plans/${version}.yaml`
  ];
}

function verifyAppSignature(commit) {
  if (
    commit.commit?.verification?.verified !== true ||
    commit.author?.type !== "Bot" ||
    commit.author?.login !== releaseAuthor
  ) {
    throw new Error(
      "Frozen source is not a GitHub-verified release App commit"
    );
  }
}

async function commitMetadata(
  github,
  repository,
  branch,
  parent,
  additions,
  message
) {
  const { data: bot } = await github.rest.users.getByUsername({
    username: releaseAuthor
  });
  if (
    bot.login !== releaseAuthor ||
    bot.type !== "Bot" ||
    !Number.isSafeInteger(bot.id) ||
    bot.id < 1
  ) {
    throw new Error("Could not resolve the release App sign-off identity");
  }
  const signoff = `${releaseAuthor} <${bot.id}+${releaseAuthor}@users.noreply.github.com>`;
  return github.graphql(
    `
    mutation($input: CreateCommitOnBranchInput!) {
      createCommitOnBranch(input: $input) { commit { oid } }
    }
  `,
    {
      input: {
        branch: { repositoryNameWithOwner: repository, branchName: branch },
        expectedHeadOid: parent,
        message: {
          ...message,
          body: `${message.body}\n\nSigned-off-by: ${signoff}`
        },
        fileChanges: { additions }
      }
    }
  );
}

// Keep existing first-RC review history instead of rebasing/replacing the
// preparation branch. The CAS rejects PM edits made after our notes snapshot.
/** @param {{github: any, context: any, core: any}} options */
export async function updateFrozenPreparation({ github, context, core }) {
  verifyAppToken(core);
  const version = core.getInput("VERSION", { required: true });
  const head = core.getInput("HEAD_SHA", { required: true });
  const number = Number(core.getInput("PR_NUMBER", { required: true }));
  const output = core.getInput("OUTPUT_DIR", { required: true });
  const workspace = core.getInput("WORKSPACE", { required: true });
  if (
    !versionPattern.test(version) ||
    !shaPattern.test(head) ||
    !Number.isSafeInteger(number) ||
    number < 1
  ) {
    throw new Error("Invalid frozen preparation identity");
  }
  const { owner, repo } = context.repo;
  const branch = `automation/prepare-release-${version.slice(1)}`;
  const { data: pull } = await github.rest.pulls.get({
    owner,
    repo,
    pull_number: number
  });
  if (
    pull.state !== "open" ||
    pull.merged_at ||
    pull.base.ref !== "main" ||
    pull.head.ref !== branch ||
    pull.head.repo?.full_name !== `${owner}/${repo}` ||
    pull.head.sha !== head
  ) {
    throw new Error(
      "Release PR changed during preparation; rerun to retain its latest curation"
    );
  }
  const additions = [];
  for (const file of metadataFiles(version)) {
    const contents = (await readFile(path.join(workspace, file))).toString(
      "base64"
    );
    const { data } = await github.rest.repos.getContent({
      owner,
      repo,
      ref: head,
      path: file
    });
    if (
      Array.isArray(data) ||
      data.type !== "file" ||
      data.encoding !== "base64" ||
      !data.content
    ) {
      throw new Error(`Expected existing release metadata at ${file}`);
    }
    if (data.content.replace(/\s/g, "") !== contents)
      additions.push({ path: file, contents });
  }
  if (additions.length) {
    const result = await commitMetadata(
      github,
      `${owner}/${repo}`,
      branch,
      head,
      additions,
      {
        headline: `chore(release): prepare ${version}`,
        body: "Retain the frozen source and reviewed release-note sections."
      }
    );
    const sha = result.createCommitOnBranch.commit.oid;
    const { data: commit } = await github.rest.repos.getCommit({
      owner,
      repo,
      ref: sha
    });
    verifyAppSignature(commit);
    if (commit.parents?.length !== 1 || commit.parents[0].sha !== head) {
      throw new Error("Preparation commit did not retain the reviewed PR head");
    }
  }
  await github.rest.pulls.update({
    owner,
    repo,
    pull_number: number,
    body: await readFile(path.join(output, "release-pr-body.md"), "utf8")
  });
  core.setOutput("pull-request-url", pull.html_url);
}

/** @param {{github: any, context: any, core: any}} options */
export default async function reconcileFrozenReleaseSource({
  github,
  context,
  core
}) {
  const mode = core.getInput("MODE", { required: true });
  const version = core.getInput("VERSION", { required: true });
  const product = core.getInput("PRODUCT_COMMIT", { required: true });
  const metadata = core.getInput("SOURCE_PR_COMMIT", { required: true });
  const tree = core.getInput("RELEASE_TREE", { required: true });
  const requested = core.getInput("RELEASE_COMMIT", { required: true });
  const releaseVersion = version.match(versionPattern);
  if (!["read", "create"].includes(mode) || !releaseVersion) {
    throw new Error("Frozen sources require read/create mode and a first RC");
  }
  if (mode === "create") verifyAppToken(core);
  if (
    ![product, metadata, tree, requested].every((sha) => shaPattern.test(sha))
  ) {
    throw new Error("Frozen source identities must be full SHAs");
  }
  const { owner, repo } = context.repo;
  const branch = `automation/release-source-${version.slice(1)}`;
  const trailer = `(cherry picked from commit ${metadata})`;
  const { data: approved } = await github.rest.repos.getCommit({
    owner,
    repo,
    ref: metadata
  });
  if (approved.commit?.verification?.verified !== true) {
    throw new Error("Approved release metadata commit is not GitHub-verified");
  }

  async function verify(reference) {
    const sha = reference.object.sha;
    if (requested !== metadata && requested !== sha) {
      throw new Error(
        "Requested release commit differs from the frozen source ref"
      );
    }
    const { data: commit } = await github.rest.repos.getCommit({
      owner,
      repo,
      ref: sha
    });
    if (
      commit.parents?.length !== 1 ||
      commit.parents[0].sha !== product ||
      commit.commit?.tree?.sha !== tree ||
      !commit.commit?.message?.split("\n").includes(trailer)
    ) {
      throw new Error(
        "Frozen source history/tree differs from the approved release"
      );
    }
    verifyAppSignature(commit);
    const observed = await getRef(github, owner, repo, branch);
    if (observed?.object.sha !== sha) {
      throw new Error("Frozen source ref changed during verification");
    }
    core.setOutput("release-commit", sha);
    core.setOutput("release-identifier", `${version}-${sha}`);
    core.setOutput("resolved", "true");
  }

  let reference = await getRef(github, owner, repo, branch);
  if (reference && reference.object.sha !== product) {
    await verify(reference);
    return;
  }
  if (requested !== metadata) {
    throw new Error(
      "Requested release commit has no matching frozen source ref"
    );
  }
  // Do not start a new history underneath a release that already exists.
  for (const ref of [
    `heads/release/${releaseVersion[1]}.${releaseVersion[2]}`,
    `tags/${version}`,
    `heads/automation/release-state-${version.slice(1)}`
  ]) {
    try {
      await github.rest.git.getRef({ owner, repo, ref });
    } catch (error) {
      if (errorStatus(error) === 404) continue;
      throw error;
    }
    throw new Error(`${ref} exists without a resolved frozen source`);
  }
  if (mode === "read") {
    core.setOutput("resolved", "false");
    return;
  }
  const files = metadataFiles(version);
  const additions = await Promise.all(
    files.map(async (path) => {
      const { data } = await github.rest.repos.getContent({
        owner,
        repo,
        ref: metadata,
        path
      });
      if (
        Array.isArray(data) ||
        data.type !== "file" ||
        data.encoding !== "base64" ||
        !data.content
      ) {
        throw new Error(`Expected approved release metadata at ${path}`);
      }
      return { path, contents: data.content.replace(/\s/g, "") };
    })
  );
  if (!reference) {
    try {
      await github.rest.git.createRef({
        owner,
        repo,
        ref: `refs/heads/${branch}`,
        sha: product
      });
    } catch (error) {
      if (errorStatus(error) !== 422) throw error;
    }
    reference = await getRef(github, owner, repo, branch);
    if (!reference) throw new Error(`Could not create ${branch}`);
    if (reference.object.sha !== product) {
      await verify(reference);
      return;
    }
  }

  // The App-authenticated API signs one atomic commit, guarded by its parent.
  // An interrupted request is adopted only after its exact tree and signature
  // have been checked; this ref is never reset or force-updated.
  try {
    await commitMetadata(
      github,
      `${owner}/${repo}`,
      branch,
      product,
      additions,
      {
        headline: `chore(release): freeze ${version}`,
        body: trailer
      }
    );
  } catch (error) {
    reference = await getRef(github, owner, repo, branch);
    if (!reference || reference.object.sha === product) throw error;
    core.notice(
      "Adopting the frozen commit after an interrupted creation response"
    );
  }
  reference = await getRef(github, owner, repo, branch);
  if (!reference || reference.object.sha === product) {
    throw new Error(`No frozen metadata commit was created on ${branch}`);
  }
  await verify(reference);
}
