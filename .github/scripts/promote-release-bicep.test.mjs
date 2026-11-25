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
  promotionTargets,
  finalizeExtensions
} from "./promote-release-bicep.mjs";
import { recoveryInputs, verifyPair } from "./stage-release-bicep.mjs";
import { selectAliases, mandatoryChecks } from "./publish-draft-release.mjs";

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
      const bytes = JSON.stringify(manifest),
        digest = hash(bytes);
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
  const manifest = {
    schemaVersion: 1,
    tag: plan.version,
    sourceSha,
    channel: plan.channel,
    releaseType: plan.releaseType,
    checks: Object.fromEntries(
      mandatoryChecks.map((name) => [name, "verified"])
    ),
    observed: {
      downstream: {
        bicepExtensionLock: structuredClone(lock),
        ociArtifacts: lock.artifacts.map((record) => ({
          name: record.name,
          reference: record.reference,
          descriptor: { digest: record.digest },
          manifest: JSON.parse(store.get(record.reference))
        }))
      }
    }
  };
  const copies = [],
    calls = [];
  const run = async (args, options) => {
    calls.push(args);
    assert.ok(args.at(-1).startsWith("ghcr.io/"), "No production ACR requests");
    if (options) assert.ok(options.timeout > 0 && options.timeout <= 30_000);
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
    manifest,
    store,
    run,
    copies,
    calls,
    file,
    directory,
    promote: (channel = "true", latest = "true", execute = run) =>
      promoteExtensions(plan, sourceSha, lock, channel, latest, file, execute),
    finalize: (channel = "true", latest = "true", overrides = {}) =>
      finalizeExtensions({
        plan,
        sourceSha,
        lock,
        manifest,
        channel,
        latest,
        directory,
        run,
        ...overrides
      })
  };
}

test("finalization publishes GHCR only, retries exact bytes and never rewrites locks", async (t) => {
  for (const [version, channel, latest, count] of [
    ["v0.62.1", "true", "true", 4],
    ["v0.62.1", "true", "false", 2],
    ["v0.62.1", "false", "false", 0],
    ["v0.62.0-rc.1", "false", "false", 0]
  ]) {
    const f = await fixture(t, version);
    const before = JSON.stringify(f.lock);
    const receipt = await f.finalize(channel, latest);
    assert.equal(receipt.status, "complete");
    assert.equal(receipt.copies, undefined);
    assert.equal(receipt.promotions, undefined);
    assert.equal(f.copies.length, count);
    assert.ok(f.copies.every((args) => args[2].includes("@sha256:")));
    await f.finalize(channel, latest);
    assert.equal(f.copies.length, count);
    assert.equal(JSON.stringify(f.lock), before);
  }
});

test("unapproved manifest and substituted exact pair fail before writes", async (t) => {
  for (const mutate of [
    (f) => {
      f.manifest.checks.installation = "pending";
    },
    (f) => {
      f.manifest.sourceSha = "f".repeat(40);
    },
    (f) => {
      f.manifest.channel = "0.63";
    },
    (f) => {
      f.manifest.releaseType = "rc";
    },
    (f) => {
      f.manifest.observed.downstream.bicepExtensionLock.artifacts[0].digest =
        hash("changed");
    },
    (f) => {
      f.plan.version = "v0.62.2";
    },
    (f) => {
      f.lock.artifacts[0].source.commit = sourceSha;
    },
    (f) => {
      f.lock.artifacts[1].digest = hash("changed");
    },
    (f) => {
      f.plan.expectedOutputs.bicepExtensionsContract = "";
    },
    (f) => {
      f.lock.artifacts.pop();
    },
    (f) => {
      f.store.set(f.lock.artifacts[1].reference, "{}");
    }
  ]) {
    const f = await fixture(t);
    mutate(f);
    await assert.rejects(f.finalize());
    assert.equal(f.copies.length, 0);
    assert.notEqual(JSON.parse(await readFile(f.file)).status, "complete");
  }
});

test("both locked full-version tags must be anonymously readable before authenticated alias writes", async (t) => {
  for (const failure of [
    "none",
    "aws denied",
    "radius denied",
    "radius moved"
  ]) {
    const f = await fixture(t);
    await verifyPair(f.plan, sourceSha, f.lock, f.run);
    const anonymousTags = new Set();
    const run = async (args, options) => {
      const configIndex = args.indexOf("--registry-config");
      if (configIndex !== -1) {
        assert.deepEqual(
          JSON.parse(await readFile(args[configIndex + 1], "utf8")),
          { auths: {} }
        );
        const tag = f.lock.artifacts.find(
          (entry) => entry.reference === args.at(-1)
        );
        if (tag) {
          anonymousTags.add(tag.name);
          if (failure === "aws denied" && tag.name === "aws-bicep-types")
            throw new Error("anonymous AWS read denied");
          if (failure === "radius denied" && tag.name === "radius-bicep-types")
            throw new Error("anonymous Radius read denied");
          if (failure === "radius moved" && tag.name === "radius-bicep-types")
            return JSON.stringify({ digest: hash("other manifest") });
        }
      }
      if (args[1] === "cp") {
        assert.equal(configIndex, -1, "Writes must retain GHCR login");
        assert.deepEqual([...anonymousTags].sort(), [
          "aws-bicep-types",
          "radius-bicep-types"
        ]);
      }
      return f.run(args, options);
    };
    if (failure === "none") {
      await f.finalize("true", "true", { run });
      assert.equal(f.copies.length, 4);
    } else {
      await assert.rejects(
        f.finalize("true", "true", { run }),
        /anonymous|tag changed/
      );
      assert.equal(f.copies.length, 0);
      assert.notEqual(JSON.parse(await readFile(f.file)).status, "complete");
    }
  }
});

test("finalization auth errors and budget exhaustion retain failures without ACR fallback", async (t) => {
  for (const mode of ["auth", "budget"]) {
    const f = await fixture(t);
    let time = 0;
    await assert.rejects(
      f.finalize("true", "true", {
        now: () => time,
        run: async (args, options) => {
          if (mode === "auth") throw new Error("401 denied");
          const result = await f.run(args, options);
          time = 24 * 60_000;
          return result;
        }
      }),
      mode === "auth" ? /401 denied/ : /budget exhausted/
    );
    assert.equal(f.copies.length, 0);
    assert.ok(JSON.parse(await readFile(f.file)).error);
  }
});

test("partial finalization survives alias failure and converges without recopying successful aliases", async (t) => {
  const f = await fixture(t);
  await assert.rejects(
    f.finalize("true", "true", {
      run: async (args, options) => {
        if (
          args[1] === "cp" &&
          args[3] === "ghcr.io/radius-project/bicep-types-radius:0.62"
        )
          throw new Error("alias permission denied");
        return f.run(args, options);
      }
    })
  );
  assert.equal(JSON.parse(await readFile(f.file)).status, "partial");
  const partial = JSON.parse(
    await readFile(join(f.directory, "ghcr-receipt.json"))
  );
  assert.equal(partial.destinations[0].status, "verified");
  assert.match(partial.error, /alias permission denied/);
  await f.finalize();
  assert.equal(f.copies.length, 4);
});

test("late helper entry respects job deadline and leaves time to upload receipts", async (t) => {
  const f = await fixture(t);
  await assert.rejects(
    f.finalize("true", "true", {
      now: () => 28 * 60_000,
      jobDeadline: 30 * 60_000,
      run: async () => assert.fail("no late registry operation")
    }),
    /budget exhausted/
  );
  const receipt = JSON.parse(await readFile(join(f.directory, "receipt.json")));
  assert.notEqual(receipt.status, "complete");
  assert.equal(f.copies.length, 0);
  const workflow = JSON.parse(
    await tool(["yq", "-o=json", ".", ".github/workflows/build-release.yaml"])
  );
  for (const name of ["finalize-release"]) {
    assert.equal(
      workflow.jobs[name].steps[0].name,
      "Record Bicep job deadline"
    );
    assert.ok(
      workflow.jobs[name].steps.some(
        (step) =>
          step.env?.INPUT_JOB_DEADLINE === "${{ env.BICEP_JOB_DEADLINE }}"
      )
    );
  }
});

test("uncertain alias write requires exact readback before claiming success", async (t) => {
  const f = await fixture(t);
  const receipt = await f.finalize("true", "true", {
    run: async (args, options) => {
      const result = await f.run(args, options);
      if (args[1] === "cp") throw new Error("response lost");
      return result;
    }
  });
  assert.ok(
    receipt.ghcr.destinations.every((entry) => entry.recoveredUncertainWrite)
  );
  assert.equal(f.copies.length, 4);
});

test("RC never writes stable aliases", async (t) => {
  const f = await fixture(t, "v0.62.0-rc.1");
  assert.throws(() => promotionTargets(f.plan, f.lock, "true", "false"), /RC/);
  await assert.rejects(f.finalize("false", "true"), /RC/);
  await f.finalize("false", "false");
  assert.equal(f.copies.length, 0);
});

test("existing eligibility drives aliases without touching development tags", async (t) => {
  for (const [version, published, count, latest] of [
    ["v0.62.1", [], 4, 2],
    ["v0.62.1", ["v0.63.0"], 2, 0],
    ["v0.62.1", ["v0.62.2"], 0, 0],
    ["v0.62.0-rc.1", [], 0, 0]
  ]) {
    const f = await fixture(t, version);
    const policy = selectAliases(
      published.map((tag_name) => ({
        tag_name,
        draft: false,
        prerelease: false
      })),
      version
    );
    const protectedRefs = [
      "biceptypes.azurecr.io/aws:latest",
      "biceptypes.azurecr.io/radius:latest",
      "ghcr.io/radius-project/bicep-types-aws:edge",
      "ghcr.io/radius-project/bicep-types-radius:edge"
    ];
    for (const ref of protectedRefs) f.store.set(ref, "development-bytes");
    await f.finalize(String(policy.channel), String(policy.latest));
    assert.equal(f.copies.length, count);
    assert.equal(
      f.copies.filter((args) => args[3].endsWith(":latest")).length,
      latest
    );
    for (const ref of protectedRefs)
      assert.equal(f.store.get(ref), "development-bytes");
  }
  const workflow = JSON.parse(
    await tool(["yq", "-o=json", ".", ".github/workflows/build-release.yaml"])
  );
  for (const name of [
    "Promote stable aliases",
    "Promote locked Bicep extensions"
  ]) {
    const step = workflow.jobs["finalize-release"].steps.find(
      (step) => step.name === name
    );
    assert.equal(
      step.env.RELEASE_PROMOTE_LATEST ?? step.env.INPUT_PROMOTE_LATEST,
      "${{ steps.alias-policy.outputs.promote_latest }}"
    );
  }
});

test("partially finalized newer release prevents pair-wide downgrade", async (t) => {
  const f = await fixture(t);
  const manifest = JSON.parse(f.store.get(f.lock.artifacts[0].reference));
  manifest.annotations["org.opencontainers.image.version"] = "0.62.2";
  for (const tag of ["0.62", "latest"])
    f.store.set(
      `ghcr.io/radius-project/bicep-types-aws:${tag}`,
      JSON.stringify(manifest)
    );
  const receipt = await f.finalize();
  assert.equal(f.copies.length, 0);
  assert.ok(
    receipt.ghcr.destinations.every((entry) => entry.status === "superseded")
  );
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

test("cross-run recovery requires explicit provenance and never regenerates staged version", async () => {
  const context = { runId: 10, ref: "refs/tags/v0.62.1" };
  const source = {
    runId: 10,
    generationAttempt: 1,
    commit: sourceSha,
    repository: "radius-project/radius",
    workflow: ".github/workflows/build-release.yaml",
    ref: context.ref
  };
  const record = {
    reference: "ghcr.io/radius-project/bicep-types-radius:0.62.1"
  };
  const github = {
    rest: {
      actions: {
        getWorkflowRunAttempt: async () => ({
          data: {
            id: 10,
            run_attempt: 1,
            run_number: 10,
            head_sha: sourceSha,
            head_branch: "v0.62.1",
            event: "push",
            path: source.workflow,
            repository: { full_name: source.repository },
            head_repository: { full_name: source.repository }
          }
        }),
        listWorkflowRuns: "runs",
        listWorkflowRunArtifacts: "artifacts"
      }
    },
    paginate: async (method) =>
      method === "runs" ?
        [{ id: 9, head_sha: sourceSha, head_branch: "v0.62.1", event: "push" }]
      : []
  };
  await assert.rejects(
    recoveryInputs(github, context, source, record, "", ""),
    /Unknown prior generation history/
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
  "native OCI GHCR-only finalization preserves exact manifest and blob bytes",
  {
    skip: !process.env.BICEP_RELEASE_NATIVE_REGISTRY
  },
  async (t) => {
    const registry = process.env.BICEP_RELEASE_NATIVE_REGISTRY;
    assert.match(registry, /^localhost:[0-9]+$/);
    for (const version of ["v0.62.1", "v0.62.0-rc.1"]) {
      const f = await fixture(t, version);
      const namespace = randomUUID();
      const mappings = bicepExtensionTargets.map((entry, index) => [
        entry.repository,
        `${registry}/${namespace}/extension-${index}`
      ]);
      const map = (arg) =>
        mappings.reduce((text, [from, to]) => text.replace(from, to), arg);
      const blobFile = join(f.directory, "blob");
      await writeFile(blobFile, "blob");
      for (const artifact of f.lock.artifacts) {
        await tool([
          "oras",
          "blob",
          "push",
          "--plain-http",
          map(artifact.reference.split(":")[0]),
          blobFile
        ]);
        const file = join(f.directory, `${artifact.name}.json`);
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
      const run = async (args, options) => {
        assert.ok(args.at(-1).startsWith("ghcr.io/"), "No ACR requests");
        const mapped = args.map(map);
        if (args[1] === "cp") {
          mapped.splice(2, 0, "--from-plain-http", "--to-plain-http");
          copies.push(args);
        } else mapped.splice(3, 0, "--plain-http");
        try {
          return await tool(mapped, options);
        } catch (error) {
          if (error.stderr)
            error.stderr = mappings.reduce(
              (text, [from, to]) => text.replaceAll(to, from),
              error.stderr
            );
          throw error;
        }
      };
      const stable = String(!version.includes("-rc."));
      await f.finalize(stable, stable, { run });
      const receipt = await f.finalize(stable, stable, { run });
      assert.equal(copies.length, stable === "true" ? 4 : 0);
      for (const entry of [...f.lock.artifacts, ...receipt.ghcr.destinations]) {
        const bytes = await run(["oras", "manifest", "fetch", entry.reference]);
        assert.equal(hash(bytes), entry.digest);
        const manifest = JSON.parse(bytes);
        for (const blob of [manifest.config, ...manifest.layers]) {
          const repository = map(entry.reference).slice(
            0,
            map(entry.reference).lastIndexOf(":")
          );
          const bytes = await tool([
            "oras",
            "blob",
            "fetch",
            "--plain-http",
            "--output",
            "-",
            `${repository}@${blob.digest}`
          ]);
          assert.equal(hash(bytes), blob.digest);
        }
      }
    }
  }
);
