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
import monitorRemoteWorkflow from "./monitor-remote-workflow.mjs";
import {
  promotionRequests,
  dispatchPromotion
} from "./promote-bicep-types.mjs";
import { validatePublicationManifest } from "./publish-draft-release.mjs";
import {
  validateBicepExtensionLock,
  verifyBicepExtensionOutputs
} from "./release-bicep-extensions.mjs";

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

export function releaseCopyRequests(plan, sourceSha, lock) {
  validateBicepExtensionLock(plan, sourceSha, lock);
  return lock.artifacts.map((record) => {
    const target = record.name === "aws-bicep-types" ? "aws" : "radius";
    const identifier = `copy-${target}-${record.version}-${record.source.commit}-${record.digest.slice(7)}`;
    assert.ok(
      identifier.length <= 200,
      "Copy identifier exceeds monitor limit"
    );
    return {
      mode: "copy",
      source_repository: record.source.repository,
      source_ref: `refs/tags/${plan.version}`,
      source_sha: record.source.commit,
      registry_target: target,
      rel_channel: plan.releaseType === "rc" ? record.version : plan.channel,
      digest: record.digest,
      release_identifier: identifier
    };
  });
}

export async function finalizeExtensions({
  github,
  core,
  plan,
  sourceSha,
  lock,
  manifest,
  channel,
  latest,
  directory,
  monitor = monitorRemoteWorkflow,
  run = tool,
  now = Date.now
}) {
  const deadline = now() + 24 * 60_000;
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
  const reconcile = (args) =>
    monitor({
      ...args,
      core: {
        ...args.core,
        getInput: (name, options) =>
          name === "MAX_WAIT_SECONDS" ?
            String(Math.min(300, Math.floor(remaining() / 1000)))
          : args.core.getInput(name, options)
      }
    });
  const receipt = {
    schemaVersion: 1,
    version: plan.version,
    releaseSourceCommit: sourceSha,
    status: "verifying",
    copies: [],
    promotions: []
  };
  await mkdir(directory, { recursive: true });
  const save = () =>
    writeFile(
      join(directory, "receipt.json"),
      JSON.stringify(receipt, null, 2) + "\n"
    );
  const registryConfig = join(directory, "anonymous-registry.json");
  const readback = async (reference, digest) => {
    assert.equal(
      hash(
        await execute(
          [
            "oras",
            "manifest",
            "fetch",
            "--registry-config",
            registryConfig,
            reference
          ],
          { timeout: 30_000 }
        )
      ),
      digest,
      `Private Bicep copy digest mismatch: ${reference}`
    );
  };
  const remoteCore = (entry) => ({
    ...core,
    setOutput: (name, value) => {
      entry[name] = value;
    },
    setFailed: (message) => {
      throw new Error(message);
    }
  });
  const requireSuccess = (entry) => {
    assert.equal(
      entry.conclusion,
      "success",
      "Private publication did not succeed; inspect retained run identity"
    );
    assert.match(
      String(entry.run_id ?? ""),
      /^[1-9]\d*$/,
      "Missing private run identity"
    );
    assert.equal(
      entry.run_state,
      "found",
      "Private run acceptance is not established"
    );
  };
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
    await writeFile(registryConfig, JSON.stringify({ auths: {} }));
    receipt.status = "partial";
    for (const request of releaseCopyRequests(plan, sourceSha, lock)) {
      const target = request.registry_target;
      const record = lock.artifacts.find(
        (entry) => entry.name === `${target}-bicep-types`
      );
      const destination = `biceptypes.azurecr.io/${target}:${record.version}`;
      const entry = { request, destination, status: "pending" };
      receipt.copies.push(entry);
      await save();
      await readback(record.reference, record.digest);
      const inputs = {
        OWNER: "azure-octo",
        REPO: "radius-publisher",
        WORKFLOW_FILE:
          target === "aws" ?
            "publish-bicep-types-aws.yml"
          : "publish-bicep-types.yml",
        EVENT_TYPE: target === "aws" ? "bicep-types-aws" : "bicep-types",
        RELEASE_IDENTIFIER: request.release_identifier,
        CLIENT_PAYLOAD: JSON.stringify(request),
        REUSE_SUCCESSFUL: "false",
        MAX_WAIT_SECONDS: "300",
        POLL_INTERVAL_SECONDS: "10"
      };
      await reconcile({
        github,
        core: { ...remoteCore(entry), getInput: (name) => inputs[name] ?? "" }
      });
      requireSuccess(entry);
      await readback(destination, record.digest);
      entry.status = "verified";
      await save();
    }
    if (channel === "true") {
      for (const request of promotionRequests(
        manifest,
        plan.version,
        sourceSha,
        plan,
        lock
      )) {
        const entry = { request, status: "pending" };
        receipt.promotions.push(entry);
        await save();
        await dispatchPromotion(github, remoteCore(entry), request, reconcile);
        requireSuccess(entry);
        await readback(
          `biceptypes.azurecr.io/${request.registry_target}:${request.channel}`,
          request.digest
        );
        entry.status = "verified";
        await save();
      }
    }
    // Azure writes are complete before local mutable GHCR aliases move.
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

export default async function finalize({ github, core, context }) {
  const load = async (name) =>
    JSON.parse(await readFile(core.getInput(name, { required: true }), "utf8"));
  const plan = await load("PLAN_FILE");
  assert.equal(context.repo.owner, "radius-project");
  assert.equal(context.repo.repo, "radius");
  assert.equal(context.ref, `refs/tags/${plan.version}`);
  await finalizeExtensions({
    github,
    core,
    plan,
    sourceSha: context.sha,
    lock: await load("LOCK_FILE"),
    manifest: await load("MANIFEST_FILE"),
    channel: core.getInput("PROMOTE_CHANNEL", { required: true }),
    latest: core.getInput("PROMOTE_LATEST", { required: true }),
    directory: core.getInput("DIRECTORY", { required: true })
  });
}
