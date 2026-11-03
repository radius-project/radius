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
  releaseCopyRequests,
  finalizeExtensions
} from "./promote-release-bicep.mjs";
import { recoveryInputs, verifyPair } from "./stage-release-bicep.mjs";
import { selectAliases } from "./publish-draft-release.mjs";
import { mandatoryChecks } from "./publish-draft-release.mjs";

const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const sourceSha = "a".repeat(40);

function remoteFixture(f) {
  const manifest = {
    schemaVersion: 1,
    tag: f.plan.version,
    sourceSha,
    channel: f.plan.channel,
    releaseType: f.plan.releaseType,
    checks: Object.fromEntries(
      mandatoryChecks.map((name) => [name, "verified"])
    ),
    observed: {
      downstream: {
        bicepExtensionLock: structuredClone(f.lock),
        ociArtifacts: f.lock.artifacts.map((record) => ({
          name: record.name,
          reference: record.reference,
          descriptor: { digest: record.digest },
          manifest: JSON.parse(f.store.get(record.reference))
        }))
      }
    }
  };
  const calls = [];
  const monitor = async ({ core }) => {
    const request = JSON.parse(core.getInput("CLIENT_PAYLOAD"));
    assert.equal(core.getInput("OWNER"), "azure-octo");
    assert.equal(core.getInput("REPO"), "radius-publisher");
    assert.equal(core.getInput("REUSE_SUCCESSFUL"), "false");
    assert.equal(core.getInput("MAX_WAIT_SECONDS"), "300");
    const target = request.registry_target;
    const record = f.lock.artifacts.find(
      (entry) => entry.name === `${target}-bicep-types`
    );
    const copy = core.getInput("EVENT_TYPE") !== "bicep-types.promote";
    if (copy) {
      assert.equal(request.mode, "copy");
      assert.ok(!Object.hasOwn(request, "source_evidence"));
      assert.equal(request.source_repository, record.source.repository);
      assert.equal(request.source_sha, record.source.commit);
      assert.equal(request.source_ref, `refs/tags/${f.plan.version}`);
      assert.equal(
        core.getInput("WORKFLOW_FILE"),
        target === "aws" ?
          "publish-bicep-types-aws.yml"
        : "publish-bicep-types.yml"
      );
      assert.equal(
        request.release_identifier,
        `copy-${target}-${record.version}-${record.source.commit}-${record.digest.slice(7)}`
      );
      const destination = `biceptypes.azurecr.io/${target}:${record.version}`;
      const existing = f.store.get(destination);
      if (existing && hash(existing) !== record.digest)
        core.setFailed("Immutable version conflict");
      f.store.set(destination, f.store.get(record.reference));
    } else {
      assert.equal(request.source_repository, "radius-project/radius");
      assert.equal(request.source_sha, sourceSha);
      assert.equal(
        calls.filter((entry) => entry.mode === "copy").length >= 2,
        true
      );
      assert.equal(core.getInput("WORKFLOW_FILE"), "promote-bicep-types.yml");
      const bytes = f.store.get(
        `biceptypes.azurecr.io/${target}:${record.version}`
      );
      assert.equal(hash(bytes), request.digest);
      f.store.set(`biceptypes.azurecr.io/${target}:${request.channel}`, bytes);
    }
    calls.push(request);
    core.setOutput("run_id", String(900 + calls.length));
    core.setOutput(
      "run_url",
      `https://github.com/azure-octo/radius-publisher/actions/runs/${900 + calls.length}`
    );
    core.setOutput("run_state", "found");
    core.setOutput("conclusion", "success");
  };
  const run = async (args, options) => {
    if (args[1] === "cp")
      assert.ok(args[3].startsWith("ghcr.io/"), "Radius cannot write ACR");
    if (args.at(-1).startsWith("biceptypes.azurecr.io/")) {
      assert.ok(
        args.includes("--registry-config"),
        "ACR read must be anonymous"
      );
      const config = JSON.parse(
        await readFile(args[args.indexOf("--registry-config") + 1])
      );
      assert.deepEqual(config, { auths: {} });
      assert.equal(options.timeout, 30_000);
    }
    return f.run(args);
  };
  const directory = join(f.file, "../remote");
  return {
    manifest,
    calls,
    monitor,
    run,
    directory,
    finalize: (channel = "true", latest = "true", overrides = {}) =>
      finalizeExtensions({
        github: {},
        core: { info() {} },
        plan: f.plan,
        sourceSha,
        lock: f.lock,
        manifest,
        channel,
        latest,
        directory,
        monitor,
        run,
        ...overrides
      })
  };
}

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

test("private full copies precede private channels and local GHCR aliases for stable and RC", async (t) => {
  for (const [version, channel, latest, promotions] of [
    ["v0.62.1", "true", "true", 2],
    ["v0.62.1", "false", "false", 0],
    ["v0.62.0-rc.1", "false", "false", 0]
  ]) {
    const f = await fixture(t, version);
    const r = remoteFixture(f);
    const receipt = await r.finalize(channel, latest);
    assert.equal(receipt.status, "complete");
    assert.equal(receipt.copies.length, 2);
    assert.equal(receipt.promotions.length, promotions);
    assert.deepEqual(
      r.calls.slice(0, 2).map((entry) => entry.registry_target),
      ["aws", "radius"]
    );
    assert.equal(
      r.calls[0].rel_channel,
      version.includes("-rc.") ? version.slice(1) : "0.62"
    );
    for (const record of f.lock.artifacts) {
      const target = record.name.split("-")[0];
      assert.equal(
        hash(f.store.get(`biceptypes.azurecr.io/${target}:${record.version}`)),
        record.digest
      );
      assert.equal(
        f.store.has(`biceptypes.azurecr.io/${target}:latest`),
        false
      );
    }
    assert.equal(f.copies.length, promotions ? 4 : 0);
    await r.finalize(channel, latest);
    assert.equal(
      r.calls.filter((entry) => entry.mode === "copy").length,
      4,
      "retries reconcile remote copies instead of trusting cached success"
    );
  }
});

test("unknown/failed private work and mismatched anonymous readback retain partial receipt", async (t) => {
  for (const mode of ["unknown", "failure", "digest", "auth"]) {
    const f = await fixture(t);
    const r = remoteFixture(f);
    let count = 0;
    const monitor = async (args) => {
      if (++count === 2) {
        args.core.setOutput("run_id", "901");
        args.core.setOutput(
          "run_state",
          mode === "unknown" ? "unknown" : "found"
        );
        if (mode === "unknown" || mode === "failure")
          args.core.setFailed(`${mode} dispatch`);
      }
      await r.monitor(args);
      if (count === 2 && mode === "digest")
        f.store.set("biceptypes.azurecr.io/radius:0.62.1", "{}");
    };
    const run = async (args, options) => {
      if (
        mode === "auth" &&
        args.at(-1) === "biceptypes.azurecr.io/radius:0.62.1"
      )
        throw new Error("401 denied");
      return r.run(args, options);
    };
    await assert.rejects(r.finalize("true", "true", { monitor, run }));
    const receipt = JSON.parse(
      await readFile(join(r.directory, "receipt.json"))
    );
    assert.equal(receipt.status, "partial");
    assert.equal(receipt.copies[0].status, "verified");
    assert.equal(receipt.promotions.length, 0);
    assert.equal(f.copies.length, 0);
    assert.ok(receipt.copies[1].run_id);
    if (mode !== "digest") {
      await r.finalize();
      assert.equal(f.copies.length, 4);
    } else {
      await assert.rejects(r.finalize(), /Immutable version conflict/);
    }
  }
});

test("approved lock source/tag/digest substitutions reject before remote operations", async (t) => {
  for (const mutate of [
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
    }
  ]) {
    const f = await fixture(t);
    const r = remoteFixture(f);
    mutate(f);
    await assert.rejects(r.finalize());
    assert.equal(r.calls.length, 0);
  }
});

test("tagged requests are stable across publication retries and use durable lock without Actions reads", async (t) => {
  const f = await fixture(t);
  const before = JSON.stringify(f.lock);
  const requests = releaseCopyRequests(f.plan, sourceSha, f.lock);
  const r = remoteFixture(f);
  await r.finalize("false", "false", {
    github: new Proxy(
      {},
      {
        get() {
          throw new Error("No source Actions reads");
        }
      }
    )
  });
  assert.deepEqual(releaseCopyRequests(f.plan, sourceSha, f.lock), requests);
  assert.equal(JSON.stringify(f.lock), before);
  assert.ok(
    requests.every((entry) => !Object.hasOwn(entry, "source_evidence"))
  );
});

test("private promotion failure stops local aliases and retry preserves accepted full versions", async (t) => {
  const f = await fixture(t);
  const r = remoteFixture(f);
  await assert.rejects(
    r.finalize("true", "true", {
      monitor: async (args) => {
        if (args.core.getInput("EVENT_TYPE") === "bicep-types.promote")
          args.core.setFailed("promotion denied");
        await r.monitor(args);
      }
    }),
    /promotion denied/
  );
  const receipt = JSON.parse(await readFile(join(r.directory, "receipt.json")));
  assert.ok(receipt.copies.every((entry) => entry.status === "verified"));
  assert.equal(f.copies.length, 0);
  await r.finalize();
  assert.equal(f.copies.length, 4);
});

test("no-op monitor success without authenticated run/conclusion cannot claim a copy", async (t) => {
  const f = await fixture(t);
  const r = remoteFixture(f);
  await assert.rejects(
    r.finalize("false", "false", { monitor: async () => {} }),
    /Private publication did not succeed/
  );
  assert.equal(f.copies.length, 0);
});

test("unapproved manifest or changed paired evidence fails before dispatch", async (t) => {
  for (const mutate of [
    (manifest) => {
      manifest.checks.installation = "pending";
    },
    (manifest) => {
      manifest.sourceSha = "f".repeat(40);
    },
    (manifest) => {
      manifest.observed.downstream.bicepExtensionLock.artifacts[0].digest =
        hash("changed");
    }
  ]) {
    const f = await fixture(t);
    const r = remoteFixture(f);
    mutate(r.manifest);
    await assert.rejects(r.finalize());
    assert.equal(r.calls.length, 0);
    assert.equal(f.copies.length, 0);
  }
});

test("finalization budget exhaustion retains partial copies without further dispatch", async (t) => {
  const f = await fixture(t);
  const r = remoteFixture(f);
  let time = 0;
  await assert.rejects(
    r.finalize("true", "true", {
      now: () => time,
      monitor: async (args) => {
        await r.monitor(args);
        time = 24 * 60_000;
      }
    }),
    /budget exhausted/
  );
  const receipt = JSON.parse(await readFile(join(r.directory, "receipt.json")));
  assert.equal(receipt.status, "partial");
  assert.equal(receipt.copies.length, 1);
  assert.equal(receipt.copies[0].conclusion, "success");
  assert.equal(receipt.promotions.length, 0);
  assert.equal(f.copies.length, 0);
});

test("local stable promotion writes GHCR aliases only, never production ACR", async (t) => {
  const f = await fixture(t);
  const receipt = await f.promote();
  assert.equal(receipt.status, "complete");
  assert.equal(f.copies.length, 4);
  assert.ok(f.copies.every((args) => args[3].startsWith("ghcr.io/")));
  assert.ok(f.copies.every((args) => args[2].includes("@sha256:")));
  assert.ok(
    !f.copies.some(
      (args) =>
        /azurecr\.io\/[^:]+:latest$/.test(args[3]) || args[3].endsWith(":edge")
    )
  );
  await f.promote();
  assert.equal(f.copies.length, 4);
});

test("RC never writes local stable aliases or ACR", async (t) => {
  const f = await fixture(t, "v0.62.0-rc.1");
  assert.throws(() => promotionTargets(f.plan, f.lock, "true", "false"), /RC/);
  await f.promote("false", "false");
  assert.equal(f.copies.length, 0);
});

test("channel and latest follow existing independent eligibility decisions", async (t) => {
  const f = await fixture(t);
  await f.promote("true", "false");
  assert.equal(f.copies.length, 2);
  assert.ok(f.copies.every((args) => args[3].endsWith(":0.62")));
  await f.promote("false", "false");
  assert.equal(f.copies.length, 2);
});

test("shared release eligibility drives GHCR latest while preserving development aliases", async (t) => {
  for (const [version, published, expectedCopies, expectedLatest] of [
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
    await f.promote(String(policy.channel), String(policy.latest));
    assert.equal(f.copies.length, expectedCopies);
    assert.equal(
      f.copies.filter((args) => args[3].endsWith(":latest")).length,
      expectedLatest
    );
    for (const ref of protectedRefs)
      assert.equal(f.store.get(ref), "development-bytes");
  }
  const workflow = JSON.parse(
    await tool(["yq", "-o=json", ".", ".github/workflows/build-release.yaml"])
  );
  const steps = workflow.jobs["finalize-release"].steps;
  for (const name of [
    "Promote stable aliases",
    "Promote and mirror locked Bicep extensions"
  ])
    assert.equal(
      steps.find((step) => step.name === name).env.RELEASE_PROMOTE_LATEST ??
        steps.find((step) => step.name === name).env.INPUT_PROMOTE_LATEST,
      "${{ steps.alias-policy.outputs.promote_latest }}"
    );
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

test("partial GHCR receipt survives failure and retry converges on same bytes", async (t) => {
  const f = await fixture(t);
  const failing = async (args) => {
    if (
      args[1] === "cp" &&
      args[3] === "ghcr.io/radius-project/bicep-types-radius:0.62"
    )
      throw new Error("alias permission denied");
    return f.run(args);
  };
  await assert.rejects(f.promote("true", "true", failing));
  const receipt = JSON.parse(await readFile(f.file));
  assert.equal(receipt.status, "partial");
  assert.equal(receipt.destinations[0].status, "verified");
  assert.match(receipt.error, /alias permission denied/);
  await f.promote();
  assert.equal(f.copies.length, 4);
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
  "native OCI private-copy transport fixture and local aliases preserve exact bytes",
  {
    skip: !process.env.BICEP_RELEASE_NATIVE_REGISTRY
  },
  async (t) => {
    const registry = process.env.BICEP_RELEASE_NATIVE_REGISTRY;
    assert.match(registry, /^localhost:[0-9]+$/);
    for (const version of ["v0.62.1", "v0.62.0-rc.1"]) {
      const f = await fixture(t, version);
      const r = remoteFixture(f);
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
        [
          "biceptypes.azurecr.io/radius",
          `${registry}/${namespace}/mirror-radius`
        ]
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
          assert.ok(
            args[3].startsWith("ghcr.io/"),
            "Public caller cannot write production ACR"
          );
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
      const monitor = async (args) => {
        await r.monitor(args);
        const request = JSON.parse(args.core.getInput("CLIENT_PAYLOAD"));
        const target = request.registry_target;
        const source =
          request.mode === "copy" ?
            `ghcr.io/radius-project/bicep-types-${target}@${request.digest}`
          : `biceptypes.azurecr.io/${target}@${request.digest}`;
        const destination = `biceptypes.azurecr.io/${target}:${
          request.mode === "copy" ? version.slice(1) : request.channel
        }`;
        // This is the private transport test double, not public workflow code.
        await tool([
          "oras",
          "cp",
          "--from-plain-http",
          "--to-plain-http",
          map(source),
          map(destination)
        ]);
      };
      const stable = !version.includes("-rc.");
      await r.finalize(String(stable), String(stable), { monitor, run });
      await r.finalize(String(stable), String(stable), { monitor, run });
      assert.equal(copies.length, stable ? 4 : 0);
      const receipt = JSON.parse(
        await readFile(join(r.directory, "receipt.json"))
      );
      assert.equal(receipt.status, "complete");
      for (const entry of receipt.copies) {
        assert.equal(
          hash(await run(["oras", "manifest", "fetch", entry.destination])),
          entry.request.digest
        );
        const copied = JSON.parse(
          await run(["oras", "manifest", "fetch", entry.destination])
        );
        for (const blob of [copied.config, ...copied.layers]) {
          const bytes = await tool([
            "oras",
            "blob",
            "fetch",
            "--plain-http",
            "--output",
            "-",
            `${map(entry.destination).split(":").slice(0, -1).join(":")}@${blob.digest}`
          ]);
          assert.equal(hash(bytes), blob.digest);
        }
      }
      for (const entry of receipt.ghcr.destinations) {
        assert.equal(
          hash(await run(["oras", "manifest", "fetch", entry.reference])),
          entry.digest
        );
      }
    }
  }
);
