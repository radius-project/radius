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

import assert from "node:assert/strict";
import { writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { tool } from "./bicep-types.mjs";
import { readPublishedReleaseManifest } from "./release-assets.mjs";

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
    head: `${owner}:${branch}`,
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

export async function verifyReleaseBuildSource({ github, core, run = tool }) {
  const version = core.getInput("VERSION", { required: true });
  const sourceSha = core.getInput("SOURCE_SHA", { required: true });
  assert.match(version, versionPattern);
  assert.match(sourceSha, commitPattern);
  const planFile = core.getInput("PLAN_FILE", { required: true });
  const plan = JSON.parse(await run(["yq", "-o=json", ".", planFile]));
  assert.equal(plan.version, version);
  const manifest = await readPublishedReleaseManifest(
    github,
    version,
    sourceSha
  );
  let engine;
  if (manifest) {
    assert.equal(manifest.channel, plan.channel);
    assert.equal(manifest.releaseType, plan.releaseType);
    const engines = manifest.observed?.imageLocks?.filter(
      (entry) => entry.name === "deployment-engine"
    );
    assert.equal(
      engines?.length,
      1,
      "Missing published Deployment Engine lock"
    );
    [engine] = engines;
    assert.match(engine.sourceSha, commitPattern);
    assert.match(engine.digest, /^sha256:[0-9a-f]{64}$/);
    assert.equal(
      engine.reference,
      `ghcr.io/radius-project/deployment-engine:${version.slice(1)}`
    );
    const siblings = plan.siblingRepositories.filter(
      (entry) => entry.name === "bicep-types-aws"
    );
    assert.equal(siblings.length, 1, "Expected exactly one planned AWS source");
    assert.equal(siblings[0].repository, "radius-project/bicep-types-aws");
    const { data: tag } = await github.rest.repos.getCommit({
      owner: "radius-project",
      repo: "bicep-types-aws",
      ref: `refs/tags/${version}`
    });
    assert.equal(
      tag.sha,
      siblings[0].sourceCommit,
      "Published release AWS tag is missing or differs from the approved source; never recreate it"
    );
  }
  const targetsFile = join(dirname(planFile), "approved-targets.json");
  await writeFile(targetsFile, JSON.stringify(plan.expectedOutputs));
  const args = [
    "bash",
    "./.github/scripts/verify-release-build-contract.sh",
    sourceSha,
    "--expected-targets",
    targetsFile
  ];
  if (plan.changelogPolicy === "stable-only")
    args.push("--require-sibling-releases");
  if (!manifest)
    args.push("--require-current-bicep", "--require-current-engine");
  await run(args);
  if (engine) {
    core.setOutput("de-source-commit", engine.sourceSha);
    core.setOutput("de-digest", engine.digest);
  }
  core.setOutput("published", String(Boolean(manifest)));
}
