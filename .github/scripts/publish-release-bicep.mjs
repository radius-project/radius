// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { lstat, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { capture, prepareBundle, tool } from "./bicep-types.mjs";
import { plannedBicepExtensions } from "./release-bicep-extensions.mjs";
import { findReleasePull } from "./resolve-release-controller.mjs";

const repo = { owner: "radius-project", repo: "radius" };
const workflow = ".github/workflows/build-release.yaml";
const generationJob = "Capture versioned Radius Bicep types";
const digestPattern = /^sha256:[a-f0-9]{64}$/;
const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const save = (file, value) =>
  writeFile(file, JSON.stringify(value, null, 2) + "\n");
const load = async (file) => JSON.parse(await readFile(file, "utf8"));
const request = () => ({ signal: AbortSignal.timeout(20_000) });
const snapshotName = (source) => `bicep-types-radius-${source.runId}.tar`;
const positive = (value) => Number.isSafeInteger(value) && value > 0;

export function releaseIdentity(context, env, plan) {
  assert.equal(
    `${context.repo.owner}/${context.repo.repo}`,
    "radius-project/radius"
  );
  assert.ok(
    ["push", "workflow_dispatch"].includes(context.eventName),
    "Untrusted release event"
  );
  assert.equal(
    context.ref,
    `refs/tags/${plan.version}`,
    "Release tag/plan mismatch"
  );
  assert.equal(
    env.GITHUB_WORKFLOW_REF,
    `radius-project/radius/${workflow}@${context.ref}`,
    "Only the tagged release build may call this producer"
  );
  const expected = plannedBicepExtensions(plan, context.sha)[1];
  assert.ok(positive(context.runId));
  assert.match(env.GITHUB_RUN_ATTEMPT, /^[1-9]\d*$/);
  assert.ok(positive(Number(env.GITHUB_RUN_ATTEMPT)));
  return {
    repository: expected.source.repository,
    ref: context.ref,
    commit: context.sha,
    workflow,
    runId: context.runId,
    generationAttempt: Number(env.GITHUB_RUN_ATTEMPT)
  };
}

export async function verifyRun(
  github,
  source,
  attempt = source.generationAttempt
) {
  const { data: run } = await github.rest.actions.getWorkflowRunAttempt({
    ...repo,
    run_id: source.runId,
    attempt_number: attempt,
    request: request()
  });
  assert.ok(
    run.id === source.runId &&
      run.head_sha === source.commit &&
      run.head_branch === source.ref.slice("refs/tags/".length) &&
      run.path === workflow &&
      ["push", "workflow_dispatch"].includes(run.event) &&
      run.run_attempt === attempt &&
      run.repository.full_name === source.repository &&
      run.head_repository.full_name === source.repository,
    "Generation run does not match the approved release tag/source"
  );
}

export function generationSource(source, runId, attempt) {
  assert.equal(
    Boolean(runId),
    Boolean(attempt),
    "Supply both original generation run and attempt"
  );
  if (!runId) return source;
  for (const value of [runId, attempt]) {
    assert.match(value, /^[1-9]\d*$/);
    assert.ok(positive(Number(value)));
  }
  return {
    ...source,
    runId: Number(runId),
    generationAttempt: Number(attempt)
  };
}

export function verifySnapshot(artifact, source) {
  assert.equal(artifact.name, snapshotName(source));
  assert.equal(
    artifact.expired,
    false,
    "Snapshot expired; recover the original evidence, never regenerate a published version"
  );
  assert.ok(positive(artifact.id));
  assert.ok(
    artifact.size_in_bytes > 0 && artifact.size_in_bytes <= 64 * 1024 * 1024
  );
  assert.match(artifact.digest, digestPattern, "Missing snapshot digest");
  assert.ok(
    artifact.workflow_run.id === source.runId &&
      artifact.workflow_run.head_sha === source.commit &&
      artifact.workflow_run.head_branch ===
        source.ref.slice("refs/tags/".length),
    "Snapshot belongs to another release run"
  );
}

export function selectSnapshot(artifacts, source, reuseRequested = false) {
  const matches = artifacts.filter(
    (artifact) => artifact.name === snapshotName(source)
  );
  assert.ok(matches.length <= 1, "Ambiguous release snapshot");
  if (matches.length) {
    verifySnapshot(matches[0], source);
    return matches[0].id;
  }
  assert.ok(
    source.generationAttempt === 1 && !reuseRequested,
    "Retry snapshot missing; recover the original run artifact, never regenerate"
  );
  return "";
}

export async function prepareRelease(
  github,
  archive,
  source,
  expected,
  artifact,
  directory,
  run = tool
) {
  verifySnapshot(artifact, source);
  const stat = await lstat(archive);
  assert.ok(stat.isFile() && stat.size <= 64 * 1024 * 1024);
  assert.equal(
    hash(await readFile(archive)),
    artifact.digest,
    "Snapshot digest mismatch"
  );
  const bundle = await prepareBundle(
    archive,
    source,
    directory,
    { "org.opencontainers.image.version": expected.version },
    run
  );
  await verifyRun(github, bundle.source);
  const jobs = await github.paginate(
    github.rest.actions.listJobsForWorkflowRunAttempt,
    {
      ...repo,
      run_id: source.runId,
      attempt_number: bundle.source.generationAttempt,
      per_page: 100,
      request: request()
    }
  );
  const captures = jobs.filter(
    (job) =>
      job.name === generationJob || job.name.endsWith(` / ${generationJob}`)
  );
  assert.equal(captures.length, 1, "Missing or ambiguous generation job");
  assert.ok(
    captures[0].status === "completed" && captures[0].conclusion === "success",
    "Original snapshot generation did not succeed"
  );
  const created = Date.parse(artifact.created_at);
  assert.ok(
    created >= Date.parse(captures[0].started_at) &&
      created <= Date.parse(captures[0].completed_at),
    "Snapshot was not uploaded by the recorded generation attempt"
  );
  const record = {
    ...expected,
    digest: bundle.manifestDigest,
    generation: {
      workflow,
      runId: bundle.source.runId,
      runAttempt: bundle.source.generationAttempt,
      artifactId: artifact.id,
      artifactDigest: artifact.digest
    }
  };
  const receipt = { status: "prepared", record };
  await save(join(directory, "receipt.json"), receipt);
  return receipt;
}

// ORAS's exact resolver-not-found diagnostic is the only absence result.
// In particular, authorization, transport and arbitrary HTTP errors propagate.
export async function existingManifest(reference, run = tool) {
  try {
    return await run(["oras", "manifest", "fetch", reference]);
  } catch (error) {
    if (
      error.code === 1 &&
      error.stderr?.trim() ===
        `Error response from registry: failed to fetch the content of "${reference}": ${reference}: not found`
    )
      return null;
    throw error;
  }
}

export async function publishRelease(github, receipt, directory, run = tool) {
  const { record } = receipt;
  try {
    const existing = await existingManifest(record.reference, run);
    if (existing !== null) {
      assert.equal(
        hash(existing),
        record.digest,
        "Full-version Bicep tag conflicts; never overwrite it"
      );
      const manifest = JSON.parse(existing);
      for (const [key, value] of Object.entries({
        source: `https://github.com/${record.source.repository}`,
        revision: record.source.commit,
        version: record.version
      })) {
        assert.equal(
          manifest.annotations?.[`org.opencontainers.image.${key}`],
          value
        );
      }
    } else {
      receipt.status = "partial";
      await save(join(directory, "receipt.json"), receipt);
      await run([
        "oras",
        "cp",
        "--from-oci-layout",
        `${join(directory, "layout")}@${record.digest}`,
        record.reference
      ]);
    }
    assert.equal(
      hash(await run(["oras", "manifest", "fetch", record.reference])),
      record.digest,
      "Published full-version digest mismatch"
    );
    receipt.status = "uploaded";
    receipt.uploadedDigest = record.digest;
    const { data: pkg } = await github.rest.packages.getPackageForOrganization({
      package_type: "container",
      package_name: "bicep-types-radius",
      org: repo.owner,
      request: request()
    });
    receipt.visibility = pkg.visibility;
    assert.equal(
      pkg.visibility,
      "public",
      "Full version uploaded; a package admin must make bicep-types-radius Public, then retry with the original snapshot and verify anonymous restore"
    );
    receipt.status = "published";
    await save(join(directory, "radius-bicep-extension.json"), record);
    return record;
  } finally {
    await save(join(directory, "receipt.json"), receipt);
  }
}

export async function approvedRelease(
  { github, context },
  directory,
  env = process.env,
  run = tool
) {
  assert.match(
    context.ref,
    /^refs\/tags\/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-rc\.[1-9]\d*)?$/
  );
  const version = context.ref.slice("refs/tags/".length);
  const planPath = `.github/release-plans/${version}.yaml`;
  const plan = JSON.parse(await run(["yq", "-o=json", ".", planPath]));
  const source = releaseIdentity(context, env, plan);
  assert.equal((await run(["git", "rev-parse", "HEAD"])).trim(), source.commit);
  await verifyRun(github, source);
  const { data: tag } = await github.rest.repos.getCommit({
    ...repo,
    ref: source.ref,
    request: request()
  });
  assert.equal(
    tag.sha,
    source.commit,
    "Release tag no longer names the approved source"
  );
  const pull = await findReleasePull(github, repo.owner, repo.repo, version);
  await run([
    "git",
    "fetch",
    "--no-tags",
    "origin",
    pull.merge_commit_sha,
    "+refs/heads/release/*:refs/remotes/origin/release/*"
  ]);
  const approvedPlan = join(directory, "approved-plan.yaml");
  await writeFile(
    approvedPlan,
    await run(["git", "show", `${pull.merge_commit_sha}:${planPath}`])
  );
  await run([
    "bash",
    ".github/scripts/validate-release-controller-plan.sh",
    "--plan-file",
    approvedPlan,
    "--version",
    version,
    "--source-pr-commit",
    pull.merge_commit_sha,
    "--release-commit",
    source.commit,
    "--trigger",
    "dispatch",
    "--output-dir",
    join(directory, "approved")
  ]);
  assert.equal(
    (await readFile(join(directory, "approved/ready.txt"), "utf8")).trim(),
    "true"
  );
  return { source, expected: plannedBicepExtensions(plan, source.commit)[1] };
}

/** @param {import('@actions/github-script').AsyncFunctionArguments} args */
export default async function script({ github, context, core }) {
  try {
    const directory = core.getInput("directory", { required: true });
    await mkdir(directory, { recursive: true });
    const { source, expected } = await approvedRelease(
      { github, context },
      directory
    );
    const runId = core.getInput("generation_run_id");
    const attempt = core.getInput("generation_run_attempt");
    const generation = generationSource(source, runId, attempt);
    await verifyRun(github, generation);
    const operation = core.getInput("operation", { required: true });
    if (operation === "select") {
      const artifacts = await github.paginate(
        github.rest.actions.listWorkflowRunArtifacts,
        {
          ...repo,
          run_id: generation.runId,
          per_page: 100,
          request: request()
        }
      );
      const id = selectSnapshot(artifacts, generation, Boolean(runId));
      core.setOutput("artifact_id", id);
      core.setOutput("generation_run_id", generation.runId);
      core.setOutput("reuse", id !== "");
      core.setOutput("version", expected.version);
    } else if (operation === "capture") {
      assert.equal(runId, "", "An explicit generation run may only be reused");
      assert.equal(source.generationAttempt, 1, "Never regenerate on retry");
      await capture(
        source,
        directory,
        core.getInput("registry", { required: true })
      );
    } else if (operation === "prepare") {
      const id = Number(core.getInput("artifact_id", { required: true }));
      assert.ok(positive(id));
      const { data: artifact } = await github.rest.actions.getArtifact({
        ...repo,
        artifact_id: id,
        request: request()
      });
      const files = await readdir(join(directory, "download"));
      assert.equal(
        files.length,
        1,
        "Expected exactly the selected raw snapshot"
      );
      const receipt = await prepareRelease(
        github,
        join(directory, "download", files[0]),
        generation,
        expected,
        artifact,
        directory
      );
      if (attempt)
        assert.equal(
          receipt.record.generation.runAttempt,
          Number(attempt),
          "Explicit generation attempt mismatch"
        );
    } else if (operation === "publish") {
      const receipt = await load(join(directory, "receipt.json"));
      for (const [key, value] of Object.entries(expected))
        assert.deepEqual(
          receipt.record[key],
          value,
          "Prepared release identity mismatch"
        );
      const record = await publishRelease(github, receipt, directory);
      core.setOutput("record", JSON.stringify(record));
      core.setOutput("manifest_digest", record.digest);
    } else {
      throw new Error(`Unknown release Bicep operation: ${operation}`);
    }
  } catch (error) {
    core.setFailed(error instanceof Error ? error.message : String(error));
  }
}
