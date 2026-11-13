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
  complete,
  copyRequest,
  prepare,
  publish,
  publishGHCR,
  shouldPublish,
  selectSnapshot,
  sourceIdentity,
  targets,
  tool,
  verifyArtifact
} from "./bicep-types.mjs";
import monitorRemoteWorkflow from "./monitor-remote-workflow.mjs";

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
const mainWorkflow = readYaml(".github/workflows/build-main.yaml");
const publisherWorkflow = readYaml(
  ".github/workflows/__publish-bicep-types.yaml"
);
const publisherSecrets = {
  "client-id": "RADIUS_PUBLISHER_BOT_CLIENT_ID",
  "private-key": "RADIUS_PUBLISHER_BOT_PRIVATE_KEY"
};

function checkPublisherSecrets(main, reusable) {
  const caller = main.jobs["build-and-push-bicep-types"];
  assert.equal(caller.uses, "./.github/workflows/__publish-bicep-types.yaml");
  const names = Object.values(publisherSecrets).sort();
  assert.deepEqual(Object.keys(caller.secrets ?? {}).sort(), names);
  const declarations = reusable.on.workflow_call.secrets;
  assert.deepEqual(Object.keys(declarations ?? {}).sort(), names);
  const token = reusable.jobs.publish.steps.find(
    (step) => step.id === "publisher-token"
  );
  assert.match(token.uses, /^actions\/create-github-app-token@/);
  for (const [input, name] of Object.entries(publisherSecrets)) {
    assert.equal(caller.secrets[name], `\${{ secrets.${name} }}`);
    assert.equal(declarations[name].required, true);
    assert.equal(token.with[input], `\${{ secrets.${name} }}`);
  }
  assert.equal(reusable.jobs.publish.environment, "publish-bicep");
  assert.deepEqual(reusable.jobs.bundle.permissions, {
    contents: "read",
    actions: "read"
  });
  const runtime = structuredClone(reusable);
  delete runtime.on;
  runtime.jobs.publish.steps = runtime.jobs.publish.steps.filter(
    (step) => step.id !== "publisher-token"
  );
  assert.doesNotMatch(
    JSON.stringify(runtime),
    /\bsecrets\b|RADIUS_PUBLISHER_BOT_|actions\/create-github-app-token@/
  );
}

test("publisher App secrets are explicitly forwarded, required and consumed only by the publishing token step", () => {
  checkPublisherSecrets(mainWorkflow, publisherWorkflow);
});

test("publisher secret contract rejects missing wiring and broader credential use", async (t) => {
  const mutations = [
    [
      "broad inheritance",
      (caller) => {
        caller.secrets = "inherit";
      }
    ],
    [
      "generation consumes secrets",
      (_caller, reusable) => {
        reusable.jobs.bundle.env = {
          APP_KEY: "${{ secrets.RADIUS_PUBLISHER_BOT_PRIVATE_KEY }}"
        };
      }
    ]
  ];
  for (const name of Object.values(publisherSecrets)) {
    mutations.push(
      [
        `missing forwarding: ${name}`,
        (caller) => {
          delete caller.secrets[name];
        }
      ],
      [
        `wrong source: ${name}`,
        (caller) => {
          caller.secrets[name] = "${{ secrets.OTHER }}";
        }
      ],
      [
        `missing declaration: ${name}`,
        (_caller, reusable) => {
          delete reusable.on.workflow_call.secrets[name];
        }
      ],
      [
        `optional declaration: ${name}`,
        (_caller, reusable) => {
          reusable.on.workflow_call.secrets[name].required = false;
        }
      ]
    );
  }
  for (const [name, mutate] of mutations) {
    await t.test(name, () => {
      const main = structuredClone(mainWorkflow);
      const reusable = structuredClone(publisherWorkflow);
      mutate(main.jobs["build-and-push-bicep-types"], reusable);
      assert.throws(
        () => checkPublisherSecrets(main, reusable),
        assert.AssertionError
      );
    });
  }
});

test("queued main isolates the private App and has no production Azure authority", async () => {
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
  const [, generation, publishing] = reusable.split(
    /^  (?:bundle|publish):\s*$/m
  );
  assert.doesNotMatch(
    caller + reusable,
    /BICEPTYPES_|id-token:|az acr|az account|azure-oidc-login/
  );
  assert.doesNotMatch(
    generation,
    /secrets\.|packages:\s*write|id-token:\s*write/
  );
  assert.match(generation, /uses: \.\/\.github\/actions\/go-mod-download/);
  assert.match(publishing, /environment: publish-bicep/);
  for (const name of [
    "RADIUS_PUBLISHER_BOT_CLIENT_ID",
    "RADIUS_PUBLISHER_BOT_PRIVATE_KEY"
  ]) {
    assert.ok(publishing.includes(`secrets.${name}`));
  }
  assert.match(
    publishing,
    /owner: azure-octo\s+repositories: radius-publisher\s+permission-actions: read\s+permission-contents: write/
  );
  const ordered = [
    "Publish GHCR and verify public access",
    "Get private publisher App token",
    "Request and monitor the exact private ACR copy",
    "Verify the mirrored digest and record the result",
    "Retain publication receipt"
  ].map((name) => publishing.indexOf(`name: ${name}`));
  assert.ok(
    ordered.every(
      (index, i) => index >= 0 && (i === 0 || index > ordered[i - 1])
    )
  );
  const mirror = publishing
    .split("name: Request and monitor the exact private ACR copy")[1]
    .split("      - name:")[0];
  assert.match(
    mirror,
    /github-token: \$\{\{ steps\.publisher-token\.outputs\.token \}\}/
  );
  assert.match(mirror, /monitor-remote-workflow\.mjs/);
  assert.match(mirror, /INPUT_REUSE_SUCCESSFUL: "false"/);
  assert.match(mirror, /INPUT_WORKFLOW_FILE: publish-bicep-types\.yml/);
  assert.match(mirror, /INPUT_EVENT_TYPE: bicep-types/);
  assert.match(
    mirror,
    /if: steps\.publish\.outputs\.status == 'pending-mirror'/
  );
  assert.doesNotMatch(mirror, /retries:|bicep-types\.mjs/);
  assert.equal(
    (publishing.match(/github-token: \$\{\{ steps\.publisher-token/g) || [])
      .length,
    1
  );
  assert.match(
    publishing,
    /if: always\(\) && steps\.publish\.outcome == 'success'/
  );
  assert.match(
    publishing,
    /status: \$\{\{ steps\.complete\.outputs\.status \}\}/
  );
  assert.match(
    publishing,
    /MIRROR_RUN_ID: \$\{\{ steps\.mirror\.outputs\.run_id \}\}/
  );
  assert.match(main, /group:\s*build-main-\$\{\{ github\.ref \}\}/);
  assert.match(main, /queue:\s*max/);
  assert.doesNotMatch(main, /cancel-in-progress:\s*true/);
});

function receiptFixture(publicationAttempt = 1) {
  return {
    source: { ...source },
    artifact: { id: artifact.id, digest: artifact.digest },
    manifestDigest: hash("uploaded manifest"),
    publicationAttempt,
    status: "failed",
    ghcr: { reference: targets.ghcr },
    acr: { reference: targets.acr }
  };
}

test("copy contract binds the digest to original generation evidence across publication retries", () => {
  const receipt = receiptFixture(2);
  const payload = copyRequest(receipt);
  assert.deepEqual(payload, {
    mode: "copy",
    source_repository: "radius-project/radius",
    source_ref: "refs/heads/main",
    source_sha: context.sha,
    registry_target: "radius",
    rel_channel: "edge",
    digest: receipt.manifestDigest,
    release_identifier: `copy-radius-edge-${context.sha}-123-1-${receipt.manifestDigest.slice(7)}`,
    source_evidence: {
      run_id: 123,
      run_attempt: 1,
      artifact_id: 7,
      artifact_digest: artifact.digest
    }
  });
  assert.deepEqual(copyRequest(receiptFixture(3)), payload);
  for (const mutate of [
    (r) => {
      r.source.runId++;
    },
    (r) => {
      r.source.generationAttempt++;
    },
    (r) => {
      r.source.commit = "b".repeat(40);
    },
    (r) => {
      r.manifestDigest = hash("different native bytes");
    }
  ]) {
    const changed = receiptFixture(2);
    mutate(changed);
    assert.notEqual(
      copyRequest(changed).release_identifier,
      payload.release_identifier
    );
  }
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
      r.acr.reference = "biceptypes.azurecr.io/aws:latest";
    }
  ]) {
    const invalid = receiptFixture(2);
    mutate(invalid);
    assert.throws(() => copyRequest(invalid));
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
  await publishGHCR(receipt, "/layout", "b".repeat(40), async () =>
    assert.fail("No writes")
  );
  assert.equal(receipt.status, "superseded");
  await assert.rejects(
    publishGHCR({ source, publicationAttempt: 2 }, "/layout", "b".repeat(40)),
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
    { BICEP_PUBLICATION_STATUS: "pending-mirror" },
    { BICEP_PUBLICATION_STATUS: "uploaded" },
    { BICEP_PUBLICATION_STATUS: "" }
  ]) {
    assert.throws(() => checkSummary(context, { ...expected, ...override }));
  }
});

function mirrorEnvironment(receipt) {
  return {
    MIRROR_RESULT: "success",
    MIRROR_RELEASE_IDENTIFIER: copyRequest(receipt).release_identifier,
    MIRROR_RUN_STATE: "found",
    MIRROR_RUN_ID: "456",
    MIRROR_RUN_URL:
      "https://github.com/azure-octo/radius-publisher/actions/runs/456",
    MIRROR_CONCLUSION: "success"
  };
}

test("completion rejects missing or mismatched correlation before registry reads", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "bicep-completion-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const receipt = receiptFixture();
  receipt.status = "pending-mirror";
  receipt.visibility = "public";
  receipt.ghcr.digest = receipt.manifestDigest;
  for (const override of [
    { MIRROR_RESULT: "cancelled" },
    { MIRROR_RELEASE_IDENTIFIER: "" },
    { MIRROR_RELEASE_IDENTIFIER: "another-copy" },
    { MIRROR_RUN_STATE: "unknown" },
    { MIRROR_RUN_ID: "" },
    { MIRROR_RUN_ID: "0" },
    { MIRROR_RUN_ID: String(Number.MAX_SAFE_INTEGER + 1) },
    { MIRROR_RUN_URL: "https://github.com/other/repo/actions/runs/456" },
    { MIRROR_CONCLUSION: "" }
  ]) {
    await writeFile(join(directory, "receipt.json"), JSON.stringify(receipt));
    await assert.rejects(
      complete(
        {
          core: {
            setOutput: () => assert.fail("No publication output"),
            summary: { addCodeBlock: () => ({ write: async () => {} }) }
          }
        },
        source,
        directory,
        { ...mirrorEnvironment(receipt), ...override },
        async () =>
          assert.fail("No registry read before successful correlation")
      )
    );
    const retained = JSON.parse(
      await readFile(join(directory, "receipt.json"), "utf8")
    );
    assert.equal(retained.status, "pending-mirror");
    assert.equal(retained.acr.digest, undefined);
  }
});

test("private first upload retries with the same digest and original generation evidence", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "bicep-public-retry-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  await writeFile(
    join(directory, "receipt.json"),
    JSON.stringify(receiptFixture())
  );
  let visibility = "private";
  const uploads = [],
    payloads = [];
  const github = {
    rest: {
      repos: {
        getBranch: async () => ({
          data: { protected: true, commit: { sha: source.commit } }
        })
      },
      packages: {
        getPackageForOrganization: async () => ({ data: { visibility } })
      }
    }
  };
  const core = {
    setOutput: (name, value) => {
      if (name === "client_payload") payloads.push(JSON.parse(value));
    },
    summary: { addCodeBlock: () => ({ write: async () => {} }) }
  };
  const run = async (args) => {
    if (args[1] === "cp") {
      uploads.push(args[3]);
      assert.equal(args.at(-1), targets.ghcr);
      return "";
    }
    assert.deepEqual(args.slice(0, 3), ["oras", "manifest", "fetch"]);
    return "uploaded manifest";
  };
  await assert.rejects(
    publish({ github, core }, source, directory, run),
    /mirror has not been requested/
  );
  const first = JSON.parse(
    await readFile(join(directory, "receipt.json"), "utf8")
  );
  assert.equal(first.status, "pending-mirror");
  assert.deepEqual(payloads, []);
  visibility = "public";
  await writeFile(
    join(directory, "receipt.json"),
    JSON.stringify({ ...first, publicationAttempt: 2 })
  );
  await publish(
    { github, core },
    { ...source, generationAttempt: 2 },
    directory,
    run
  );
  assert.equal(uploads.length, 2);
  assert.equal(uploads[0], uploads[1]);
  assert.equal(payloads[0].digest, first.manifestDigest);
  assert.deepEqual(
    payloads[0].source_evidence,
    copyRequest(first).source_evidence
  );
  assert.equal(
    payloads[0].release_identifier,
    copyRequest(first).release_identifier
  );
});

test("native process readback hashes exact bytes and never writes an ACR target", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "bicep-copy-readback-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const receipt = receiptFixture();
  receipt.status = "pending-mirror";
  receipt.visibility = "public";
  receipt.ghcr.digest = receipt.manifestDigest;
  const stub = join(directory, "oras-stub.mjs");
  await writeFile(
    stub,
    `
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const [directory, ...args] = process.argv.slice(2);
assert.deepEqual(args.slice(0, 4), ["oras", "manifest", "fetch", "--registry-config"]);
assert.deepEqual(JSON.parse(readFileSync(args[4], "utf8")), { auths: {} });
const references = ${JSON.stringify([targets.acr, targets.ghcr])};
assert.ok(references.includes(args[5]));
process.stdout.write(readFileSync(directory + (args[5] === references[0] ? "/acr-manifest" : "/ghcr-manifest")));
`
  );
  await writeFile(join(directory, "ghcr-manifest"), "uploaded manifest");
  const outputs = {};
  const core = {
    setOutput: (name, value) => {
      outputs[name] = value;
    },
    summary: { addCodeBlock: () => ({ write: async () => {} }) }
  };
  const run = (args, options) =>
    tool([process.execPath, stub, directory, ...args], options);
  for (const bytes of ["uploaded manifest", "uploaded manifest\n"]) {
    await writeFile(join(directory, "receipt.json"), JSON.stringify(receipt));
    await writeFile(join(directory, "acr-manifest"), bytes);
    const promise = complete(
      { core },
      source,
      directory,
      mirrorEnvironment(receipt),
      run
    );
    if (bytes === "uploaded manifest") {
      await promise;
      assert.equal(outputs.status, "published");
    } else {
      delete outputs.status;
      await assert.rejects(promise, /Digest mismatch/);
      assert.equal(outputs.status, undefined);
    }
  }
});

test("GHCR bootstrap, correlated private copy and anonymous readback preserve honest receipts", async (t) => {
  for (const outcome of [
    "public",
    "publication retry",
    "active remote",
    "private",
    "internal",
    "API failure",
    "anonymous failure",
    "upload failure",
    "GHCR digest failure",
    "mirror failure",
    "mirror timeout",
    "uncertain dispatch",
    "App failure",
    "ACR digest failure",
    "GHCR moved",
    "upload summary failure",
    "final summary failure",
    "stale"
  ]) {
    await t.test(outcome, async (t) => {
      const directory = await mkdtemp(join(tmpdir(), "bicep-bootstrap-"));
      t.after(() => rm(directory, { recursive: true, force: true }));
      const raw = "uploaded manifest",
        digest = hash(raw);
      const currentSource = {
        ...source,
        generationAttempt: outcome === "publication retry" ? 2 : 1
      };
      await writeFile(
        join(directory, "receipt.json"),
        JSON.stringify(receiptFixture(currentSource.generationAttempt))
      );
      const uploadOutputs = {},
        outputs = {},
        calls = [];
      let packageExists = false,
        summary,
        phase = "upload";
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
              assert.deepEqual(calls, ["upload", "verify GHCR"]);
              const retained = JSON.parse(
                await readFile(join(directory, "receipt.json"), "utf8")
              );
              assert.equal(retained.ghcr.digest, digest);
              assert.equal(retained.status, "pending-mirror");
              calls.push("visibility");
              if (outcome === "API failure") throw new Error("HTTP 403");
              return {
                data: {
                  visibility:
                    ["private", "internal"].includes(outcome) ? outcome : (
                      "public"
                    )
                }
              };
            }
          }
        }
      };
      const core = {
        setOutput: (key, value) => {
          (phase === "upload" ? uploadOutputs : outputs)[key] = value;
        },
        summary: {
          addCodeBlock: (value) => {
            summary = JSON.parse(value);
            return {
              write: async () => {
                if (
                  (phase === "upload" &&
                    outcome === "upload summary failure") ||
                  (phase === "complete" && outcome === "final summary failure")
                )
                  throw new Error("summary write failed");
              }
            };
          }
        }
      };
      const run = async (args, options) => {
        if (args[1] === "cp") {
          calls.push("upload");
          packageExists = true;
          assert.deepEqual(args, [
            "oras",
            "cp",
            "--from-oci-layout",
            `${directory}/layout@${digest}`,
            targets.ghcr
          ]);
          if (outcome === "upload failure") throw new Error("GHCR unavailable");
          return "";
        }
        assert.deepEqual(args.slice(0, 3), ["oras", "manifest", "fetch"]);
        assert.equal(options.timeout, 60_000);
        const reference = args.at(-1);
        if (args.includes("--registry-config")) {
          assert.deepEqual(JSON.parse(await readFile(args[4], "utf8")), {
            auths: {}
          });
          assert.equal(args[4], join(directory, "anonymous-registry.json"));
          if (phase === "upload") {
            assert.equal(reference, `${targets.ghcr.split(":")[0]}@${digest}`);
            calls.push("anonymous GHCR");
            if (outcome === "anonymous failure")
              throw new Error("Anonymous pull denied");
          } else {
            calls.push(
              reference === targets.acr ? "verify ACR" : "recheck GHCR"
            );
          }
        } else {
          assert.equal(reference, targets.ghcr);
          calls.push("verify GHCR");
        }
        if (
          (outcome === "GHCR digest failure" &&
            !args.includes("--registry-config")) ||
          (outcome === "ACR digest failure" && reference === targets.acr) ||
          (outcome === "GHCR moved" &&
            phase === "complete" &&
            reference === targets.ghcr)
        )
          return "wrong digest";
        return raw;
      };
      let failure;
      try {
        await publish({ github, core }, currentSource, directory, run);
      } catch (error) {
        failure = error.message;
      }
      const remoteOutputs = {};
      let mirrorResult = "skipped",
        dispatches = 0;
      if (
        !failure &&
        uploadOutputs.status === "pending-mirror" &&
        outcome !== "App failure"
      ) {
        phase = "monitor";
        const payload = JSON.parse(uploadOutputs.client_payload);
        assert.equal(payload.source_evidence.run_attempt, 1);
        assert.equal(payload.source_evidence.artifact_digest, artifact.digest);
        const inputs = {
          OWNER: "azure-octo",
          REPO: "radius-publisher",
          WORKFLOW_FILE: "publish-bicep-types.yml",
          EVENT_TYPE: "bicep-types",
          RELEASE_IDENTIFIER: uploadOutputs.release_identifier,
          CLIENT_PAYLOAD: uploadOutputs.client_payload,
          REUSE_SUCCESSFUL: "false",
          MAX_WAIT_SECONDS: "3",
          POLL_INTERVAL_SECONDS: "1"
        };
        const remoteRun = {
          id: 456,
          event: "repository_dispatch",
          display_title: `bicep-types / ${payload.release_identifier}`,
          status: "completed",
          conclusion: outcome === "mirror failure" ? "failure" : "success",
          html_url:
            "https://github.com/azure-octo/radius-publisher/actions/runs/456"
        };
        const runs =
          outcome === "active remote" ?
            [{ ...remoteRun, status: "in_progress", conclusion: null }]
          : [];
        let clock = 0;
        const failures = [];
        await monitorRemoteWorkflow({
          core: {
            getInput: (name) => inputs[name] ?? "",
            setOutput: (name, value) => {
              remoteOutputs[name] = value;
            },
            setFailed: (message) => {
              failures.push(message);
            },
            info: () => {}
          },
          now: () => clock,
          sleep: async (ms) => {
            clock += ms;
          },
          github: {
            paginate: async () => runs,
            rest: {
              repos: {
                createDispatchEvent: async (request) => {
                  dispatches++;
                  calls.push("dispatch");
                  assert.equal(request.owner, "azure-octo");
                  assert.equal(request.repo, "radius-publisher");
                  assert.equal(request.event_type, "bicep-types");
                  assert.deepEqual(request.client_payload, payload);
                  const retained = JSON.parse(
                    await readFile(join(directory, "receipt.json"), "utf8")
                  );
                  assert.equal(retained.status, "pending-mirror");
                  assert.equal(retained.ghcr.digest, digest);
                  assert.equal(retained.visibility, "public");
                  assert.equal(retained.acr.digest, undefined);
                  if (outcome === "uncertain dispatch")
                    throw Object.assign(new Error("response lost"), {
                      status: 503
                    });
                  runs.push(
                    outcome === "mirror timeout" ?
                      { ...remoteRun, status: "in_progress", conclusion: null }
                    : remoteRun
                  );
                }
              },
              actions: {
                listWorkflowRuns: async () => ({
                  data: { workflow_runs: runs }
                }),
                getWorkflowRun: async () => ({
                  data: outcome === "mirror timeout" ? runs[0] : remoteRun
                }),
                listJobsForWorkflowRun: async () => ({ data: { jobs: [] } })
              }
            }
          }
        });
        mirrorResult = failures.length ? "failure" : "success";
        assert.equal(dispatches, outcome === "active remote" ? 0 : 1);
        if (["mirror timeout", "uncertain dispatch"].includes(outcome))
          assert.equal(clock, 3000);
      }
      if (!failure) {
        phase = "complete";
        try {
          await complete(
            { core },
            currentSource,
            directory,
            {
              MIRROR_RESULT: mirrorResult,
              MIRROR_RELEASE_IDENTIFIER: remoteOutputs.release_identifier,
              MIRROR_RUN_STATE: remoteOutputs.run_state,
              MIRROR_RUN_ID: remoteOutputs.run_id,
              MIRROR_RUN_URL: remoteOutputs.run_url,
              MIRROR_CONCLUSION: remoteOutputs.conclusion
            },
            run
          );
        } catch (error) {
          failure = error.message;
        }
      }
      const receipt = JSON.parse(
        await readFile(join(directory, "receipt.json"), "utf8")
      );
      assert.deepEqual(summary, receipt);
      if (outcome === "stale") {
        assert.deepEqual(calls, []);
        assert.equal(failure, undefined);
        assert.equal(outputs.status, "superseded");
        assert.equal(dispatches, 0);
      } else if (
        ["public", "publication retry", "active remote"].includes(outcome)
      ) {
        assert.equal(failure, undefined);
        assert.equal(outputs.status, "published");
        assert.equal(receipt.status, "published");
        assert.equal(receipt.ghcr.digest, digest);
        assert.equal(receipt.acr.digest, digest);
        assert.equal(receipt.mirror.run_id, "456");
        assert.equal(calls.filter((call) => call === "upload").length, 1);
        assert.deepEqual(calls.slice(-2), ["verify ACR", "recheck GHCR"]);
      } else {
        assert.ok(failure, outcome);
        assert.equal(outputs.status, undefined);
        if (["upload failure", "GHCR digest failure"].includes(outcome)) {
          assert.equal(receipt.status, "partial");
          assert.equal(receipt.ghcr.digest, undefined);
          assert.ok(!calls.includes("visibility"));
        } else {
          assert.equal(receipt.ghcr.digest, digest);
          assert.equal(
            receipt.status,
            outcome === "final summary failure" ? "published" : "pending-mirror"
          );
        }
        if (
          ["private", "internal", "API failure", "anonymous failure"].includes(
            outcome
          )
        ) {
          assert.equal(dispatches, 0);
          assert.equal(receipt.mirror.run_state, "not-requested");
          assert.equal(receipt.acr.digest, undefined);
          assert.match(failure, /private ACR mirror has not been requested/);
          assert.match(failure, /Change visibility -> Public/);
        }
        if (
          [
            "mirror failure",
            "mirror timeout",
            "uncertain dispatch",
            "App failure"
          ].includes(outcome)
        ) {
          assert.equal(
            receipt.mirror.run_state,
            outcome === "uncertain dispatch" ? "unknown"
            : outcome === "App failure" ? "not-requested"
            : "found"
          );
          assert.equal(receipt.acr.digest, undefined);
          assert.ok(!calls.includes("verify ACR"));
        }
        if (["ACR digest failure", "GHCR moved"].includes(outcome)) {
          assert.match(failure, /Digest mismatch/);
        }
      }
    });
  }
});
