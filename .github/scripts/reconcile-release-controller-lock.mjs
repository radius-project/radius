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

import { tagCommit } from "./coordinate-release.mjs";

const commitPattern = /^[0-9a-f]{40}$/;
const digestPattern = /^sha256:[0-9a-f]{64}$/;
const versionPattern = /^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/;
const finalVersionPattern = /^v\d+\.\d+\.\d+$/;
const lockPath = ".github/release-state/deployment-engine.json";

/** @param {string} version */
function lockBranch(version) {
  return `automation/release-state-${version.slice(1)}`;
}

function errorStatus(error) {
  return Number(error?.status ?? error?.response?.status ?? 0);
}

function canonicalLock(inputs, digest) {
  return {
    schemaVersion: 1,
    version: inputs.version,
    releaseSourceCommit: inputs.releaseSourceCommit,
    deploymentEngine: {
      signedTag: inputs.signedTag,
      sourceCommit: inputs.deSourceCommit,
      image: "ghcr.io/radius-project/deployment-engine",
      imageTag: inputs.imageTag,
      digest
    }
  };
}

function parseLock(content) {
  const decoded = Buffer.from(content.replace(/\s/g, ""), "base64").toString(
    "utf8"
  );
  return JSON.parse(decoded);
}

function equalJSON(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

/** @param {any} github @param {string} owner @param {string} repo @param {string} branch */
async function getRef(github, owner, repo, branch) {
  try {
    const { data } = await github.rest.git.getRef({
      owner,
      repo,
      ref: `heads/${branch}`
    });
    return data;
  } catch (error) {
    if (errorStatus(error) === 404) {
      return undefined;
    }
    throw error;
  }
}

/** @param {any} github @param {string} owner @param {string} repo @param {string} ref @param {string} path */
async function getContent(github, owner, repo, ref, path) {
  try {
    const { data } = await github.rest.repos.getContent({
      owner,
      repo,
      path,
      ref
    });
    if (Array.isArray(data) || data.type !== "file" || !data.content) {
      throw new Error(`Expected a release lock file at ${path}`);
    }
    return data;
  } catch (error) {
    if (errorStatus(error) === 404) {
      return undefined;
    }
    throw error;
  }
}

// The controller writes the lock through the API with an App token, which
// GitHub signs. A person-authored or unsigned commit means the lock was
// replaced outside the controller.
/** @param {any} github @param {string} owner @param {string} repo @param {string} branch @param {string} sha @param {string} releaseSourceCommit */
async function verifyLockCommit(
  github,
  owner,
  repo,
  branch,
  sha,
  releaseSourceCommit
) {
  const { data: commit } = await github.rest.repos.getCommit({
    owner,
    repo,
    ref: sha
  });
  if (
    commit.parents?.length !== 1 ||
    commit.parents[0].sha !== releaseSourceCommit
  ) {
    throw new Error(`Release state branch ${branch} has unexpected history`);
  }
  if (
    commit.commit?.verification?.verified !== true ||
    commit.author?.type !== "Bot"
  ) {
    throw new Error(
      `Release state commit ${sha} on ${branch} is not a GitHub-verified App commit`
    );
  }
}

// A final release promotes its last RC, so it must ship the Deployment Engine
// commit that RC locked. The RC tag in the Deployment Engine repository can
// move after the RC; its lock, anchored on the RC's Radius tag, cannot.
/** @param {{github: any, context: any, core: any}} options */
async function readReleaseCandidateLock({ github, context, core }) {
  const version = core.getInput("VERSION", { required: true });
  const candidate = core.getInput("RELEASE_CANDIDATE", { required: true });
  if (
    !finalVersionPattern.test(version) ||
    !versionPattern.test(candidate) ||
    !candidate.startsWith(`${version}-rc.`)
  ) {
    throw new Error(`${candidate} is not a release candidate of ${version}`);
  }

  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const branch = lockBranch(candidate);
  const releaseSourceCommit = await tagCommit(github, context, candidate);
  const reference = await getRef(github, owner, repo, branch);
  const content =
    reference &&
    (await getContent(github, owner, repo, reference.object.sha, lockPath));
  if (!content) {
    throw new Error(
      `${candidate} has no Deployment Engine lock on ${branch}; finish that release candidate first`
    );
  }
  const lock = parseLock(content.content);
  const deSourceCommit = lock?.deploymentEngine?.sourceCommit ?? "";
  const expected = canonicalLock(
    {
      version: candidate,
      releaseSourceCommit,
      signedTag: candidate,
      deSourceCommit,
      imageTag: lock?.deploymentEngine?.imageTag
    },
    lock?.deploymentEngine?.digest
  );
  if (
    !commitPattern.test(deSourceCommit) ||
    !digestPattern.test(lock?.deploymentEngine?.digest ?? "") ||
    !equalJSON(lock, expected)
  ) {
    throw new Error(
      `${branch} does not hold the ${candidate} lock for Radius ${releaseSourceCommit}`
    );
  }
  await verifyLockCommit(
    github,
    owner,
    repo,
    branch,
    reference.object.sha,
    releaseSourceCommit
  );

  core.setOutput("de-source-commit", deSourceCommit);
}

/** @param {{github: any, context: any, core: any}} options */
export default async function reconcileReleaseControllerLock({
  github,
  context,
  core
}) {
  const mode = core.getInput("MODE", { required: true });
  if (mode === "release-candidate") {
    await readReleaseCandidateLock({ github, context, core });
    return;
  }
  const inputs = {
    version: core.getInput("VERSION", { required: true }),
    releaseSourceCommit: core.getInput("RELEASE_SOURCE_SHA", {
      required: true
    }),
    signedTag: core.getInput("DE_SIGNED_TAG", { required: true }),
    deSourceCommit: core.getInput("DE_SOURCE_SHA", { required: true }),
    imageTag: core.getInput("DE_IMAGE_TAG", { required: true })
  };
  const requestedDigest = core.getInput("DE_DIGEST");
  if (!new Set(["read", "create"]).has(mode)) {
    throw new Error("MODE must be read, create, or release-candidate");
  }
  if (!versionPattern.test(inputs.version)) {
    throw new Error("VERSION must use vX.Y.Z or vX.Y.Z-rc.N format");
  }
  if (
    !commitPattern.test(inputs.releaseSourceCommit) ||
    !commitPattern.test(inputs.deSourceCommit)
  ) {
    throw new Error("release and Deployment Engine commits must be full SHAs");
  }
  if (inputs.signedTag !== inputs.version) {
    throw new Error("Deployment Engine signed tag must match release version");
  }
  if (mode === "create" && !digestPattern.test(requestedDigest)) {
    throw new Error("DE_DIGEST must be a SHA-256 digest when creating a lock");
  }

  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const branch = lockBranch(inputs.version);
  const path = lockPath;
  let reference = await getRef(github, owner, repo, branch);

  if (!reference && mode === "create") {
    try {
      await github.rest.git.createRef({
        owner,
        repo,
        ref: `refs/heads/${branch}`,
        sha: inputs.releaseSourceCommit
      });
    } catch (error) {
      if (errorStatus(error) !== 422) {
        throw error;
      }
    }
    reference = await getRef(github, owner, repo, branch);
    if (!reference) {
      throw new Error(`Could not create release state branch ${branch}`);
    }
  }

  if (!reference) {
    core.setOutput("exists", "false");
    core.setOutput("branch", branch);
    return;
  }

  let content = await getContent(github, owner, repo, branch, path);
  if (!content && mode === "create") {
    if (reference.object.sha !== inputs.releaseSourceCommit) {
      throw new Error(
        `Release state branch ${branch} is rooted at ${reference.object.sha}, not ${inputs.releaseSourceCommit}`
      );
    }
    const lock = canonicalLock(inputs, requestedDigest);
    try {
      await github.rest.repos.createOrUpdateFileContents({
        owner,
        repo,
        path,
        branch,
        message: `chore(release): lock Deployment Engine for ${inputs.version}`,
        content: Buffer.from(`${JSON.stringify(lock, null, 2)}\n`).toString(
          "base64"
        )
      });
    } catch (error) {
      content = await getContent(github, owner, repo, branch, path);
      if (!content) {
        throw error;
      }
    }
    content = await getContent(github, owner, repo, branch, path);
    reference = await getRef(github, owner, repo, branch);
    if (!reference) {
      throw new Error(`Release state branch ${branch} disappeared`);
    }
  }

  if (!content) {
    if (reference.object.sha !== inputs.releaseSourceCommit) {
      throw new Error(`Release state branch ${branch} has no lock file`);
    }
    core.setOutput("exists", "false");
    core.setOutput("branch", branch);
    return;
  }

  const lock = parseLock(content.content);
  if (!digestPattern.test(lock?.deploymentEngine?.digest ?? "")) {
    throw new Error(
      "Release state contains an invalid Deployment Engine digest"
    );
  }
  const expected = canonicalLock(inputs, lock.deploymentEngine.digest);
  if (!equalJSON(lock, expected)) {
    throw new Error("Release state does not match the approved release plan");
  }
  if (mode === "create" && lock.deploymentEngine.digest !== requestedDigest) {
    throw new Error(
      `Deployment Engine digest is locked at ${lock.deploymentEngine.digest}, not ${requestedDigest}`
    );
  }

  await verifyLockCommit(
    github,
    owner,
    repo,
    branch,
    reference.object.sha,
    inputs.releaseSourceCommit
  );

  core.setOutput("exists", "true");
  core.setOutput("branch", branch);
  core.setOutput("commit", reference.object.sha);
  core.setOutput("digest", lock.deploymentEngine.digest);
}

export { canonicalLock, errorStatus, getRef };
