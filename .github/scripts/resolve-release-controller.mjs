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

const versionPattern = /^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/;
const releasePullPattern =
  /^automation\/prepare-release-(\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?)$/;
const backportPattern = /^automation\/backport-([1-9]\d*)-to-(\d+\.\d+)$/;
const commitPattern = /^[0-9a-f]{40}$/;

function releasePullVersion(pull, repository) {
  const match = pull.head.ref.match(releasePullPattern);
  if (
    !match ||
    pull.base.ref !== "main" ||
    pull.head.repo?.full_name !== repository ||
    !pull.merged_at ||
    !commitPattern.test(pull.merge_commit_sha ?? "")
  ) {
    return "";
  }
  return `v${match[1]}`;
}

async function findReleasePull(github, owner, repo, version) {
  const branch = `automation/prepare-release-${version.slice(1)}`;
  const pulls = await github.paginate(github.rest.pulls.list, {
    owner,
    repo,
    base: "main",
    state: "closed",
    per_page: 100
  });
  const repository = `${owner}/${repo}`;
  const matches = pulls.filter(
    (pull) =>
      pull.head.ref === branch &&
      releasePullVersion(pull, repository) === version
  );
  if (matches.length !== 1) {
    throw new Error(
      `Expected one merged generated release pull request for ${version}, found ${matches.length}`
    );
  }
  return matches[0];
}

function setSelection(core, sourcePull, version, releaseCommit, trigger) {
  if (!commitPattern.test(releaseCommit ?? "")) {
    throw new Error("Release commit must be a full commit SHA");
  }
  core.setOutput("selected", "true");
  core.setOutput("source-pr-number", String(sourcePull.number));
  core.setOutput("source-pr-commit", sourcePull.merge_commit_sha);
  core.setOutput("version", version);
  core.setOutput("release-commit", releaseCommit);
  core.setOutput("trigger", trigger);
}

/** @param {{github: any, context: any, core: any}} options */
export default async function resolveReleaseController({
  github,
  context,
  core
}) {
  const mode = core.getInput("MODE", { required: true });
  const repository = `${context.repo.owner}/${context.repo.repo}`;
  let sourcePull;
  let version;
  let releaseCommit;
  let trigger;

  if (mode === "dispatch") {
    version = core.getInput("VERSION", { required: true });
    releaseCommit = core.getInput("SOURCE_SHA", { required: true });
    if (!versionPattern.test(version)) {
      throw new Error("VERSION must use vX.Y.Z or vX.Y.Z-rc.N format");
    }
    if (!commitPattern.test(releaseCommit)) {
      throw new Error("SOURCE_SHA must be a full commit SHA");
    }
    sourcePull = await findReleasePull(
      github,
      context.repo.owner,
      context.repo.repo,
      version
    );
    trigger = "dispatch";
  } else if (mode === "event") {
    const pull = context.payload.pull_request;
    if (!pull?.merged) {
      core.setOutput("selected", "false");
      return;
    }

    version = releasePullVersion(pull, repository);
    if (version) {
      sourcePull = pull;
      releaseCommit = pull.merge_commit_sha;
      trigger = "release-pr";
    } else {
      const backport = pull.head.ref.match(backportPattern);
      if (
        !backport ||
        pull.base.ref !== `release/${backport[2]}` ||
        pull.head.repo?.full_name !== repository
      ) {
        core.setOutput("selected", "false");
        return;
      }
      const { data } = await github.rest.pulls.get({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: Number(backport[1])
      });
      sourcePull = data;
      version = releasePullVersion(sourcePull, repository);
      if (!version) {
        core.setOutput("selected", "false");
        return;
      }
      releaseCommit = pull.merge_commit_sha;
      trigger = "release-backport";
    }
  } else {
    throw new Error("MODE must be event or dispatch");
  }

  setSelection(core, sourcePull, version, releaseCommit, trigger);
}

export { findReleasePull, releasePullVersion };
