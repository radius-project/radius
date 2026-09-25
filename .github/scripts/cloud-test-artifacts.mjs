// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import {
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile
} from "node:fs/promises";
import { basename, join } from "node:path";
import { promisify } from "node:util";

const repo = { owner: "radius-project", repo: "radius" };
const repository = "radius-project/radius";
const workflow = ".github/workflows/functional-test-cloud.yaml";
const digestPattern = /^sha256:[a-f0-9]{64}$/;
const manifestTypes = [
  "application/vnd.oci.image.manifest.v1+json",
  "application/vnd.docker.distribution.manifest.v2+json"
];
const sha256 = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const json = async (file) => JSON.parse(await readFile(file, "utf8"));
const save = (file, value) =>
  writeFile(file, JSON.stringify(value, null, 2) + "\n");
const number = (value) => {
  assert.match(String(value), /^[1-9][0-9]*$/);
  assert.ok(Number.isSafeInteger(Number(value)), "Invalid numeric identity");
  return Number(value);
};
const request = () => ({ signal: AbortSignal.timeout(20_000) });
const archiveName = (source) =>
  `cloud-test-inputs-${source.runId}-${source.generationAttempt}.tar`;
const refName = "org.opencontainers.image.ref.name";
const workDirectory = (core) =>
  core.getInput("directory") ||
  join(process.env.RUNNER_TEMP, "cloud-test-inputs");

export const images = [
  "ucpd",
  "applications-rp",
  "dynamic-rp",
  "controller",
  "testrp",
  "magpiego",
  "bicep",
  "pre-upgrade"
];
export const registries = {
  images: "ghcr.io/radius-project/dev",
  recipes: "ghcr.io/radius-project/dev",
  types: "crradfunctest1b2s.azurecr.io"
};

export async function tool(args, options = {}) {
  const { stdout } = await promisify(execFile)(args[0], args.slice(1), {
    timeout: 180_000,
    maxBuffer: 1024 * 1024,
    ...options
  });
  return stdout;
}

export function sourceIdentity(context, env, attempt = env.GITHUB_RUN_ATTEMPT) {
  assert.equal(`${context.repo.owner}/${context.repo.repo}`, repository);
  assert.ok(
    [
      "pull_request_target",
      "merge_group",
      "schedule",
      "repository_dispatch",
      "workflow_dispatch"
    ].includes(context.eventName)
  );
  assert.match(env.CHECKOUT_REPO, /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/);
  for (const sha of [context.sha, env.CHECKOUT_REF])
    assert.match(sha, /^[a-f0-9]{40}$/);
  if (context.eventName === "pull_request_target") {
    assert.equal(
      env.CHECKOUT_REPO,
      context.payload.pull_request.head.repo.full_name
    );
    assert.equal(env.CHECKOUT_REF, context.payload.pull_request.head.sha);
  }
  return {
    repository: env.CHECKOUT_REPO,
    commit: env.CHECKOUT_REF,
    workflowRepository: repository,
    workflowSHA: context.sha,
    // PR-target API metadata names the candidate; manual source branches need not equal the run head.
    runHeadSHA:
      context.eventName === "pull_request_target" ?
        env.CHECKOUT_REF
      : context.sha,
    runId: number(context.runId),
    generationAttempt: number(attempt)
  };
}

export function verifyArtifact(artifact, source, repositoryId, suite) {
  number(artifact.id);
  assert.equal(artifact.expired, false, "Artifact expired; rerun its producer");
  assert.match(artifact.digest, digestPattern, "Missing artifact digest");
  const limit = suite ? 20 * 1024 ** 2 : 4 * 1024 ** 3;
  assert.ok(
    artifact.size_in_bytes > 0 && artifact.size_in_bytes <= limit,
    "Artifact exceeds size limit"
  );
  assert.equal(artifact.workflow_run.id, source.runId);
  assert.equal(artifact.workflow_run.repository_id, number(repositoryId));
  assert.equal(
    artifact.workflow_run.head_sha,
    source.runHeadSHA,
    "Artifact run head mismatch"
  );
  if (suite) {
    assert.ok(
      ["corerp-cloud", "ucp-cloud"].includes(suite),
      "Unexpected result suite"
    );
    assert.equal(artifact.name, `functional_test_results_${suite}.xml`);
    return source.generationAttempt;
  }
  const match = /^cloud-test-inputs-([0-9]+)-([0-9]+)\.tar$/.exec(
    artifact.name
  );
  assert.ok(
    match && number(match[1]) === source.runId,
    "Unexpected input artifact"
  );
  const attempt = number(match[2]);
  assert.ok(
    attempt <= source.generationAttempt,
    "Artifact from a future attempt"
  );
  return attempt;
}

/** Authenticate selection before the native action downloads any candidate bytes. */
export async function select({ github, core, context }, env = process.env) {
  let source = sourceIdentity(context, env);
  const suite = core.getInput("suite");
  let id = core.getInput("artifact_id");
  if (suite) {
    assert.ok(["corerp-cloud", "ucp-cloud"].includes(suite));
    const { data } = await github.rest.actions.listWorkflowRunArtifacts({
      ...repo,
      run_id: source.runId,
      name: `functional_test_results_${suite}.xml`,
      per_page: 100,
      request: request()
    });
    assert.equal(data.total_count, 1, "Missing or ambiguous test results");
    assert.equal(data.artifacts.length, 1);
    id = data.artifacts[0].id;
  }
  const { data: artifact } = await github.rest.actions.getArtifact({
    ...repo,
    artifact_id: number(id),
    request: request()
  });
  assert.equal(artifact.id, number(id));
  const attempt = verifyArtifact(
    artifact,
    source,
    env.GITHUB_REPOSITORY_ID,
    suite
  );
  const { data: run } = await github.rest.actions.getWorkflowRunAttempt({
    ...repo,
    run_id: source.runId,
    attempt_number: attempt,
    request: request()
  });
  assert.ok(
    run.id === source.runId &&
      run.run_attempt === attempt &&
      run.head_sha === source.runHeadSHA &&
      run.event === context.eventName &&
      run.repository.full_name === repository &&
      run.path.split("@")[0] === workflow,
    "Producing workflow attempt mismatch"
  );
  if (!suite) {
    const created = Date.parse(artifact.created_at);
    assert.ok(
      created >= Date.parse(run.run_started_at),
      "Artifact predates producer"
    );
    if (attempt < source.generationAttempt) {
      const { data: next } = await github.rest.actions.getWorkflowRunAttempt({
        ...repo,
        run_id: source.runId,
        attempt_number: attempt + 1,
        request: request()
      });
      assert.ok(
        created <= Date.parse(next.run_started_at),
        "Artifact postdates producer"
      );
    }
    source = { ...source, generationAttempt: attempt };
  }
  const directory = workDirectory(core);
  await mkdir(directory, { recursive: true });
  await save(join(directory, "selected.json"), { source, artifact, suite });
  core.setOutput("artifact_id", artifact.id);
}

export function destinations(roots, tag) {
  assert.match(tag, /^pr-func[a-f0-9]{10}$/);
  assert.ok(
    Array.isArray(roots) && roots.length >= 10 && roots.length <= 137,
    "Incomplete artifact set"
  );
  const seen = new Set();
  const targets = roots.map((root) => {
    descriptor(root, manifestTypes, 1024 * 1024);
    const match = /^(images|recipes|types)-([a-z0-9][a-z0-9_.-]{0,95})$/.exec(
      root.annotations?.[refName]
    );
    assert.ok(match, "Unexpected OCI reference");
    const [, kind, name] = match;
    if (kind === "images") assert.ok(images.includes(name), "Unexpected image");
    if (kind === "types") assert.equal(name, "radius", "Unexpected provider");
    assert.ok(!seen.has(`${kind}/${name}`), "Duplicate artifact");
    seen.add(`${kind}/${name}`);
    return {
      kind,
      name,
      digest: root.digest,
      reference: `${registries[kind]}/${repositoryPath(kind, name)}:${tag}`
    };
  });
  for (const required of [
    ...images.map((name) => `images/${name}`),
    "types/radius"
  ])
    assert.ok(seen.has(required), `Missing ${required}`);
  return targets;
}

function repositoryPath(kind, name) {
  if (kind === "recipes") return `test/testrecipes/test-bicep-recipes/${name}`;
  return kind === "types" ? "test/radius" : name;
}

/** Candidate execution ends at an opaque, raw OCI archive, not a script or executable. */
export async function capture(
  { core, context },
  env = process.env,
  run = tool
) {
  const source = sourceIdentity(context, env);
  const directory = workDirectory(core);
  await mkdir(directory, { recursive: true });
  const layout = await mkdtemp(join(directory, "layout-"));
  try {
    const recipes = (
      await readdir("test/testrecipes/test-bicep-recipes", { recursive: true })
    )
      .filter(
        (file) => file.endsWith(".bicep") && !basename(file).startsWith("_")
      )
      .map((file) => basename(file, ".bicep"))
      .sort();
    const entries = [
      ...images.map((name) => ({ kind: "images", name })),
      { kind: "types", name: "radius" },
      ...recipes.map((name) => ({ kind: "recipes", name }))
    ];
    for (const entry of entries) {
      const path =
        entry.kind === "images" ?
          `images/${entry.name}`
        : repositoryPath(entry.kind, entry.name);
      await run([
        "oras",
        "cp",
        "--to-oci-layout",
        `localhost:5000/${path}:${env.REL_VERSION}`,
        `${layout}:${entry.kind}-${entry.name}`
      ]);
    }
    destinations(
      (await json(join(layout, "index.json"))).manifests,
      env.REL_VERSION
    );
    await save(join(layout, "source.json"), { source, tag: env.REL_VERSION });
    await run([
      "tar",
      "-cf",
      join(directory, archiveName(source)),
      "-C",
      layout,
      "oci-layout",
      "index.json",
      "blobs",
      "source.json"
    ]);
  } finally {
    await rm(layout, { recursive: true, force: true });
  }
}

function descriptor(value, types, limit = 2 * 1024 ** 3) {
  assert.ok(
    value && types.includes(value.mediaType),
    "Unexpected OCI media type"
  );
  assert.match(value.digest, digestPattern);
  assert.ok(
    Number.isSafeInteger(value.size) && value.size >= 0 && value.size <= limit,
    "Invalid OCI size"
  );
  assert.ok(
    !value.urls && !value.data,
    "Foreign or inline OCI content is forbidden"
  );
  return value.size;
}

export function validateManifest(raw, entry) {
  assert.equal(sha256(raw), entry.digest, "Manifest digest mismatch");
  const manifest = JSON.parse(raw);
  assert.equal(manifest.schemaVersion, 2);
  assert.ok(manifestTypes.includes(manifest.mediaType ?? manifestTypes[0]));
  assert.ok(!manifest.subject, "OCI subjects are forbidden");
  assert.ok(
    Array.isArray(manifest.layers) &&
      manifest.layers.length > 0 &&
      manifest.layers.length <= 128
  );
  let configs, layers;
  if (entry.kind === "images") {
    assert.ok(!manifest.artifactType, "Unexpected image artifact type");
    configs = [
      "application/vnd.oci.image.config.v1+json",
      "application/vnd.docker.container.image.v1+json"
    ];
    layers = [
      "application/vnd.oci.image.layer.v1.tar",
      "application/vnd.oci.image.layer.v1.tar+gzip",
      "application/vnd.oci.image.layer.v1.tar+zstd",
      "application/vnd.docker.image.rootfs.diff.tar.gzip"
    ];
  } else {
    const prefix = `application/vnd.ms.bicep.${entry.kind === "types" ? "provider" : "module"}.`;
    assert.ok(
      manifest.artifactType === `${prefix}artifact` ||
        (entry.kind === "recipes" && !manifest.artifactType),
      "Unexpected Bicep artifact type"
    );
    assert.equal(
      manifest.layers.length,
      1,
      "Additional/executable Bicep layers are forbidden"
    );
    configs = [`${prefix}config.v1+json`];
    layers = [
      `${prefix}layer.v1${entry.kind === "types" ? ".tar+gzip" : "+json"}`
    ];
    assert.equal(
      manifest.config.digest,
      sha256("{}"),
      "Bicep config must be empty"
    );
    assert.equal(manifest.config.size, 2);
  }
  return (
    descriptor(manifest.config, configs) +
    manifest.layers.reduce((size, layer) => size + descriptor(layer, layers), 0)
  );
}

async function downloaded(directory, artifact) {
  const download = join(directory, "download");
  assert.deepEqual(
    await readdir(download),
    [artifact.name],
    "Unexpected downloaded files"
  );
  const file = join(download, artifact.name),
    info = await lstat(file);
  assert.ok(
    info.isFile() && info.size === artifact.size_in_bytes,
    "Invalid raw artifact size"
  );
  return file;
}

/** Only bounded metadata is read by tar; ORAS imports the verified graph, never archive paths as code. */
export async function prepare({ core }, env = process.env, run = tool) {
  const directory = workDirectory(core);
  const selected = await json(join(directory, "selected.json"));
  const archive = await downloaded(directory, selected.artifact);
  const metadata = JSON.parse(
    await run(["tar", "-xOf", archive, "source.json"])
  );
  assert.deepEqual(
    metadata.source,
    selected.source,
    "Candidate source metadata mismatch"
  );
  assert.equal(metadata.tag, env.REL_VERSION, "Candidate tag mismatch");
  const index = JSON.parse(await run(["tar", "-xOf", archive, "index.json"]));
  assert.equal(index.schemaVersion, 2);
  const targets = destinations(index.manifests, env.REL_VERSION);
  let bytes = 0;
  for (const entry of targets) {
    const raw = await run([
      "oras",
      "manifest",
      "fetch",
      "--oci-layout",
      `${archive}@${entry.digest}`
    ]);
    bytes += validateManifest(raw, entry);
    assert.ok(bytes <= 12 * 1024 ** 3, "OCI graph exceeds size limit");
  }
  for (const entry of targets)
    await run([
      "oras",
      "cp",
      "--from-oci-layout",
      "--to-oci-layout",
      `${archive}@${entry.digest}`,
      `${join(directory, "layout")}:${entry.kind}-${entry.name}`
    ]);
  await save(join(directory, "receipt.json"), {
    source: selected.source,
    artifact: { id: selected.artifact.id, digest: selected.artifact.digest },
    tag: metadata.tag,
    roots: index.manifests,
    published: []
  });
}

export async function publish(
  { core, context },
  env = process.env,
  run = tool
) {
  assert.deepEqual(
    [
      env.CONTAINER_REGISTRY,
      env.BICEP_RECIPE_REGISTRY,
      env.TEST_BICEP_TYPES_REGISTRY
    ],
    [registries.images, registries.recipes, registries.types],
    "Test registry allowlist mismatch"
  );
  const directory = workDirectory(core);
  const destination = core.getInput("destination", { required: true });
  assert.ok(["ghcr", "acr"].includes(destination));
  const receipt = await json(join(directory, "receipt.json"));
  assert.deepEqual(
    receipt.source,
    sourceIdentity(context, env, receipt.source.generationAttempt)
  );
  const targets = destinations(receipt.roots, env.REL_VERSION).filter(
    (entry) => (entry.kind === "types") === (destination === "acr")
  );
  try {
    for (const entry of targets) {
      await run([
        "oras",
        "cp",
        "--from-oci-layout",
        `${join(directory, "layout")}@${entry.digest}`,
        entry.reference
      ]);
      assert.equal(
        sha256(await run(["oras", "manifest", "fetch", entry.reference])),
        entry.digest,
        "Published digest changed"
      );
      receipt.published.push(entry.reference);
    }
  } finally {
    await save(join(directory, "receipt.json"), receipt);
    await core.summary.addCodeBlock(JSON.stringify(receipt), "json").write();
  }
}

export async function results({ core }) {
  const directory = workDirectory(core);
  const { artifact } = await json(join(directory, "selected.json"));
  const bytes = await readFile(await downloaded(directory, artifact));
  const xml = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  assert.ok(
    !/[\0]|<!DOCTYPE|<!ENTITY/i.test(xml),
    "XML declarations/entities are forbidden"
  );
  // The pinned JUnit reporter owns parsing and fails on missing/inconclusive reports.
  await mkdir("trusted-test-results");
  await writeFile("trusted-test-results/results.xml", bytes);
}
