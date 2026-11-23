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
  recipes: "ghcr.io/radius-project/dev"
};

// The caller must supply repository configuration, never candidate metadata.
export function testTypesRegistry(env = process.env) {
  const registry = env.TEST_BICEP_TYPES_REGISTRY;
  assert.ok(
    typeof registry === "string" &&
      /^[a-z0-9]{5,50}\.azurecr\.io$/.test(registry) &&
      registry.trim() === registry,
    "Invalid TEST_BICEP_TYPES_REGISTRY: expected a bare lowercase ACR hostname"
  );
  return registry;
}

export async function tool(args, options = {}) {
  const { onStderr, ...settings } = options;
  let result;
  try {
    result = await promisify(execFile)(args[0], args.slice(1), {
      timeout: 180_000,
      maxBuffer: 1024 * 1024,
      ...settings
    });
  } catch (error) {
    onStderr?.(error.stderr ?? "");
    throw error;
  }
  onStderr?.(result.stderr);
  return result.stdout;
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

export function producerTag(uniqueID, source) {
  assert.match(uniqueID, /^func[a-f0-9]{10}$/);
  return `test-${uniqueID}-${number(source.runId)}-${number(source.generationAttempt)}`;
}

function validateTag(tag, source, env) {
  assert.equal(
    tag,
    producerTag(env.UNIQUE_ID, source),
    "Tag does not identify the authenticated producer"
  );
  assert.equal(tag, env.REL_VERSION, "Producer output tag mismatch");
}

export function verifyArtifact(artifact, source, repositoryId) {
  number(artifact.id);
  assert.equal(artifact.expired, false, "Artifact expired; rerun its producer");
  assert.match(artifact.digest, digestPattern, "Missing artifact digest");
  assert.ok(
    artifact.size_in_bytes > 0 && artifact.size_in_bytes <= 4 * 1024 ** 3,
    "Artifact exceeds size limit"
  );
  assert.equal(artifact.workflow_run.id, source.runId);
  assert.equal(artifact.workflow_run.repository_id, number(repositoryId));
  assert.equal(
    artifact.workflow_run.head_sha,
    source.runHeadSHA,
    "Artifact run head mismatch"
  );
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
  const id = core.getInput("artifact_id", { required: true });
  const { data: artifact } = await github.rest.actions.getArtifact({
    ...repo,
    artifact_id: number(id),
    request: request()
  });
  assert.equal(artifact.id, number(id));
  const attempt = verifyArtifact(artifact, source, env.GITHUB_REPOSITORY_ID);
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
  validateTag(env.REL_VERSION, source, env);
  const directory = workDirectory(core);
  await mkdir(directory, { recursive: true });
  await save(join(directory, "selected.json"), { source, artifact });
  core.setOutput("artifact_id", artifact.id);
}

export function destinations(roots, tag, env = process.env) {
  const typesRegistry = testTypesRegistry(env);
  assert.match(tag, /^test-func[a-f0-9]{10}-[1-9][0-9]*-[1-9][0-9]*$/);
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
      mediaType: root.mediaType,
      size: root.size,
      reference: `${kind === "types" ? typesRegistry : registries[kind]}/${repositoryPath(kind, name)}:${tag}`
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
  testTypesRegistry(env);
  const source = sourceIdentity(context, env);
  validateTag(env.REL_VERSION, source, env);
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
        "--from-plain-http=false",
        "--to-oci-layout",
        `localhost:5000/${path}:${env.REL_VERSION}`,
        `${layout}:${entry.kind}-${entry.name}`
      ]);
    }
    destinations(
      (await json(join(layout, "index.json"))).manifests,
      env.REL_VERSION,
      env
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
  testTypesRegistry(env);
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
  validateTag(metadata.tag, selected.source, env);
  const index = JSON.parse(await run(["tar", "-xOf", archive, "index.json"]));
  assert.equal(index.schemaVersion, 2);
  const targets = destinations(index.manifests, env.REL_VERSION, env);
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

function manifestPaths(entry) {
  const slash = entry.reference.indexOf("/");
  const colon = entry.reference.lastIndexOf(":");
  const repository = entry.reference.slice(slash + 1, colon);
  return {
    host: entry.reference.slice(0, slash),
    prefix: `/v2/${repository}/`,
    tag: `/v2/${repository}/manifests/${entry.reference.slice(colon + 1)}`,
    digest: `/v2/${repository}/manifests/${entry.digest}`
  };
}

/** Project ORAS's in-memory trace onto an allowlist; never emit the raw trace. */
export function registryResponses(trace, entry) {
  const paths = manifestPaths(entry);
  const requests = new Map(),
    responses = [];
  for (const block of trace.split(/^\[[^\]\r\n]+\]\[DEBUG\]: /m)) {
    const request = /^--> Request #([0-9]+)\n/.exec(block);
    if (request) {
      let method = "unknown",
        path = "[redacted]",
        trusted = false;
      try {
        const url = new URL(
          JSON.parse(/^> Request URL: (.+)$/m.exec(block)[1])
        );
        method = JSON.parse(/^> Request method: (.+)$/m.exec(block)[1]);
        if (!/^(GET|HEAD|PUT|POST|PATCH|DELETE)$/.test(method))
          method = "unknown";
        trusted =
          url.host === paths.host &&
          url.protocol === "https:" &&
          !url.username &&
          !url.password;
        if (trusted) {
          if (url.pathname === paths.tag || url.pathname === paths.digest)
            path = url.pathname;
          else if (
            url.pathname.startsWith(paths.prefix) &&
            /^blobs\/sha256:[a-f0-9]{64}$/.test(
              url.pathname.slice(paths.prefix.length)
            )
          )
            path = url.pathname;
          else if (url.pathname.startsWith(`${paths.prefix}blobs/uploads/`))
            path = `${paths.prefix}blobs/uploads/[redacted]`;
        }
      } catch {
        // Unknown trace formats fail closed and cannot authorize recovery.
      }
      requests.set(request[1], { method, path, trusted });
    }
    const response =
      /^<-- Response #([0-9]+)\n< Response Status: "([1-5][0-9]{2})(?: [^"\r\n]*)?"\n/.exec(
        block
      );
    if (!response) continue;
    const {
      method = "unknown",
      path = "[redacted]",
      trusted
    } = requests.get(response[1]) ?? {};
    requests.delete(response[1]);
    const requestIDs = {};
    if (trusted) {
      const headers =
        block.split("< Response headers:\n")[1]?.split("< Response body:")[0] ??
        "";
      for (const match of headers.matchAll(
        /^\s+"(X-Github-Request-Id|X-Ms-Request-Id|X-Ms-Correlation-Request-Id)": "([^"\r\n]+)"$/gim
      )) {
        const value = match[2];
        if (
          /^[a-f0-9]+(?::[a-f0-9]+){3,5}$/i.test(value) ||
          /^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(value)
        )
          requestIDs[match[1].toLowerCase()] = value;
      }
    }
    responses.push({ method, path, status: Number(response[2]), requestIDs });
  }
  for (const { method, path } of requests.values())
    responses.push({ method, path, status: null, requestIDs: {} });
  return responses;
}

async function registryCommand(args, entry, phase, core, run, options = {}) {
  let responses = [];
  try {
    return await run([...args.slice(0, -1), "--debug", args.at(-1)], {
      ...options,
      onStderr: (trace) => {
        responses = registryResponses(trace, entry);
        for (const response of responses)
          core.info(JSON.stringify({ phase, ...response }));
      }
    });
  } catch (error) {
    // execFile errors include raw stderr, headers and URLs, even in their message.
    throw Object.assign(
      new Error(
        `Registry ${phase} failed for ${entry.reference}; see sanitized HTTP diagnostics`
      ),
      {
        responses,
        exited: error.code === 1 && !error.killed && !error.signal
      }
    );
  }
}

function missingManifest(error, path) {
  const last = error.responses?.at(-1);
  return (
    error.exited &&
    last?.method === "GET" &&
    last.path === path &&
    last.status === 404
  );
}

export async function publish(
  { core, context, sleep },
  env = process.env,
  run = tool
) {
  testTypesRegistry(env);
  assert.deepEqual(
    [env.CONTAINER_REGISTRY, env.BICEP_RECIPE_REGISTRY],
    [registries.images, registries.recipes],
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
  validateTag(receipt.tag, receipt.source, env);
  const targets = destinations(receipt.roots, receipt.tag, env).filter(
    (entry) => (entry.kind === "types") === (destination === "acr")
  );
  // Capture is a standalone sparse checkout; only the full trusted publisher needs this helper.
  const { waitForState } = await import("./release-reconciliation.mjs");
  try {
    for (const entry of targets) {
      const file = join(
        directory,
        "layout",
        "blobs",
        "sha256",
        entry.digest.slice(7)
      );
      const info = await lstat(file);
      assert.ok(
        info.isFile() && info.size === entry.size,
        "Imported root size/type mismatch"
      );
      const raw = await readFile(file);
      assert.equal(raw.length, entry.size, "Imported root size mismatch");
      validateManifest(raw, entry);
      assert.equal(
        JSON.parse(raw).mediaType ?? entry.mediaType,
        entry.mediaType,
        "Imported root media type mismatch"
      );
      core.info(
        JSON.stringify({
          phase: "verified-import",
          reference: entry.reference,
          digest: sha256(raw),
          size: raw.length,
          mediaType: entry.mediaType
        })
      );
      const paths = manifestPaths(entry);
      try {
        await registryCommand(
          [
            "oras",
            "cp",
            "--from-oci-layout",
            `${join(directory, "layout")}@${entry.digest}`,
            entry.reference
          ],
          entry,
          "copy",
          core,
          run
        );
      } catch (error) {
        const put = error.responses?.at(-2);
        if (
          !missingManifest(error, paths.digest) ||
          put?.method !== "PUT" ||
          put.path !== paths.tag ||
          put.status !== 201
        )
          throw error;
        core.info(
          `ORAS post-copy destination read failed for ${entry.digest}; verifying without another upload`
        );
      }
      for (const [phase, reference, path] of [
        [
          "digest-readback",
          `${entry.reference.slice(0, entry.reference.lastIndexOf(":"))}@${entry.digest}`,
          paths.digest
        ],
        ["tag-readback", entry.reference, paths.tag]
      ]) {
        await waitForState(
          async () => {
            let published;
            try {
              published = await registryCommand(
                ["oras", "manifest", "fetch", reference],
                entry,
                phase,
                core,
                run,
                { timeout: 10_000 }
              );
            } catch (error) {
              if (missingManifest(error, path)) return undefined;
              throw error;
            }
            assert.equal(
              sha256(published),
              entry.digest,
              `Published digest changed during ${phase}`
            );
            assert.equal(
              Buffer.byteLength(published),
              entry.size,
              `Published size changed during ${phase}`
            );
            return true;
          },
          { core, sleep, description: `${phase} ${reference}` }
        );
      }
      receipt.published.push(entry.reference);
    }
  } finally {
    await save(join(directory, "receipt.json"), receipt);
    await core.summary.addCodeBlock(JSON.stringify(receipt), "json").write();
  }
}
