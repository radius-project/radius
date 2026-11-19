// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { tool } from "./bicep-types.mjs";
import { existingManifest } from "./publish-release-bicep.mjs";
import { verifyBicepExtensionOutput } from "./release-bicep-extensions.mjs";
import { verifyPair } from "./stage-release-bicep.mjs";
import { validatePublicationManifest } from "./publish-draft-release.mjs";
import { verifyBicepExtensionOutputs } from "./release-bicep-extensions.mjs";

const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const stable = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const newer = (a, b) => {
  const left = a.split(".").map(BigInt);
  const right = b.split(".").map(BigInt);
  for (let i = 0; i < 3; ++i) {
    if (left[i] !== right[i]) return left[i] > right[i];
  }
  return false;
};

export function promotionTargets(plan, lock, channel, latest) {
  assert.ok(["true", "false"].includes(channel));
  assert.ok(["true", "false"].includes(latest));
  const rc = plan.releaseType === "rc";
  if (rc)
    assert.ok(
      channel === "false" && latest === "false",
      "RC must not promote stable aliases"
    );
  const result = [];
  for (const record of lock.artifacts) {
    const repository = record.reference.slice(
      0,
      record.reference.lastIndexOf(":")
    );
    const add = (reference, group) =>
      result.push({
        record,
        source: `${repository}@${record.digest}`,
        reference,
        group
      });
    if (!rc) {
      assert.match(record.version, stable);
      assert.equal(
        plan.channel,
        record.version.split(".").slice(0, 2).join(".")
      );
      if (channel === "true") {
        add(`${repository}:${plan.channel}`, "channel");
      }
      if (latest === "true") add(`${repository}:latest`, "latest");
    }
  }
  return result;
}

export function inspectDestination(target, bytes) {
  if (bytes === null) return { status: "absent" };
  const digest = hash(bytes);
  if (digest === target.record.digest) return { status: "matching", digest };
  const manifest = JSON.parse(bytes);
  const version = manifest.annotations?.["org.opencontainers.image.version"];
  assert.match(
    version ?? "",
    stable,
    "Alias has unknown or prerelease provenance"
  );
  if (target.group === "channel")
    assert.equal(
      version.split(".").slice(0, 2).join("."),
      target.record.version.split(".").slice(0, 2).join("."),
      "Channel alias names another channel"
    );
  const commit = manifest.annotations?.["org.opencontainers.image.revision"];
  assert.match(commit ?? "", /^(?!0{40}$)[a-f0-9]{40}$/);
  verifyBicepExtensionOutput(
    {
      ...target.record,
      reference: target.reference,
      digest,
      version,
      source: { ...target.record.source, commit }
    },
    { reference: target.reference, descriptor: { digest }, manifest }
  );
  assert.notEqual(
    version,
    target.record.version,
    "Same-version alias differs from the immutable lock"
  );
  return {
    status: newer(version, target.record.version) ? "newer" : "older",
    digest,
    version
  };
}

export async function promoteExtensions(
  plan,
  sourceSha,
  lock,
  channel,
  latest,
  receiptFile,
  run = tool
) {
  const receipt = {
    schemaVersion: 1,
    version: plan.version,
    releaseSourceCommit: sourceSha,
    status: "verifying",
    destinations: []
  };
  await mkdir(dirname(receiptFile), { recursive: true });
  const save = () =>
    writeFile(receiptFile, JSON.stringify(receipt, null, 2) + "\n");
  try {
    await verifyPair(plan, sourceSha, lock, run);
    const targets = promotionTargets(plan, lock, channel, latest);
    const superseded = new Set();
    for (const target of targets) {
      const observed = inspectDestination(
        target,
        await existingManifest(target.reference, run)
      );
      if (observed.status === "newer") superseded.add(target.group);
      receipt.destinations.push({
        reference: target.reference,
        digest: target.record.digest,
        group: target.group,
        observed,
        status: "pending"
      });
    }
    receipt.status = "partial";
    await save();
    for (const [index, target] of targets.entries()) {
      const entry = receipt.destinations[index];
      if (superseded.has(target.group)) {
        entry.status = "superseded";
      } else {
        if (entry.observed.status !== "matching") {
          try {
            await run(["oras", "cp", target.source, target.reference]);
          } catch (error) {
            let actual;
            try {
              actual = hash(
                await run(["oras", "manifest", "fetch", target.reference])
              );
            } catch (readbackError) {
              throw new AggregateError(
                [error, readbackError],
                `Bicep copy to ${target.reference} failed: ${error.message}; readback failed: ${readbackError.message}`
              );
            }
            if (actual !== target.record.digest) throw error;
            entry.recoveredUncertainWrite = true;
          }
        }
        assert.equal(
          hash(await run(["oras", "manifest", "fetch", target.reference])),
          target.record.digest,
          `Bicep destination ${target.reference} failed readback`
        );
        entry.status = "verified";
      }
      await save();
    }
    receipt.status = "complete";
    return receipt;
  } catch (error) {
    receipt.error = error.message;
    throw error;
  } finally {
    await save();
  }
}

export async function finalizeExtensions({
  plan,
  sourceSha,
  lock,
  manifest,
  channel,
  latest,
  directory,
  run = tool,
  now = Date.now,
  jobDeadline
}) {
  const deadline = Math.min(
    now() + 24 * 60_000,
    jobDeadline === undefined ? Infinity : jobDeadline - 180_000
  );
  const remaining = () => {
    const milliseconds = deadline - now();
    assert.ok(
      milliseconds > 30_000,
      "Bicep finalization budget exhausted; resume from retained receipts"
    );
    return milliseconds - 30_000;
  };
  const execute = (args, options = {}) =>
    run(args, {
      ...options,
      timeout: Math.min(options.timeout ?? 30_000, remaining())
    });
  const receipt = {
    schemaVersion: 1,
    version: plan.version,
    releaseSourceCommit: sourceSha,
    status: "verifying"
  };
  await mkdir(directory, { recursive: true });
  const save = () =>
    writeFile(
      join(directory, "receipt.json"),
      JSON.stringify(receipt, null, 2) + "\n"
    );
  try {
    validatePublicationManifest(manifest, plan.version, sourceSha);
    assert.equal(manifest.releaseType, plan.releaseType);
    assert.equal(manifest.channel, plan.channel);
    assert.deepEqual(
      manifest.observed?.downstream?.bicepExtensionLock,
      lock,
      "Approved manifest and paired lock differ"
    );
    verifyBicepExtensionOutputs(
      plan,
      sourceSha,
      lock,
      manifest.observed.downstream.ociArtifacts
    );
    promotionTargets(plan, lock, channel, latest);
    await verifyPair(plan, sourceSha, lock, (args) => execute(args));
    receipt.status = "partial";
    await save();
    receipt.ghcr = await promoteExtensions(
      plan,
      sourceSha,
      lock,
      channel,
      latest,
      join(directory, "ghcr-receipt.json"),
      execute
    );
    receipt.status = "complete";
    return receipt;
  } catch (error) {
    receipt.error = error.message;
    throw error;
  } finally {
    await save();
  }
}

function readJobDeadline(core) {
  const deadline = Number(core.getInput("JOB_DEADLINE", { required: true }));
  assert.ok(
    Number.isSafeInteger(deadline) && deadline > 0,
    "Invalid Bicep job deadline"
  );
  return deadline;
}

export default async function finalize({ core, context }) {
  const load = async (name) =>
    JSON.parse(await readFile(core.getInput(name, { required: true }), "utf8"));
  const plan = await load("PLAN_FILE");
  assert.equal(context.repo.owner, "radius-project");
  assert.equal(context.repo.repo, "radius");
  assert.equal(context.ref, `refs/tags/${plan.version}`);
  await finalizeExtensions({
    plan,
    sourceSha: context.sha,
    lock: await load("LOCK_FILE"),
    manifest: await load("MANIFEST_FILE"),
    channel: core.getInput("PROMOTE_CHANNEL", { required: true }),
    latest: core.getInput("PROMOTE_LATEST", { required: true }),
    directory: core.getInput("DIRECTORY", { required: true }),
    jobDeadline: readJobDeadline(core)
  });
}
