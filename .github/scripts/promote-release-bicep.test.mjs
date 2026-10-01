// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { tool } from "./bicep-types.mjs";
import {
  bicepExtensionTargets,
  plannedBicepExtensions
} from "./release-bicep-extensions.mjs";
import {
  promoteExtensions,
  promotionTargets
} from "./promote-release-bicep.mjs";
import { recoveryInputs, verifyPair } from "./stage-release-bicep.mjs";

const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const sourceSha = "a".repeat(40);
async function fixture(t, version = "v0.62.1") {
  const directory = await mkdtemp(join(tmpdir(), "bicep-promotion-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const plan = {
    schemaVersion: 2,
    version,
    releaseType: version.includes("-rc.") ? "rc" : "patch",
    channel: "0.62",
    expectedOutputs: {
      repository: "radius-project/radius",
      bicepExtensionsContract: "ghcr-v1",
      ociArtifacts: bicepExtensionTargets
    },
    siblingRepositories: [
      {
        name: "bicep-types-aws",
        repository: "radius-project/bicep-types-aws",
        sourceCommit: "b".repeat(40)
      }
    ]
  };
  const store = new Map();
  const artifacts = plannedBicepExtensions(plan, sourceSha).map(
    (record, index) => {
      const media = "application/vnd.ms.bicep.provider.";
      const blob = (type) => ({
        mediaType: media + type,
        digest: hash("blob"),
        size: 4
      });
      const manifest = {
        schemaVersion: 2,
        mediaType: "application/vnd.oci.image.manifest.v1+json",
        artifactType: media + "artifact",
        config: blob("config.v1+json"),
        layers: [blob("layer.v1.tar+gzip")],
        annotations: {
          "bicep.serialization.format": "v1",
          "org.opencontainers.image.source": `https://github.com/${record.source.repository}`,
          "org.opencontainers.image.revision": record.source.commit,
          "org.opencontainers.image.version": record.version
        }
      };
      const bytes = JSON.stringify(manifest);
      const digest = hash(bytes);
      store.set(record.reference, bytes);
      store.set(`${bicepExtensionTargets[index].repository}@${digest}`, bytes);
      return {
        ...record,
        digest,
        generation: {
          workflow: ".github/workflows/build-release.yaml",
          runId: index + 1,
          runAttempt: 1,
          artifactId: index + 10,
          artifactDigest: hash("snapshot")
        }
      };
    }
  );
  const lock = {
    schemaVersion: 1,
    version,
    releaseSourceCommit: sourceSha,
    artifacts
  };
  const copies = [];
  const run = async (args) => {
    const reference = args.at(-1);
    if (args[1] === "cp") {
      copies.push(args);
      assert.ok(store.has(args[2]));
      store.set(args[3], store.get(args[2]));
      return "";
    }
    if (!store.has(reference))
      throw Object.assign(new Error("absent"), {
        code: 1,
        stderr: `Error response from registry: failed to fetch the content of "${reference}": ${reference}: not found`
      });
    return args.includes("--descriptor") ?
        JSON.stringify({ digest: hash(store.get(reference)) })
      : store.get(reference);
  };
  const file = join(directory, "receipt.json");
  return {
    plan,
    lock,
    store,
    run,
    copies,
    file,
    promote: (channel = "true", latest = "true", execute = run) =>
      promoteExtensions(plan, sourceSha, lock, channel, latest, file, execute)
  };
}

test("stable copies both locked packages to eligible aliases and ACR channels, never development latest", async (t) => {
  const f = await fixture(t);
  const receipt = await f.promote();
  assert.equal(receipt.status, "complete");
  assert.equal(f.copies.length, 6);
  assert.ok(f.copies.every((args) => args[2].includes("@sha256:")));
  assert.ok(
    !f.copies.some(
      (args) =>
        /azurecr\.io\/[^:]+:latest$/.test(args[3]) || args[3].endsWith(":edge")
    )
  );
  await f.promote();
  assert.equal(f.copies.length, 6);
});

test("RC mirrors full versions only; conflicting RC cannot be overwritten", async (t) => {
  const f = await fixture(t, "v0.62.0-rc.1");
  assert.throws(() => promotionTargets(f.plan, f.lock, "true", "false"), /RC/);
  await f.promote("false", "false");
  assert.equal(f.copies.length, 2);
  assert.ok(f.copies.every((args) => args[3].endsWith(":0.62.0-rc.1")));
  f.store.set(f.copies[0][3], "{}");
  await assert.rejects(f.promote("false", "false"), /RC tag conflicts/);
  assert.equal(f.copies.length, 2);
});

test("channel and latest follow existing independent eligibility decisions", async (t) => {
  const f = await fixture(t);
  await f.promote("true", "false");
  assert.equal(f.copies.length, 4);
  assert.ok(f.copies.every((args) => args[3].endsWith(":0.62")));
  await f.promote("false", "false");
  assert.equal(f.copies.length, 4);
});

test("partially finalized newer release prevents pair-wide channel/latest downgrade", async (t) => {
  const f = await fixture(t);
  const record = f.lock.artifacts[0];
  const manifest = JSON.parse(f.store.get(record.reference));
  manifest.annotations["org.opencontainers.image.version"] = "0.62.2";
  f.store.set(
    "ghcr.io/radius-project/bicep-types-aws:0.62",
    JSON.stringify(manifest)
  );
  f.store.set(
    "ghcr.io/radius-project/bicep-types-aws:latest",
    JSON.stringify(manifest)
  );
  const receipt = await f.promote();
  assert.equal(f.copies.length, 0);
  assert.ok(
    receipt.destinations.every((entry) => entry.status === "superseded")
  );
});

test("partial mirror receipt survives failure and retry converges on same bytes", async (t) => {
  const f = await fixture(t);
  const failing = async (args) => {
    if (args[1] === "cp" && args[3] === "biceptypes.azurecr.io/aws:0.62")
      throw new Error("mirror permission denied");
    return f.run(args);
  };
  await assert.rejects(f.promote("true", "true", failing));
  const receipt = JSON.parse(await readFile(f.file));
  assert.equal(receipt.status, "partial");
  assert.equal(receipt.destinations[0].status, "verified");
  assert.match(receipt.error, /mirror permission denied/);
  await f.promote();
  assert.equal(f.copies.length, 6);
});

test("auth errors and source/version/digest substitution fail before writes", async (t) => {
  for (const kind of ["auth", "source", "version", "digest", "missing-aws"]) {
    const f = await fixture(t);
    let run = f.run;
    if (kind === "auth")
      run = async () => {
        throw new Error("unauthorized");
      };
    if (kind === "source") f.lock.artifacts[0].source.commit = sourceSha;
    if (kind === "version") f.lock.artifacts[0].version = "0.62.2";
    if (kind === "digest") f.store.set(f.lock.artifacts[0].reference, "{}");
    if (kind === "missing-aws") f.lock.artifacts.shift();
    await assert.rejects(f.promote("true", "true", run));
    assert.equal(f.copies.length, 0);
  }
});

test("post-approval pair verification rejects modified immutable tags", async (t) => {
  const f = await fixture(t);
  await verifyPair(f.plan, sourceSha, f.lock, f.run);
  f.store.set(f.lock.artifacts[1].reference, "{}");
  await assert.rejects(
    verifyPair(f.plan, sourceSha, f.lock, f.run),
    /tag changed/
  );
});

test("cross-run recovery requires paired explicit provenance and never regenerates staged version", async () => {
  const context = { runId: 10, ref: "refs/tags/v0.62.1" };
  const source = { runId: 10, generationAttempt: 1, commit: sourceSha };
  const record = {
    reference: "ghcr.io/radius-project/bicep-types-radius:0.62.1"
  };
  const github = {
    rest: {
      actions: {
        listWorkflowRuns: "runs",
        listWorkflowRunArtifacts: "artifacts"
      }
    },
    paginate: async (method) =>
      method === "runs" ?
        [
          {
            id: 9,
            head_sha: sourceSha,
            head_branch: "v0.62.1",
            event: "push"
          }
        ]
      : []
  };
  await assert.rejects(
    recoveryInputs(github, context, source, record, "", ""),
    /prior release run/
  );
  await assert.rejects(
    recoveryInputs(github, context, source, record, "9", ""),
    /both/
  );
  assert.deepEqual(
    await recoveryInputs(github, context, source, record, "9", "1"),
    { runId: "9", attempt: "1" }
  );
  github.paginate = async () => [];
  await assert.rejects(
    recoveryInputs(github, context, source, record, "", "", async () => "{}"),
    /never regenerate/
  );
});

test(
  "native OCI pair promotion and mirror retry use exact fixture bytes (not AWS generation)",
  {
    skip: !process.env.BICEP_RELEASE_NATIVE_REGISTRY
  },
  async (t) => {
    const registry = process.env.BICEP_RELEASE_NATIVE_REGISTRY;
    assert.match(registry, /^localhost:[0-9]+$/);
    const f = await fixture(t);
    const directory = join(f.file, "..");
    const namespace = randomUUID();
    const mappings = [
      [
        "ghcr.io/radius-project/bicep-types-aws",
        `${registry}/${namespace}/cutover-aws`
      ],
      [
        "ghcr.io/radius-project/bicep-types-radius",
        `${registry}/${namespace}/cutover-radius`
      ],
      ["biceptypes.azurecr.io/aws", `${registry}/${namespace}/mirror-aws`],
      ["biceptypes.azurecr.io/radius", `${registry}/${namespace}/mirror-radius`]
    ];
    const map = (arg) =>
      mappings.reduce((text, [from, to]) => text.replace(from, to), arg);
    const blobFile = join(directory, "blob");
    await writeFile(blobFile, "blob");
    for (const artifact of f.lock.artifacts) {
      const repository = map(artifact.reference.split(":")[0]);
      await tool([
        "oras",
        "blob",
        "push",
        "--plain-http",
        repository,
        blobFile
      ]);
      const file = join(directory, `${artifact.name}.json`);
      await writeFile(file, f.store.get(artifact.reference));
      await tool([
        "oras",
        "manifest",
        "push",
        "--plain-http",
        map(artifact.reference),
        file
      ]);
    }
    const copies = [];
    const run = async (args) => {
      const mapped = args.map(map);
      if (args[1] === "cp") {
        mapped.splice(2, 0, "--from-plain-http", "--to-plain-http");
        copies.push(args);
      } else {
        mapped.splice(3, 0, "--plain-http");
      }
      try {
        return await tool(mapped);
      } catch (error) {
        if (error.stderr)
          error.stderr = mappings.reduce(
            (text, [from, to]) => text.replaceAll(to, from),
            error.stderr
          );
        throw error;
      }
    };
    await f.promote("true", "true", run);
    await f.promote("true", "true", run);
    assert.equal(copies.length, 6);
    for (const entry of JSON.parse(await readFile(f.file)).destinations) {
      assert.equal(
        hash(await run(["oras", "manifest", "fetch", entry.reference])),
        entry.digest
      );
    }
  }
);
