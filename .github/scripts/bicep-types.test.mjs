// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import script, {
  checkSummary,
  prepare,
  publish,
  publishPair,
  shouldPublish,
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

test("queued main explicitly forwards only the required Azure IDs to trusted publishing", async () => {
  const main = await readFile(
    new URL("../workflows/build-main.yaml", import.meta.url),
    "utf8"
  );
  const reusable = await readFile(
    new URL("../workflows/__publish-bicep-types.yaml", import.meta.url),
    "utf8"
  );
  const caller = main
    .split(/^  build-and-push-bicep-types:\s*$/m)[1]
    .split(/^  [\w-]+:\s*$/m)[0];
  const [contract, generation, publishing] = reusable.split(
    /^  (?:bundle|publish):\s*$/m
  );
  const names = [
    "BICEPTYPES_CLIENT_ID",
    "BICEPTYPES_TENANT_ID",
    "BICEPTYPES_SUBSCRIPTION_ID"
  ];
  assert.deepEqual(
    [
      ...caller.matchAll(
        /^\s+(BICEPTYPES_[A-Z_]+):\s*\$\{\{ secrets\.(BICEPTYPES_[A-Z_]+) \}\}/gm
      )
    ].map(([, input, secret]) => [input, secret]),
    names.map((name) => [name, name])
  );
  for (const name of names) {
    assert.match(contract, new RegExp(`${name}:\\s+required: true`));
    assert.ok(publishing.includes(`secrets.${name}`));
  }
  assert.doesNotMatch(caller, /secrets:\s*inherit/);
  assert.doesNotMatch(
    generation,
    /secrets\.|packages:\s*write|id-token:\s*write/
  );
  assert.match(main, /group:\s*build-main-\$\{\{ github\.ref \}\}/);
  assert.match(main, /queue:\s*max/);
  assert.doesNotMatch(main, /cancel-in-progress:\s*true/);
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
  ]) {
    assert.equal(shouldPublish({ ...context, ...override }, env), false);
  }
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
  ]) {
    assert.throws(() => shouldPublish(context, { ...env, ...override }));
  }
});

test("source and artifact identities cannot substitute a PR, other run, commit, or unprotected workflow", () => {
  for (const override of [
    { GITHUB_REF_PROTECTED: "false" },
    { GITHUB_WORKFLOW_REF: "other-workflow" },
    { GITHUB_RUN_ATTEMPT: "0" },
    { ONLY_CHANGED: undefined },
    { ONLY_CHANGED: "FALSE" }
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

test("summary requires a public publication result or a proven no-write skip", () => {
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
    {
      CHANGES_RESULT: "cancelled",
      ONLY_CHANGED: "true",
      BICEP_PUBLISH_RESULT: "skipped"
    },
    { ONLY_CHANGED: "" },
    { ONLY_CHANGED: "true" },
    { BICEP_PUBLISH_RESULT: "skipped" },
    { BICEP_PUBLISH_RESULT: "failure" },
    { BICEP_PUBLISH_RESULT: "cancelled" },
    { BICEP_PUBLISH_RESULT: "" },
    { BICEP_PUBLICATION_STATUS: "partial" },
    { BICEP_PUBLICATION_STATUS: "uploaded" },
    { BICEP_PUBLICATION_STATUS: "" }
  ]) {
    assert.throws(() => checkSummary(context, { ...expected, ...override }));
  }
});

test("bootstrap uploads before visibility lookup and preserves post-upload failures in the receipt", async (t) => {
  for (const outcome of [
    "public",
    "private",
    "internal",
    "API failure",
    "mirror failure",
    "digest failure",
    "stale"
  ]) {
    await t.test(outcome, async (t) => {
      const directory = await mkdtemp(join(tmpdir(), "bicep-bootstrap-"));
      t.after(() => rm(directory, { recursive: true, force: true }));
      const raw = "uploaded manifest",
        digest = hash(raw);
      await writeFile(
        join(directory, "receipt.json"),
        JSON.stringify({
          source,
          manifestDigest: digest,
          publicationAttempt: 1,
          status: "failed",
          ghcr: { reference: targets.ghcr },
          acr: { reference: targets.acr }
        })
      );
      const outputs = {},
        calls = [];
      let packageExists = false,
        summary;
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
              assert.ok(
                packageExists,
                "A missing package must first be created by the upload"
              );
              assert.deepEqual(calls, [
                "upload",
                "verify GHCR",
                "mirror",
                "verify ACR",
                "verify GHCR"
              ]);
              calls.push("visibility");
              if (outcome === "API failure") throw new Error("HTTP 403");
              return { data: { visibility: outcome } };
            }
          }
        }
      };
      const core = {
        setOutput: (key, value) => {
          outputs[key] = value;
        },
        summary: {
          addCodeBlock: (value) => {
            summary = JSON.parse(value);
            return { write: async () => {} };
          }
        }
      };
      const run = async (args) => {
        if (args[1] === "cp") {
          if (args.at(-1) === targets.ghcr) {
            calls.push("upload");
            packageExists = true;
            assert.deepEqual(args, [
              "oras",
              "cp",
              "--from-oci-layout",
              `${directory}/layout@${digest}`,
              targets.ghcr
            ]);
          } else {
            calls.push("mirror");
            assert.deepEqual(args, [
              "oras",
              "cp",
              `${targets.ghcr.split(":")[0]}@${digest}`,
              targets.acr
            ]);
            if (outcome === "mirror failure")
              throw new Error("ACR unavailable");
          }
          return "";
        }
        calls.push(args.at(-1) === targets.ghcr ? "verify GHCR" : "verify ACR");
        return outcome === "digest failure" ? "wrong digest" : raw;
      };
      let failure;
      try {
        await publish({ github, core }, source, directory, run);
      } catch (error) {
        failure = error.message;
      }
      const receipt = JSON.parse(
        await readFile(join(directory, "receipt.json"), "utf8")
      );
      assert.deepEqual(summary, receipt);
      if (outcome === "stale") {
        assert.deepEqual(calls, []);
        assert.equal(failure, undefined);
        assert.equal(outputs.status, "superseded");
      } else if (outcome.endsWith("failure") && outcome !== "API failure") {
        assert.match(
          failure,
          outcome === "mirror failure" ? /ACR unavailable/ : /Digest mismatch/
        );
        assert.equal(receipt.status, "partial");
        assert.equal(
          receipt.ghcr.digest,
          outcome === "mirror failure" ? digest : undefined
        );
        assert.equal(receipt.acr.digest, undefined);
        assert.equal(outputs.status, undefined);
        assert.ok(!calls.includes("visibility"));
      } else {
        assert.equal(receipt.ghcr.digest, digest);
        assert.equal(receipt.acr.digest, digest);
        if (outcome === "public") {
          assert.equal(failure, undefined);
          assert.equal(outputs.status, "published");
          assert.equal(receipt.status, "published");
        } else {
          assert.equal(receipt.status, "uploaded");
          assert.equal(
            receipt.visibility,
            outcome === "API failure" ? "unknown" : outcome
          );
          assert.equal(outputs.status, undefined);
          assert.match(failure, /artifact was uploaded and mirrored/);
          assert.match(
            failure,
            /Packages -> bicep-types-radius -> Package settings -> Change visibility -> Public/
          );
          assert.match(failure, /rerun and verify anonymous restore/);
          if (outcome === "API failure") assert.match(failure, /HTTP 403/);
        }
      }
    });
  }
});
