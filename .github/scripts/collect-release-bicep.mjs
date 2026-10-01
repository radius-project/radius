// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { lstat, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tool } from "./bicep-types.mjs";
import {
  plannedBicepExtensions,
  usesGhcrBicepExtensions,
  validateBicepExtensionRecord,
  verifyBicepExtensionOutput
} from "./release-bicep-extensions.mjs";

// Proposed A1 protocol. Its implementation and deployment are a merge gate.
export const awsWorkflow = ".github/workflows/publish-release-bicep.yaml";
export const awsCaptureJob = "Capture versioned AWS Bicep types";
export const evidencePath = ".github/release-state/bicep-extensions.json";
const aws = { owner: "radius-project", repo: "bicep-types-aws" };
const radius = { owner: "radius-project", repo: "radius" };
const digestPattern = /^sha256:[a-f0-9]{64}$/;
const positive = (n) => Number.isSafeInteger(n) && n > 0;
const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const save = (file, value) =>
  writeFile(file, JSON.stringify(value, null, 2) + "\n");
const load = async (file) => JSON.parse(await readFile(file, "utf8"));
const request = () => ({ signal: AbortSignal.timeout(20_000) });

export async function observeExtension(record, run = tool) {
  const descriptor = JSON.parse(
    await run(["oras", "manifest", "fetch", "--descriptor", record.reference])
  );
  assert.equal(
    descriptor.digest,
    record.digest,
    "Bicep full-version tag changed"
  );
  const bytes = await run([
    "oras",
    "manifest",
    "fetch",
    `${record.reference.slice(0, record.reference.lastIndexOf(":"))}@${record.digest}`
  ]);
  assert.equal(hash(bytes), record.digest, "Bicep manifest bytes changed");
  const observed = {
    name: record.name,
    reference: record.reference,
    descriptor,
    manifest: JSON.parse(bytes)
  };
  verifyBicepExtensionOutput(record, observed);
  return observed;
}

function validateRun(run, plan, expected, attempt) {
  assert.ok(positive(run.id) && positive(attempt));
  assert.equal(run.run_attempt, attempt);
  assert.equal(run.repository.full_name, expected.source.repository);
  assert.equal(run.head_repository.full_name, expected.source.repository);
  assert.equal(run.head_sha, expected.source.commit);
  assert.equal(run.head_branch, plan.version);
  assert.equal(run.path, awsWorkflow);
  assert.ok(["push", "workflow_dispatch"].includes(run.event));
}

function validateArtifact(artifact, run, name, maxSize) {
  assert.ok(positive(artifact.id));
  assert.equal(artifact.name, name);
  assert.equal(
    artifact.expired,
    false,
    "AWS evidence expired; recover original evidence"
  );
  assert.ok(artifact.size_in_bytes > 0 && artifact.size_in_bytes <= maxSize);
  assert.match(artifact.digest, digestPattern);
  assert.equal(artifact.workflow_run.id, run.id);
  assert.equal(artifact.workflow_run.head_sha, run.head_sha);
  assert.equal(artifact.workflow_run.head_branch, run.head_branch);
}

export async function selectAwsPublication(
  github,
  plan,
  sourceSha,
  {
    now = Date.now,
    sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
  } = {}
) {
  const expected = plannedBicepExtensions(plan, sourceSha)[0];
  const { data: tag } = await github.rest.repos.getCommit({
    ...aws,
    ref: `refs/tags/${plan.version}`,
    request: request()
  });
  assert.equal(
    tag.sha,
    expected.source.commit,
    "AWS tag differs from frozen source"
  );
  const deadline = now() + 600_000;
  while (true) {
    const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
      ...aws,
      workflow_id: awsWorkflow.split("/").at(-1),
      head_sha: expected.source.commit,
      per_page: 100,
      request: request()
    });
    const matching = runs.filter(
      (run) =>
        run.head_branch === plan.version &&
        ["push", "workflow_dispatch"].includes(run.event)
    );
    const successful = matching.filter(
      (run) => run.status === "completed" && run.conclusion === "success"
    );
    assert.ok(
      successful.length <= 1,
      "Ambiguous AWS publishing runs; reconcile original evidence before resuming"
    );
    if (successful.length) {
      const run = successful[0];
      validateRun(run, plan, expected, run.run_attempt);
      const artifacts = await github.paginate(
        github.rest.actions.listWorkflowRunArtifacts,
        {
          ...aws,
          run_id: run.id,
          per_page: 100,
          request: request()
        }
      );
      const name = `aws-bicep-extension-${run.id}-${run.run_attempt}`;
      const matches = artifacts.filter((artifact) => artifact.name === name);
      assert.equal(
        matches.length,
        1,
        "Missing or ambiguous successful AWS record"
      );
      validateArtifact(matches[0], run, name, 1024 * 1024);
      const created = Date.parse(matches[0].created_at);
      const jobs = await github.paginate(
        github.rest.actions.listJobsForWorkflowRunAttempt,
        {
          ...aws,
          run_id: run.id,
          attempt_number: run.run_attempt,
          per_page: 100,
          request: request()
        }
      );
      assert.ok(
        jobs.some(
          (job) =>
            job.conclusion === "success" &&
            created >= Date.parse(job.started_at) &&
            created <= Date.parse(job.completed_at)
        ),
        "AWS record was not uploaded by the successful publishing attempt"
      );
      return {
        workflow: awsWorkflow,
        runId: run.id,
        runAttempt: run.run_attempt,
        artifactId: matches[0].id,
        artifactDigest: matches[0].digest
      };
    }
    if (matching.length && matching.every((run) => run.status === "completed"))
      throw new Error(
        "AWS publishing failed; resume the source-owned producer"
      );
    assert.ok(
      now() < deadline,
      "AWS publication evidence not ready; resume the controller after A1 succeeds"
    );
    await sleep(10_000);
  }
}

export async function authenticateAwsGeneration(
  github,
  plan,
  sourceSha,
  record
) {
  const expected = plannedBicepExtensions(plan, sourceSha)[0];
  validateBicepExtensionRecord(expected, record);
  const generation = record.generation;
  assert.equal(generation.workflow, awsWorkflow);
  const { data: run } = await github.rest.actions.getWorkflowRunAttempt({
    ...aws,
    run_id: generation.runId,
    attempt_number: generation.runAttempt,
    request: request()
  });
  assert.equal(run.id, generation.runId, "AWS generation run ID mismatch");
  validateRun(run, plan, expected, generation.runAttempt);
  const { data: artifact } = await github.rest.actions.getArtifact({
    ...aws,
    artifact_id: generation.artifactId,
    request: request()
  });
  assert.equal(artifact.id, generation.artifactId, "AWS snapshot ID mismatch");
  validateArtifact(
    artifact,
    run,
    `bicep-types-aws-${run.id}.tar`,
    64 * 1024 * 1024
  );
  assert.equal(artifact.digest, generation.artifactDigest);
  const jobs = await github.paginate(
    github.rest.actions.listJobsForWorkflowRunAttempt,
    {
      ...aws,
      run_id: generation.runId,
      attempt_number: generation.runAttempt,
      per_page: 100,
      request: request()
    }
  );
  const captures = jobs.filter(
    (job) =>
      job.name === awsCaptureJob || job.name.endsWith(` / ${awsCaptureJob}`)
  );
  assert.equal(captures.length, 1, "Missing or ambiguous AWS capture job");
  assert.equal(captures[0].status, "completed");
  assert.equal(captures[0].conclusion, "success");
  const created = Date.parse(artifact.created_at);
  assert.ok(
    created >= Date.parse(captures[0].started_at) &&
      created <= Date.parse(captures[0].completed_at),
    "AWS snapshot capture attempt mismatch"
  );
}

export function validateAwsEvidence(plan, sourceSha, evidence) {
  const expected = plannedBicepExtensions(plan, sourceSha)[0];
  assert.deepEqual(
    Object.keys(evidence).sort(),
    [
      "schemaVersion",
      "version",
      "releaseSourceCommit",
      "artifact",
      "publishing"
    ].sort()
  );
  assert.equal(evidence.schemaVersion, 1);
  assert.equal(evidence.version, plan.version);
  assert.equal(evidence.releaseSourceCommit, sourceSha);
  validateBicepExtensionRecord(expected, evidence.artifact);
  assert.equal(evidence.artifact.generation.workflow, awsWorkflow);
  assert.equal(evidence.publishing.workflow, awsWorkflow);
  for (const key of ["runId", "runAttempt", "artifactId"])
    assert.ok(positive(evidence.publishing[key]));
  assert.match(evidence.publishing.artifactDigest, digestPattern);
}

export async function readAwsEvidence(github, plan, sourceSha) {
  plannedBicepExtensions(plan, sourceSha);
  const branch = `automation/bicep-release-state-${plan.version.slice(1)}`;
  let ref;
  try {
    ({ data: ref } = await github.rest.git.getRef({
      ...radius,
      ref: `heads/${branch}`,
      request: request()
    }));
  } catch (error) {
    if (error.status === 404) return undefined;
    throw error;
  }
  const { data: commit } = await github.rest.repos.getCommit({
    ...radius,
    ref: ref.object.sha,
    request: request()
  });
  assert.deepEqual(
    commit.parents.map((parent) => parent.sha),
    [sourceSha],
    "Bicep evidence ref has unexpected history"
  );
  assert.equal(
    commit.files?.length,
    1,
    "Bicep evidence commit changed unexpected files"
  );
  assert.equal(commit.files[0].filename, evidencePath);
  assert.equal(commit.files[0].status, "added");
  const { data: file } = await github.rest.repos.getContent({
    ...radius,
    path: evidencePath,
    ref: ref.object.sha,
    request: request()
  });
  assert.equal(file.type, "file");
  assert.equal(file.encoding, "base64");
  const evidence = JSON.parse(
    Buffer.from(file.content, "base64").toString("utf8")
  );
  validateAwsEvidence(plan, sourceSha, evidence);
  return evidence;
}

export async function persistAwsEvidence(github, plan, sourceSha, evidence) {
  validateAwsEvidence(plan, sourceSha, evidence);
  const existing = await readAwsEvidence(github, plan, sourceSha);
  if (existing) {
    assert.deepEqual(existing, evidence, "Immutable AWS evidence conflicts");
    return;
  }
  const { data: source } = await github.rest.git.getCommit({
    ...radius,
    commit_sha: sourceSha,
    request: request()
  });
  const { data: tree } = await github.rest.git.createTree({
    ...radius,
    base_tree: source.tree.sha,
    tree: [
      {
        path: evidencePath,
        mode: "100644",
        type: "blob",
        content: JSON.stringify(evidence, null, 2) + "\n"
      }
    ],
    request: request()
  });
  const { data: commit } = await github.rest.git.createCommit({
    ...radius,
    message: `chore(release): lock AWS Bicep evidence for ${plan.version}`,
    tree: tree.sha,
    parents: [sourceSha],
    request: request()
  });
  try {
    await github.rest.git.createRef({
      ...radius,
      ref: `refs/heads/automation/bicep-release-state-${plan.version.slice(1)}`,
      sha: commit.sha,
      request: request()
    });
  } catch (error) {
    const recovered = await readAwsEvidence(github, plan, sourceSha);
    if (!recovered) throw error;
    assert.deepEqual(recovered, evidence, "Concurrent AWS evidence conflicts");
  }
  assert.deepEqual(
    await readAwsEvidence(github, plan, sourceSha),
    evidence,
    "AWS evidence persistence readback failed"
  );
}

export default async function collect({ github, core }) {
  const directory = core.getInput("directory", { required: true });
  const plan = await load(core.getInput("plan_file", { required: true }));
  const sourceSha = core.getInput("source_sha", { required: true });
  const ghcr = usesGhcrBicepExtensions(plan.expectedOutputs);
  core.setOutput("ghcr", String(ghcr));
  if (!ghcr) return;
  plannedBicepExtensions(plan, sourceSha);
  await mkdir(directory, { recursive: true });
  const operation = core.getInput("operation", { required: true });
  if (operation === "read") {
    const evidence = await readAwsEvidence(github, plan, sourceSha);
    core.setOutput("exists", String(Boolean(evidence)));
    if (evidence) {
      await observeExtension(evidence.artifact);
      await save(join(directory, "aws-evidence.json"), evidence);
    }
  } else if (operation === "select") {
    const publishing = await selectAwsPublication(github, plan, sourceSha);
    await save(join(directory, "publishing.json"), publishing);
    core.setOutput("run_id", publishing.runId);
    core.setOutput("artifact_id", publishing.artifactId);
  } else if (operation === "authenticate") {
    assert.deepEqual(
      await readdir(join(directory, "record")),
      ["aws-bicep-extension.json"],
      "AWS record artifact contains unexpected files"
    );
    const file = join(directory, "record/aws-bicep-extension.json");
    const stat = await lstat(file);
    assert.ok(stat.isFile() && stat.size <= 1024 * 1024);
    const artifact = await load(file);
    await authenticateAwsGeneration(github, plan, sourceSha, artifact);
    const evidence = {
      schemaVersion: 1,
      version: plan.version,
      releaseSourceCommit: sourceSha,
      artifact,
      publishing: await load(join(directory, "publishing.json"))
    };
    validateAwsEvidence(plan, sourceSha, evidence);
    await save(join(directory, "aws-evidence.json"), evidence);
    core.setOutput("generation_run_id", artifact.generation.runId);
    core.setOutput("generation_artifact_id", artifact.generation.artifactId);
  } else if (operation === "persist") {
    const evidence = await load(join(directory, "aws-evidence.json"));
    validateAwsEvidence(plan, sourceSha, evidence);
    const files = await readdir(join(directory, "snapshot"));
    assert.equal(files.length, 1, "Expected one raw AWS snapshot");
    const file = join(directory, "snapshot", files[0]);
    const stat = await lstat(file);
    assert.ok(stat.isFile() && stat.size <= 64 * 1024 * 1024);
    assert.equal(
      hash(await readFile(file)),
      evidence.artifact.generation.artifactDigest,
      "Downloaded AWS snapshot digest differs from original generation"
    );
    await observeExtension(evidence.artifact);
    await persistAwsEvidence(github, plan, sourceSha, evidence);
  } else {
    throw new Error(`Unknown Bicep evidence operation: ${operation}`);
  }
}
