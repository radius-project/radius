// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import script, {
  checkSummary,
  prepare,
  publishPair,
  route,
  selectSnapshot,
  sourceIdentity,
  targets,
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
  ONLY_CHANGED: "false",
  PUBLISH_ENABLED: "true"
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

test("routing is default-off, main-only, and requires explicit successful change detection", () => {
  for (const flag of [undefined, "", "false", "TRUE", "true ", "invalid"]) {
    assert.equal(route(context, { ...env, PUBLISH_ENABLED: flag }), "legacy");
  }
  for (const eventName of ["push", "workflow_dispatch"])
    assert.equal(route({ ...context, eventName }, env), "direct");
  for (const override of [
    { ref: "refs/tags/v1.0.0" },
    { eventName: "pull_request" },
    { eventName: "pull_request_target" },
    { eventName: "merge_group" },
    { repo: { owner: "fork", repo: "radius" } }
  ]) {
    assert.equal(route({ ...context, ...override }, env), "skip");
  }
  assert.equal(route(context, { ...env, ONLY_CHANGED: "true" }), "skip");
  for (const override of [
    { CHANGES_RESULT: "failure" },
    { CHANGES_RESULT: "skipped" },
    { ONLY_CHANGED: "" },
    { ONLY_CHANGED: "yes" }
  ]) {
    assert.throws(() => route(context, { ...env, ...override }));
  }
});

test("source and artifact identities cannot substitute a PR, other run, commit, or unprotected workflow", () => {
  for (const override of [
    { GITHUB_REF_PROTECTED: "false" },
    { GITHUB_WORKFLOW_REF: "other-workflow" },
    { GITHUB_RUN_ATTEMPT: "0" }
  ]) {
    assert.throws(() => sourceIdentity(context, { ...env, ...override }));
  }
  for (const override of [
    { sha: "main" },
    { ref: "refs/pull/1/merge" },
    { eventName: "pull_request" }
  ]) {
    assert.throws(() => sourceIdentity({ ...context, ...override }, env));
  }
  verifyArtifact(artifact, source);
  for (const override of [
    { name: "other-bundle" },
    { expired: true },
    { digest: "" },
    { workflow_run: { ...artifact.workflow_run, id: 999 } },
    { workflow_run: { ...artifact.workflow_run, head_sha: "b".repeat(40) } }
  ]) {
    assert.throws(() => verifyArtifact({ ...artifact, ...override }, source));
  }
});

test("snapshot retries reuse one immutable artifact and never silently regenerate missing content", () => {
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

test("an API failure is surfaced rather than interpreted as an absent snapshot", async (t) => {
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

test("preparation preserves Bicep content and deterministically stamps only trusted source identity", async (t) => {
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
});

test("preparation rejects executable Bicep content, foreign URLs, and source/digest substitution", async (t) => {
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

test("the serialized pair copies the captured digest to fixed destinations and reports partial failures", async () => {
  const raw = "final manifest bytes";
  const fresh = () => ({
    source,
    manifestDigest: hash(raw),
    publicationAttempt: 1,
    status: "failed",
    ghcr: {},
    acr: {}
  });
  const receipt = fresh(),
    calls = [];
  const run = async (args) => {
    calls.push(args);
    return args[1] === "manifest" ? raw : "";
  };
  await publishPair(receipt, "/layout", source.commit, run);
  assert.equal(receipt.status, "published");
  assert.deepEqual(
    calls.filter((args) => args[1] === "cp"),
    [
      ["oras", "cp", "--from-oci-layout", `/layout@${hash(raw)}`, targets.ghcr],
      [
        "oras",
        "cp",
        `ghcr.io/radius-project/bicep-types-radius@${hash(raw)}`,
        targets.acr
      ]
    ]
  );
  for (const failure of ["mirror", "digest"]) {
    const partial = fresh();
    await assert.rejects(
      publishPair(partial, "/layout", source.commit, async (args) => {
        if (failure === "mirror" && args.at(-1) === targets.acr)
          throw new Error("ACR unavailable");
        return failure === "digest" ? "wrong digest" : run(args);
      })
    );
    assert.equal(partial.status, "partial");
  }
});

test("stale initial work can skip, but a stale retry never hides unknown previous writes", async () => {
  const receipt = { source, publicationAttempt: 1 };
  await publishPair(receipt, "/layout", "b".repeat(40), async () =>
    assert.fail("No writes")
  );
  assert.equal(receipt.status, "superseded");
  await assert.rejects(
    publishPair({ source, publicationAttempt: 2 }, "/layout", "b".repeat(40)),
    /prior publication may be partial/
  );
});

test("summary accepts only the selected successful publisher or a proven skip", () => {
  const expected = {
    BICEP_MODE_RESULT: "success",
    BICEP_MODE: "direct",
    BICEP_LEGACY_RESULT: "skipped",
    BICEP_DIRECT_RESULT: "success",
    BICEP_PUBLICATION_STATUS: "published"
  };
  checkSummary(expected);
  for (const override of [
    { BICEP_MODE_RESULT: "failure" },
    { BICEP_MODE: "" },
    { BICEP_DIRECT_RESULT: "skipped" },
    { BICEP_LEGACY_RESULT: "success" },
    { BICEP_PUBLICATION_STATUS: "partial" }
  ]) {
    assert.throws(() => checkSummary({ ...expected, ...override }));
  }
});
