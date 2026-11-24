// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import script, {
  checkSummary,
  prepare,
  publish,
  publishGHCR,
  shouldPublish,
  selectSnapshot,
  sourceIdentity,
  targets,
  validateReceipt,
  verifyArtifact
} from "./bicep-types.mjs";

const context = {
  repo: { owner: "radius-project", repo: "radius" },
  ref: "refs/heads/main",
  eventName: "push",
  sha: "a".repeat(40),
  runId: 123
};
const env = {
  GITHUB_REF_PROTECTED: "true",
  GITHUB_RUN_ATTEMPT: "1",
  GITHUB_WORKFLOW_REF:
    "radius-project/radius/.github/workflows/build-main.yaml@refs/heads/main",
  CHANGES_RESULT: "success",
  ONLY_CHANGED: "false"
};
const source = sourceIdentity(context, env);
const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const artifact = {
  id: 7,
  name: "bicep-types-radius-123.tar",
  digest: hash("archive"),
  size_in_bytes: 100,
  expired: false,
  workflow_run: { id: 123, head_branch: "main", head_sha: context.sha }
};
const readYaml = (file) =>
  JSON.parse(execFileSync("yq", ["-o=json", ".", file], { encoding: "utf8" }));

test("main publisher has no private Bicep App, dispatch or ACR authority", () => {
  const main = readYaml(".github/workflows/build-main.yaml");
  const reusable = readYaml(".github/workflows/__publish-bicep-types.yaml");
  const caller = main.jobs["build-and-push-bicep-types"];
  assert.equal(caller.uses, "./.github/workflows/__publish-bicep-types.yaml");
  assert.equal(caller.secrets, undefined);
  assert.equal(reusable.on.workflow_call.secrets, undefined);
  assert.doesNotMatch(
    JSON.stringify([caller, reusable]),
    /RADIUS_PUBLISHER_BOT_|create-github-app-token|radius-publisher|azurecr|MIRROR_|monitor-remote-workflow|id-token/
  );
  assert.equal(reusable.jobs.publish.environment, "publish-bicep");
  assert.deepEqual(reusable.jobs.bundle.permissions, {
    contents: "read",
    actions: "read"
  });
  assert.equal(reusable.jobs.publish.permissions.packages, "write");
  assert.equal(
    reusable.jobs.publish.outputs.status,
    "${{ steps.publish.outputs.status }}"
  );
  assert.equal(main.concurrency.queue, "max");
  assert.notEqual(main.concurrency["cancel-in-progress"], true);
});

function receiptFixture(publicationAttempt = 1) {
  return {
    source: { ...source },
    artifact: { id: artifact.id, digest: artifact.digest },
    manifestDigest: hash("uploaded manifest"),
    publicationAttempt,
    status: "failed",
    ghcr: { reference: targets.ghcr }
  };
}

test("receipt binds original generation and rejects historical mirror receipts without rewriting them", () => {
  validateReceipt(receiptFixture(2));
  for (const mutate of [
    (r) => {
      r.source.repository = "other/radius";
    },
    (r) => {
      r.source.ref = "refs/tags/v1.0.0";
    },
    (r) => {
      r.source.workflow = ".github/workflows/other.yaml";
    },
    (r) => {
      r.source.commit = "main";
    },
    (r) => {
      r.source.commit = "0".repeat(40);
    },
    (r) => {
      r.source.runId = 0;
    },
    (r) => {
      r.source.generationAttempt = 3;
    },
    (r) => {
      r.artifact.id = Number.MAX_SAFE_INTEGER + 1;
    },
    (r) => {
      r.artifact.digest = "sha256:wrong";
    },
    (r) => {
      r.manifestDigest = "sha256:wrong";
    },
    (r) => {
      r.ghcr.reference = "ghcr.io/other/repo:edge";
    },
    (r) => {
      r.acr = { reference: "biceptypes.azurecr.io/radius:latest" };
    },
    (r) => {
      r.mirror = { run_id: "456" };
    }
  ]) {
    const invalid = receiptFixture(2);
    mutate(invalid);
    const before = JSON.stringify(invalid);
    assert.throws(() => validateReceipt(invalid));
    assert.equal(JSON.stringify(invalid), before);
  }
});

test("eligible main publishes; only successful docs-only or ineligible contexts may skip", () => {
  for (const eventName of ["push", "workflow_dispatch"])
    assert.equal(shouldPublish({ ...context, eventName }, env), true);
  for (const override of [
    { ref: "refs/tags/v1.0.0" },
    { eventName: "pull_request" },
    { eventName: "pull_request_target" },
    { eventName: "merge_group" },
    { eventName: "workflow_dispatch", ref: "refs/heads/topic" },
    { repo: { owner: "fork", repo: "radius" } }
  ])
    assert.equal(shouldPublish({ ...context, ...override }, env), false);
  assert.equal(shouldPublish(context, { ...env, ONLY_CHANGED: "true" }), false);
  for (const override of [
    { CHANGES_RESULT: "failure" },
    { CHANGES_RESULT: "cancelled" },
    { CHANGES_RESULT: "skipped" },
    { CHANGES_RESULT: "" },
    { ONLY_CHANGED: "" },
    { ONLY_CHANGED: "yes" },
    { ONLY_CHANGED: "FALSE" },
    { GITHUB_REF_PROTECTED: "false" }
  ])
    assert.throws(() => shouldPublish(context, { ...env, ...override }));
});

test("source and artifact identities reject substituted or unprotected invocations", () => {
  for (const override of [
    { GITHUB_REF_PROTECTED: "false" },
    { GITHUB_WORKFLOW_REF: "other-workflow" },
    { GITHUB_RUN_ATTEMPT: "0" },
    { ONLY_CHANGED: undefined },
    { ONLY_CHANGED: "FALSE" }
  ])
    assert.throws(() => sourceIdentity(context, { ...env, ...override }));
  for (const override of [
    { sha: "main" },
    { ref: "refs/pull/1/merge" },
    { eventName: "pull_request" }
  ])
    assert.throws(() => sourceIdentity({ ...context, ...override }, env));
  verifyArtifact(artifact, source);
  for (const override of [
    { name: "other-bundle" },
    { expired: true },
    { digest: "" },
    { workflow_run: { ...artifact.workflow_run, id: 999 } },
    { workflow_run: { ...artifact.workflow_run, head_sha: "b".repeat(40) } }
  ])
    assert.throws(() => verifyArtifact({ ...artifact, ...override }, source));
});

test("snapshot retries reuse one immutable artifact and never regenerate missing content", () => {
  assert.equal(selectSnapshot([], source), "");
  assert.equal(
    selectSnapshot([artifact], { ...source, generationAttempt: 2 }),
    7
  );
  assert.throws(
    () => selectSnapshot([], { ...source, generationAttempt: 2 }),
    /instead of regenerating/
  );
  assert.throws(
    () => selectSnapshot([artifact, artifact], source),
    /Ambiguous/
  );
});

test("API failure cannot be interpreted as an absent snapshot", async (t) => {
  const previous = { ...process.env };
  Object.assign(process.env, env);
  t.after(() => {
    for (const key of Object.keys(env)) {
      if (previous[key] === undefined) delete process.env[key];
      else process.env[key] = previous[key];
    }
  });
  const failures = [];
  await script({
    context,
    core: {
      getInput: () => "select",
      setFailed: (message) => failures.push(message)
    },
    github: {
      rest: {
        actions: {
          getWorkflowRun: async () => {
            throw new Error("HTTP 403");
          }
        }
      }
    }
  });
  assert.deepEqual(failures, ["HTTP 403"]);
});

async function snapshot(t, mutate = () => {}) {
  const directory = await mkdtemp(join(tmpdir(), "bicep-contract-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const blob = (data, mediaType) => ({
    mediaType,
    digest: hash(data),
    size: data.length
  });
  const prefix = "application/vnd.ms.bicep.provider.";
  const manifest = {
    schemaVersion: 2,
    mediaType: "application/vnd.oci.image.manifest.v1+json",
    artifactType: `${prefix}artifact`,
    config: blob("{}", `${prefix}config.v1+json`),
    layers: [blob("native Bicep types archive", `${prefix}layer.v1.tar+gzip`)],
    annotations: { "bicep.serialization.format": "v1" }
  };
  mutate(manifest);
  const metadata = { source, manifestDigest: hash(JSON.stringify(manifest)) };
  const run = async (args) => {
    if (args[0] === "tar") {
      assert.deepEqual(args, ["tar", "-xOf", "snapshot.tar", "source.json"]);
      return JSON.stringify(metadata);
    }
    if (args[2] === "fetch") return JSON.stringify(manifest);
    if (args[2] === "fetch-config") return "{}";
    return "";
  };
  return { directory, manifest, metadata, run };
}

test("preparation preserves native bytes and original source across retries", async (t) => {
  const first = await snapshot(t);
  const retry = await snapshot(t);
  const receipt = await prepare(
    "snapshot.tar",
    source,
    first.directory,
    first.run
  );
  assert.equal(
    receipt.manifestDigest,
    (
      await prepare(
        "snapshot.tar",
        { ...source, generationAttempt: 2 },
        retry.directory,
        retry.run
      )
    ).manifestDigest
  );
  const final = JSON.parse(
    await readFile(join(first.directory, "manifest.json"))
  );
  assert.deepEqual(final.config, first.manifest.config);
  assert.deepEqual(final.layers, first.manifest.layers);
  assert.equal(
    final.annotations["org.opencontainers.image.revision"],
    source.commit
  );
  assert.equal(receipt.acr, undefined);
});

test("preparation rejects executable content, foreign URLs, and substituted source/digest", async (t) => {
  for (const mutate of [
    (m) => m.layers.push(m.layers[0]),
    (m) => {
      m.layers[0].mediaType = "application/executable";
    },
    (m) => {
      m.config.urls = ["https://other.invalid/blob"];
    },
    (m) => {
      m.config.mediaType = "application/unknown";
    },
    (m) => {
      m.subject = { urls: ["https://other.invalid/subject"] };
    }
  ]) {
    const { directory, run } = await snapshot(t, mutate);
    await assert.rejects(prepare("snapshot.tar", source, directory, run));
  }
  const { directory, metadata, run } = await snapshot(t);
  await assert.rejects(
    prepare(
      "snapshot.tar",
      { ...source, commit: "b".repeat(40) },
      directory,
      run
    ),
    /source mismatch/
  );
  await assert.rejects(
    prepare("snapshot.tar", source, directory, (args) =>
      args[2] === "fetch-config" ? '{"localDeployEnabled":true}' : run(args)
    ),
    /Executable provider config/
  );
  metadata.manifestDigest = hash("other manifest");
  await assert.rejects(
    prepare("snapshot.tar", source, directory, run),
    /Manifest digest mismatch/
  );
});

test("stale initial work skips but a stale retry never hides previous writes", async () => {
  const receipt = { source, publicationAttempt: 1 };
  await publishGHCR(receipt, "/layout", "b".repeat(40), async () =>
    assert.fail("No writes")
  );
  assert.equal(receipt.status, "superseded");
  await assert.rejects(
    publishGHCR({ source, publicationAttempt: 2 }, "/layout", "b".repeat(40)),
    /prior publication may be partial/
  );
});

test("summary requires public publication or a proven no-write skip", () => {
  const expected = {
    ...env,
    BICEP_PUBLISH_RESULT: "success",
    BICEP_PUBLICATION_STATUS: "published"
  };
  checkSummary(context, expected);
  checkSummary(context, {
    ...expected,
    BICEP_PUBLICATION_STATUS: "superseded"
  });
  checkSummary(context, {
    ...env,
    ONLY_CHANGED: "true",
    BICEP_PUBLISH_RESULT: "skipped"
  });
  checkSummary(
    { ...context, eventName: "pull_request" },
    { BICEP_PUBLISH_RESULT: "skipped" }
  );
  for (const override of [
    { CHANGES_RESULT: "failure" },
    { ONLY_CHANGED: "" },
    { ONLY_CHANGED: "true" },
    { BICEP_PUBLISH_RESULT: "skipped" },
    { BICEP_PUBLISH_RESULT: "failure" },
    { BICEP_PUBLISH_RESULT: "cancelled" },
    { BICEP_PUBLISH_RESULT: "" },
    { BICEP_PUBLICATION_STATUS: "partial" },
    { BICEP_PUBLICATION_STATUS: "pending-mirror" },
    { BICEP_PUBLICATION_STATUS: "uploaded" },
    { BICEP_PUBLICATION_STATUS: "" }
  ])
    assert.throws(() => checkSummary(context, { ...expected, ...override }));
});

test("GHCR-only publication requires uploaded digest, public visibility and anonymous exact bytes", async (t) => {
  for (const outcome of [
    "public",
    "private",
    "internal",
    "API failure",
    "anonymous failure",
    "upload failure",
    "digest failure",
    "anonymous digest failure",
    "edge moved",
    "edge removed",
    "summary failure",
    "stale"
  ])
    await t.test(outcome, async (t) => {
      const directory = await mkdtemp(join(tmpdir(), "bicep-bootstrap-"));
      t.after(() => rm(directory, { recursive: true, force: true }));
      await writeFile(
        join(directory, "receipt.json"),
        JSON.stringify(receiptFixture())
      );
      const calls = [],
        outputs = {};
      let failing = true;
      const github = {
        rest: {
          repos: {
            getBranch: async () => ({
              data: {
                protected: true,
                commit: {
                  sha: outcome === "stale" ? "b".repeat(40) : source.commit
                }
              }
            })
          },
          packages: {
            getPackageForOrganization: async () => {
              assert.deepEqual(calls.slice(-2), ["upload", "digest"]);
              calls.push("visibility");
              if (failing && outcome === "API failure")
                throw new Error("HTTP 403");
              return {
                data: {
                  visibility:
                    failing && ["private", "internal"].includes(outcome) ?
                      outcome
                    : "public"
                }
              };
            }
          }
        }
      };
      const core = {
        setOutput: (name, value) => {
          outputs[name] = value;
        },
        summary: {
          addCodeBlock: () => ({
            write: async () => {
              if (failing && outcome === "summary failure")
                throw new Error("summary failed");
            }
          })
        }
      };
      const uploads = [];
      const run = async (args) => {
        assert.ok(args.at(-1).startsWith("ghcr.io/"), "No ACR requests");
        if (args[1] === "cp") {
          calls.push("upload");
          uploads.push(args[3]);
          if (failing && outcome === "upload failure")
            throw new Error("upload failed");
          return "";
        }
        if (args.includes("--registry-config")) {
          assert.deepEqual(JSON.parse(await readFile(args[4], "utf8")), {
            auths: {}
          });
          const tagRead = args.at(-1) === targets.ghcr;
          calls.push(tagRead ? "anonymous edge" : "anonymous digest");
          if (failing && tagRead && outcome === "edge moved")
            return "different manifest";
          if (failing && tagRead && outcome === "edge removed")
            throw new Error("edge not found");
          if (failing && outcome === "anonymous failure")
            throw new Error("anonymous denied");
          if (failing && outcome === "anonymous digest failure")
            return "uploaded manifest\n";
        } else {
          calls.push("digest");
          if (failing && outcome === "digest failure") return "wrong";
        }
        return "uploaded manifest";
      };
      if (["public", "stale"].includes(outcome)) {
        await publish({ github, core }, source, directory, run);
        assert.equal(
          outputs.status,
          outcome === "stale" ? "superseded" : "published"
        );
        assert.equal(outputs.manifest_digest, receiptFixture().manifestDigest);
        if (outcome === "stale") assert.deepEqual(calls, []);
        else
          assert.deepEqual(calls, [
            "upload",
            "digest",
            "visibility",
            "anonymous digest",
            "anonymous edge"
          ]);
      } else {
        await assert.rejects(publish({ github, core }, source, directory, run));
        assert.equal(outputs.status, undefined);
        const partial = JSON.parse(
          await readFile(join(directory, "receipt.json"))
        );
        if (["edge moved", "edge removed"].includes(outcome)) {
          assert.deepEqual(calls.slice(-2), [
            "anonymous digest",
            "anonymous edge"
          ]);
          assert.equal(partial.ghcr.digest, receiptFixture().manifestDigest);
        }
        assert.equal(
          partial.status,
          ["upload failure", "digest failure"].includes(outcome) ? "partial"
          : outcome === "summary failure" ? "published"
          : "uploaded"
        );
        failing = false;
        await writeFile(
          join(directory, "receipt.json"),
          JSON.stringify({ ...partial, publicationAttempt: 2 })
        );
        await publish(
          { github, core },
          { ...source, generationAttempt: 2 },
          directory,
          run
        );
        assert.equal(outputs.status, "published");
        assert.equal(uploads[0], uploads[1]);
      }
      const receipt = JSON.parse(
        await readFile(join(directory, "receipt.json"))
      );
      assert.deepEqual(receipt.source, source);
      assert.equal(receipt.acr, undefined);
      assert.equal(receipt.mirror, undefined);
      assert.equal(outputs.client_payload, undefined);
    });
});
