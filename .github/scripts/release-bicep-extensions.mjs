// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

export const bicepExtensionLockName = "bicep-extension-lock.json";
const providerType = "application/vnd.ms.bicep.provider.";
const manifestType = "application/vnd.oci.image.manifest.v1+json";
const digestPattern = /^sha256:[a-f0-9]{64}$/;
const commitPattern = /^(?!0{40}$)[a-f0-9]{40}$/;
const versionPattern =
  /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-rc\.[1-9]\d*)?$/;

export const bicepExtensionTargets = ["aws", "radius"].map((name) => ({
  name: `${name}-bicep-types`,
  repository: `ghcr.io/radius-project/bicep-types-${name}`,
  artifactType: `${providerType}artifact`
}));

// Absence is the historical contract, not a registry lookup fallback.
export function usesGhcrBicepExtensions(outputs) {
  if (!Object.hasOwn(outputs, "bicepExtensionsContract")) {
    assert.ok(
      outputs.ociArtifacts.every((entry) =>
        entry.repository.startsWith("biceptypes.azurecr.io/")
      ),
      "GHCR Bicep outputs require an approved contract selector"
    );
    return false;
  }
  assert.equal(
    outputs.bicepExtensionsContract,
    "ghcr-v1",
    "Unknown Bicep contract"
  );
  assert.deepEqual(
    outputs.ociArtifacts,
    bicepExtensionTargets,
    "Invalid Bicep target pair"
  );
  return true;
}

export function plannedBicepExtensions(plan, sourceSha) {
  assert.equal(plan.schemaVersion, 2, "Unsupported release plan schema");
  assert.ok(
    usesGhcrBicepExtensions(plan.expectedOutputs),
    "GHCR Bicep contract required"
  );
  assert.match(plan.version, versionPattern, "Invalid Bicep release version");
  assert.match(sourceSha, commitPattern, "Invalid Radius release source");
  assert.equal(plan.expectedOutputs.repository, "radius-project/radius");
  assert.ok(["rc", "final", "patch"].includes(plan.releaseType));
  assert.equal(plan.releaseType === "rc", plan.version.includes("-rc."));
  const siblings = plan.siblingRepositories.filter(
    (entry) => entry.repository === "radius-project/bicep-types-aws"
  );
  assert.equal(siblings.length, 1, "Exactly one frozen AWS source is required");
  assert.equal(siblings[0].name, "bicep-types-aws");
  assert.match(
    siblings[0].sourceCommit,
    commitPattern,
    "Invalid frozen AWS source"
  );
  const version = plan.version.slice(1);
  return bicepExtensionTargets.map((target) => ({
    name: target.name,
    source: {
      repository:
        target.name === "radius-bicep-types" ?
          "radius-project/radius"
        : "radius-project/bicep-types-aws",
      commit:
        target.name === "radius-bicep-types" ?
          sourceSha
        : siblings[0].sourceCommit
    },
    version,
    reference: `${target.repository}:${version}`
  }));
}

export function validateBicepExtensionLock(plan, sourceSha, lock) {
  const expected = plannedBicepExtensions(plan, sourceSha);
  assert.equal(
    lock?.schemaVersion,
    1,
    "Missing or unsupported Bicep extension lock"
  );
  assert.equal(
    lock.version,
    plan.version,
    "Bicep lock release version mismatch"
  );
  assert.equal(
    lock.releaseSourceCommit,
    sourceSha,
    "Bicep lock release source mismatch"
  );
  assert.deepEqual(
    lock.artifacts?.map((entry) => entry.name),
    expected.map((entry) => entry.name),
    "Bicep lock must contain exactly the AWS and Radius outputs in canonical order"
  );
  for (const [index, artifact] of lock.artifacts.entries()) {
    for (const [key, value] of Object.entries(expected[index])) {
      assert.deepEqual(
        artifact[key],
        value,
        `Bicep ${artifact.name} ${key} mismatch`
      );
    }
    assert.match(
      artifact.digest,
      digestPattern,
      "Invalid Bicep manifest digest"
    );
    const generation = artifact.generation;
    assert.ok(generation, "Missing Bicep generation provenance");
    assert.match(
      generation.workflow,
      /^\.github\/workflows\/[a-zA-Z0-9_-]+\.ya?ml$/
    );
    for (const key of ["runId", "runAttempt", "artifactId"]) {
      assert.ok(
        Number.isSafeInteger(generation[key]) && generation[key] > 0,
        `Invalid Bicep generation ${key}`
      );
    }
    assert.match(
      generation.artifactDigest,
      digestPattern,
      "Missing Bicep snapshot digest"
    );
  }
}

export function verifyBicepExtensionOutputs(plan, sourceSha, lock, observed) {
  validateBicepExtensionLock(plan, sourceSha, lock);
  assert.deepEqual(
    observed?.map((entry) => entry.name).sort(),
    bicepExtensionTargets.map((entry) => entry.name),
    "Invalid observed Bicep output pair"
  );
  for (const artifact of lock.artifacts) {
    const actual = observed.find((entry) => entry.name === artifact.name);
    assert.equal(
      actual.reference,
      artifact.reference,
      "Bicep full-version reference mismatch"
    );
    assert.equal(
      actual.descriptor?.digest,
      artifact.digest,
      "Bicep locked digest mismatch"
    );
    const manifest = actual.manifest;
    assert.equal(manifest?.schemaVersion, 2);
    assert.equal(manifest.mediaType, manifestType);
    assert.equal(manifest.artifactType, `${providerType}artifact`);
    assert.equal(manifest.config?.mediaType, `${providerType}config.v1+json`);
    assert.equal(manifest.layers?.length, 1, "Unexpected Bicep layers");
    assert.equal(
      manifest.layers[0].mediaType,
      `${providerType}layer.v1.tar+gzip`
    );
    assert.ok(!manifest.subject, "Unexpected Bicep subject");
    for (const blob of [manifest.config, ...manifest.layers]) {
      assert.match(blob.digest, digestPattern);
      assert.ok(Number.isSafeInteger(blob.size) && blob.size >= 0);
      assert.ok(!blob.urls, "Unexpected Bicep foreign blob URLs");
    }
    for (const [name, value] of Object.entries({
      "bicep.serialization.format": "v1",
      "org.opencontainers.image.source": `https://github.com/${artifact.source.repository}`,
      "org.opencontainers.image.revision": artifact.source.commit,
      "org.opencontainers.image.version": artifact.version
    })) {
      assert.equal(
        manifest.annotations?.[name],
        value,
        `Bicep ${name} mismatch`
      );
    }
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const [mode, planFile, sourceSha, lockFile, observedFile] =
    process.argv.slice(2);
  const load = async (file) => JSON.parse(await readFile(file, "utf8"));
  const plan = await load(planFile);
  if (mode === "targets") {
    console.log(JSON.stringify(plannedBicepExtensions(plan, sourceSha)));
  } else if (mode === "verify") {
    verifyBicepExtensionOutputs(
      plan,
      sourceSha,
      await load(lockFile),
      await load(observedFile)
    );
  } else {
    throw new Error(`Unsupported Bicep contract operation: ${mode}`);
  }
}
