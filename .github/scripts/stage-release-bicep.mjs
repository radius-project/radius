// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tool } from "./bicep-types.mjs";
import assets, { readBicepExtensionLock } from "./release-assets.mjs";
import { observeExtension, readAwsEvidence } from "./collect-release-bicep.mjs";
import {
  bicepExtensionLockName,
  plannedBicepExtensions,
  usesGhcrBicepExtensions,
  validateBicepExtensionLock,
  validateBicepExtensionRecord
} from "./release-bicep-extensions.mjs";
import {
  approvedRelease,
  existingManifest,
  generationSource,
  verifyGeneration
} from "./publish-release-bicep.mjs";

const repo = { owner: "radius-project", repo: "radius" };

export async function verifyPair(plan, sourceSha, lock, run = tool) {
  validateBicepExtensionLock(plan, sourceSha, lock);
  for (const artifact of lock.artifacts) await observeExtension(artifact, run);
}

export async function recoveryInputs(
  github,
  context,
  source,
  record,
  runId,
  attempt,
  run = tool
) {
  const selected = generationSource(source, runId, attempt);
  if (runId) return { runId, attempt };
  const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
    ...repo,
    workflow_id: "build-release.yaml",
    head_sha: source.commit,
    per_page: 100
  });
  assert.ok(
    !runs.some(
      (entry) =>
        entry.id !== context.runId &&
        entry.head_sha === source.commit &&
        entry.head_branch === context.ref.slice("refs/tags/".length) &&
        ["push", "workflow_dispatch"].includes(entry.event)
    ),
    "A prior release run exists; resume it or supply its original generation run and attempt"
  );
  const snapshots = await github.paginate(
    github.rest.actions.listWorkflowRunArtifacts,
    {
      ...repo,
      run_id: selected.runId,
      per_page: 100
    }
  );
  if (
    !snapshots.some(
      (artifact) => artifact.name === `bicep-types-radius-${selected.runId}.tar`
    )
  ) {
    assert.equal(
      await existingManifest(record.reference, run),
      null,
      "Staged Radius version lacks its original snapshot; never regenerate it"
    );
  }
  return { runId: "", attempt: "" };
}

export default async function stage({ github, context, core }) {
  const directory = core.getInput("directory", { required: true });
  await mkdir(directory, { recursive: true });
  const tag = context.ref.slice("refs/tags/".length);
  const plan = JSON.parse(
    await tool(["yq", "-o=json", ".", `.github/release-plans/${tag}.yaml`])
  );
  const ghcr = usesGhcrBicepExtensions(plan.expectedOutputs);
  core.setOutput("ghcr", String(ghcr));
  if (!ghcr) return;
  const operation = core.getInput("operation", { required: true });
  assert.ok(
    ["preflight", "lock"].includes(operation),
    "Unknown Bicep staging operation"
  );
  const { source, expected } = await approvedRelease(
    { github, context },
    directory
  );
  const aws = await readAwsEvidence(github, plan, source.commit);
  assert.ok(
    aws,
    "Trusted controller AWS evidence is missing; resume the controller before the build"
  );
  await observeExtension(aws.artifact);
  const lock = await readBicepExtensionLock(github, plan.version);
  if (lock) {
    await verifyPair(plan, source.commit, lock);
    assert.deepEqual(
      lock.artifacts[0],
      aws.artifact,
      "Paired AWS lock differs from controller evidence"
    );
    const runId = core.getInput("generation_run_id");
    const attempt = core.getInput("generation_run_attempt");
    const selected = generationSource(source, runId, attempt);
    if (runId) {
      assert.equal(selected.runId, lock.artifacts[1].generation.runId);
      assert.equal(
        selected.generationAttempt,
        lock.artifacts[1].generation.runAttempt
      );
    }
    const record = core.getInput("radius_record");
    if (record)
      assert.deepEqual(
        JSON.parse(record),
        lock.artifacts[1],
        "New Radius evidence differs from the immutable paired lock"
      );
    core.setOutput("locked", "true");
    await writeFile(
      join(directory, bicepExtensionLockName),
      JSON.stringify(lock, null, 2) + "\n"
    );
    return;
  }
  core.setOutput("locked", "false");
  if (operation === "preflight") {
    const recovery = await recoveryInputs(
      github,
      context,
      source,
      expected,
      core.getInput("generation_run_id"),
      core.getInput("generation_run_attempt")
    );
    core.setOutput("generation_run_id", recovery.runId);
    core.setOutput("generation_run_attempt", recovery.attempt);
    return;
  }
  assert.equal(operation, "lock");
  const record = JSON.parse(core.getInput("radius_record", { required: true }));
  validateBicepExtensionRecord(
    plannedBicepExtensions(plan, source.commit)[1],
    record
  );
  assert.equal(
    record.generation.workflow,
    ".github/workflows/build-release.yaml"
  );
  const { data: artifact } = await github.rest.actions.getArtifact({
    ...repo,
    artifact_id: record.generation.artifactId
  });
  assert.equal(artifact.digest, record.generation.artifactDigest);
  await verifyGeneration(
    github,
    {
      ...source,
      runId: record.generation.runId,
      generationAttempt: record.generation.runAttempt
    },
    artifact
  );
  const pair = {
    schemaVersion: 1,
    version: plan.version,
    releaseSourceCommit: source.commit,
    artifacts: [aws.artifact, record]
  };
  await verifyPair(plan, source.commit, pair);
  const file = join(directory, bicepExtensionLockName);
  await writeFile(file, JSON.stringify(pair, null, 2) + "\n");
  const inputs = {
    OWNER: repo.owner,
    REPO: repo.repo,
    TAG: plan.version,
    MODE: "upload",
    FILE: file,
    IMMUTABLE: "true"
  };
  await assets({
    github,
    core: {
      ...core,
      getInput: (name, options) => inputs[name] ?? core.getInput(name, options)
    }
  });
}
