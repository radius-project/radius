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
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const root = fileURLToPath(new URL("../../", import.meta.url));
const repo = { owner: "radius-project", repo: "radius" };
const workflow = ".github/workflows/build-main.yaml";
const manifestType = "application/vnd.oci.image.manifest.v1+json";
const providerType = "application/vnd.ms.bicep.provider.";
const sha256 = (data) =>
  `sha256:${createHash("sha256").update(data).digest("hex")}`;
const digestPattern = /^sha256:[a-f0-9]{64}$/;
const json = async (path) => JSON.parse(await readFile(path, "utf8"));
const save = (path, value) =>
  writeFile(path, JSON.stringify(value, null, 2) + "\n");
const artifactName = (source) => `bicep-types-radius-${source.runId}.tar`;
const request = () => ({ signal: AbortSignal.timeout(20_000) });

/** @typedef {{repository: string, ref: string, commit: string, workflow: string, runId: number, generationAttempt: number}} Source */

export const targets = Object.freeze({
  ghcr: "ghcr.io/radius-project/bicep-types-radius:edge",
  acr: "biceptypes.azurecr.io/radius:latest"
});

/** Run only trusted, installed tools; never a command from a downloaded bundle. */
export async function tool(args, options = {}) {
  const { stdout } = await promisify(execFile)(args[0], args.slice(1), {
    timeout: 180_000,
    maxBuffer: 1024 * 1024,
    ...options
  });
  return stdout;
}

export function route(context, env) {
  if (
    context.repo.owner !== repo.owner ||
    context.repo.repo !== repo.repo ||
    context.ref !== "refs/heads/main" ||
    !["push", "workflow_dispatch"].includes(context.eventName)
  )
    return "skip";
  assert.equal(env.CHANGES_RESULT, "success", "Bicep change detection failed");
  assert.ok(
    ["true", "false"].includes(env.ONLY_CHANGED),
    "Missing or invalid change detection"
  );
  if (env.ONLY_CHANGED === "true") return "skip";
  return env.PUBLISH_ENABLED === "true" ? "direct" : "legacy";
}

export function checkSummary(env) {
  assert.equal(env.BICEP_MODE_RESULT, "success", "Bicep mode selection failed");
  const expected = {
    skip: ["skipped", "skipped"],
    legacy: ["success", "skipped"],
    direct: ["skipped", "success"]
  }[env.BICEP_MODE];
  assert.ok(expected, "Missing Bicep publishing mode");
  assert.deepEqual(
    [env.BICEP_LEGACY_RESULT, env.BICEP_DIRECT_RESULT],
    expected,
    "Selected Bicep publisher failed or was skipped"
  );
  if (env.BICEP_MODE === "direct")
    assert.ok(
      ["published", "superseded"].includes(env.BICEP_PUBLICATION_STATUS),
      "Missing publication result"
    );
}

export function sourceIdentity(context, env) {
  assert.equal(
    `${context.repo.owner}/${context.repo.repo}`,
    "radius-project/radius"
  );
  assert.equal(context.ref, "refs/heads/main", "Only main may publish edge");
  assert.ok(
    ["push", "workflow_dispatch"].includes(context.eventName),
    "Untrusted publishing event"
  );
  assert.equal(env.GITHUB_REF_PROTECTED, "true", "Protected main is required");
  assert.equal(
    env.GITHUB_WORKFLOW_REF,
    `radius-project/radius/${workflow}@refs/heads/main`
  );
  assert.match(context.sha, /^[a-f0-9]{40}$/);
  assert.notEqual(context.sha, "0".repeat(40));
  assert.ok(Number.isSafeInteger(context.runId) && context.runId > 0);
  assert.match(env.GITHUB_RUN_ATTEMPT, /^[1-9][0-9]*$/);
  assert.ok(Number.isSafeInteger(Number(env.GITHUB_RUN_ATTEMPT)));
  return {
    repository: "radius-project/radius",
    ref: context.ref,
    commit: context.sha,
    workflow,
    runId: context.runId,
    generationAttempt: Number(env.GITHUB_RUN_ATTEMPT)
  };
}

/** @param {import('@actions/github-script').AsyncFunctionArguments} args */
async function verifiedSource({ github, context }) {
  const source = sourceIdentity(context, process.env);
  const { data: run } = await github.rest.actions.getWorkflowRun({
    ...repo,
    run_id: source.runId,
    request: request()
  });
  assert.ok(
    run.head_sha === source.commit &&
      run.head_branch === "main" &&
      run.path === workflow &&
      run.event === context.eventName &&
      run.run_attempt === source.generationAttempt &&
      run.repository.full_name === source.repository &&
      run.head_repository.full_name === source.repository,
    "Source workflow run does not match the trusted main invocation"
  );
  return source;
}

/** @param {import('@octokit/openapi-types').components['schemas']['artifact']} artifact @param {Source} source */
export function verifyArtifact(artifact, source) {
  assert.equal(artifact.name, artifactName(source));
  assert.equal(
    artifact.expired,
    false,
    "Snapshot expired; start a new main run"
  );
  assert.ok(Number.isSafeInteger(artifact.id) && artifact.id > 0);
  assert.ok(
    artifact.size_in_bytes > 0 && artifact.size_in_bytes <= 64 * 1024 * 1024,
    "Snapshot exceeds size limit"
  );
  assert.match(
    artifact.digest,
    digestPattern,
    "Missing Actions artifact digest"
  );
  assert.ok(
    artifact.workflow_run.id === source.runId &&
      artifact.workflow_run.head_sha === source.commit &&
      artifact.workflow_run.head_branch === "main",
    "Artifact belongs to another source run"
  );
}

export function selectSnapshot(artifacts, source) {
  const matches = artifacts.filter(
    (artifact) => artifact.name === artifactName(source)
  );
  assert.ok(matches.length <= 1, "Ambiguous snapshot");
  if (matches.length) {
    verifyArtifact(matches[0], source);
    return matches[0].id;
  }
  assert.equal(
    source.generationAttempt,
    1,
    "Retry snapshot missing; start a new approved main run instead of regenerating"
  );
  return "";
}

export async function capture(source, directory, registry, run = tool) {
  assert.match(
    registry,
    /^localhost:[0-9]+$/,
    "Packaging must use the isolated local registry"
  );
  const scratch = await mkdtemp(join(tmpdir(), "radius-bicep-"));
  const layout = join(scratch, "layout");
  try {
    await save(join(scratch, "bicepconfig.json"), {
      experimentalFeaturesEnabled: { ociEnabled: true }
    });
    await mkdir(join(scratch, "docker"));
    await save(join(scratch, "docker/config.json"), {});
    await run(
      [
        "bicep",
        "publish-extension",
        join(root, "hack/bicep-types-radius/generated/index.json"),
        "--target",
        `br:${registry}/radius:bundle`
      ],
      {
        cwd: scratch,
        env: {
          ...process.env,
          DOCKER_CONFIG: join(scratch, "docker"),
          BICEP_TRUSTED_REGISTRIES: "localhost"
        }
      }
    );
    await run([
      "oras",
      "cp",
      "--from-plain-http",
      "--to-oci-layout",
      `${registry}/radius:bundle`,
      `${layout}:bundle`
    ]);
    const index = await json(join(layout, "index.json"));
    await save(join(layout, "source.json"), {
      source,
      manifestDigest: index.manifests[0].digest
    });
    await run([
      "tar",
      "-cf",
      join(directory, artifactName(source)),
      "-C",
      layout,
      "oci-layout",
      "index.json",
      "blobs",
      "source.json"
    ]);
  } finally {
    await rm(scratch, { recursive: true, force: true });
  }
}

/** Check the small Bicep data contract; artifact transfer and OCI operations stay with Actions/ORAS. */
export async function prepare(archive, source, directory, run = tool) {
  const metadata = JSON.parse(
    await run(["tar", "-xOf", archive, "source.json"])
  );
  assert.ok(
    Number.isSafeInteger(metadata.source.generationAttempt) &&
      metadata.source.generationAttempt >= 1 &&
      metadata.source.generationAttempt <= source.generationAttempt
  );
  assert.deepEqual(
    { ...metadata.source, generationAttempt: source.generationAttempt },
    source,
    "Snapshot source mismatch"
  );
  const layout = join(directory, "layout");
  const raw = await run([
    "oras",
    "manifest",
    "fetch",
    "--oci-layout",
    `${archive}:bundle`
  ]);
  assert.equal(
    sha256(raw),
    metadata.manifestDigest,
    "Manifest digest mismatch"
  );
  const manifest = JSON.parse(raw);
  assert.ok(
    manifest.schemaVersion === 2 &&
      manifest.mediaType === manifestType &&
      manifest.artifactType === `${providerType}artifact`,
    "Invalid Bicep manifest"
  );
  assert.equal(manifest.config.mediaType, `${providerType}config.v1+json`);
  assert.equal(
    manifest.layers.length,
    1,
    "Executable/additional extension layers are forbidden"
  );
  assert.equal(
    manifest.layers[0].mediaType,
    `${providerType}layer.v1.tar+gzip`
  );
  assert.equal(manifest.annotations["bicep.serialization.format"], "v1");
  assert.ok(!manifest.subject, "Unexpected Bicep artifact subject");
  for (const blob of [manifest.config, ...manifest.layers]) {
    assert.match(blob.digest, digestPattern);
    assert.ok(!blob.urls, "Foreign blob URLs are forbidden");
  }
  assert.deepEqual(
    JSON.parse(
      await run([
        "oras",
        "manifest",
        "fetch-config",
        "--oci-layout",
        `${archive}:bundle`
      ])
    ),
    {},
    "Executable provider config is forbidden"
  );
  // ORAS imports only the content-addressed graph; no archive files are extracted as code.
  await run([
    "oras",
    "cp",
    "--from-oci-layout",
    "--to-oci-layout",
    `${archive}@${metadata.manifestDigest}`,
    `${layout}:bundle`
  ]);
  manifest.annotations = {
    ...manifest.annotations,
    "org.opencontainers.image.source": `https://github.com/${source.repository}`,
    "org.opencontainers.image.revision": source.commit
  };
  const stamped = JSON.stringify(manifest) + "\n";
  await writeFile(join(directory, "manifest.json"), stamped);
  await run([
    "oras",
    "manifest",
    "push",
    "--oci-layout",
    `${layout}:bundle`,
    join(directory, "manifest.json")
  ]);
  return {
    source: metadata.source,
    manifestDigest: sha256(stamped),
    publicationAttempt: source.generationAttempt,
    status: "failed",
    ghcr: { reference: targets.ghcr },
    acr: { reference: targets.acr }
  };
}

export async function publishPair(receipt, layout, mainSha, run = tool) {
  if (mainSha !== receipt.source.commit) {
    assert.equal(
      receipt.publicationAttempt,
      1,
      "Superseded retry: prior publication may be partial. Inspect both tags/receipts and recover from current main"
    );
    receipt.status = "superseded";
    return;
  }
  const verify = async (reference) =>
    assert.equal(
      sha256(await run(["oras", "manifest", "fetch", reference])),
      receipt.manifestDigest,
      `Digest mismatch: ${reference}`
    );
  receipt.status = "partial";
  await run([
    "oras",
    "cp",
    "--from-oci-layout",
    `${layout}@${receipt.manifestDigest}`,
    targets.ghcr
  ]);
  await verify(targets.ghcr);
  receipt.ghcr.digest = receipt.manifestDigest;
  await run([
    "oras",
    "cp",
    `${targets.ghcr.split(":")[0]}@${receipt.manifestDigest}`,
    targets.acr
  ]);
  await verify(targets.acr);
  await verify(targets.ghcr);
  receipt.acr.digest = receipt.manifestDigest;
  receipt.status = "published";
}

/** @param {import('@actions/github-script').AsyncFunctionArguments} args */
export default async function script(args) {
  const { github, core, context } = args;
  try {
    const operation = core.getInput("operation", { required: true });
    if (operation === "mode") {
      if (!["", "true", "false"].includes(process.env.PUBLISH_ENABLED))
        core.notice(
          "Invalid activation value; direct Bicep publishing stays disabled"
        );
      core.setOutput("mode", route(context, process.env));
      return;
    }
    if (operation === "summary") return checkSummary(process.env);
    const source = await verifiedSource(args);
    const directory = core.getInput("directory", { required: true });
    await mkdir(directory, { recursive: true });
    if (operation === "select") {
      const { data } = await github.rest.actions.listWorkflowRunArtifacts({
        ...repo,
        run_id: source.runId,
        per_page: 100,
        request: request()
      });
      assert.equal(
        data.total_count,
        data.artifacts.length,
        "Snapshot listing is incomplete"
      );
      const id = selectSnapshot(data.artifacts, source);
      core.setOutput("artifact_id", id);
      core.setOutput("reuse", id !== "");
    } else if (operation === "capture") {
      await capture(
        source,
        directory,
        core.getInput("registry", { required: true })
      );
    } else if (operation === "prepare") {
      const id = Number(core.getInput("artifact_id", { required: true }));
      assert.ok(Number.isSafeInteger(id) && id > 0);
      const { data: artifact } = await github.rest.actions.getArtifact({
        ...repo,
        artifact_id: id,
        request: request()
      });
      verifyArtifact(artifact, source);
      const downloads = await readdir(join(directory, "download"));
      assert.equal(
        downloads.length,
        1,
        "Expected exactly the selected raw snapshot"
      );
      const archive = join(directory, "download", downloads[0]);
      const stat = await lstat(archive);
      assert.ok(stat.isFile() && stat.size <= 64 * 1024 * 1024);
      assert.equal(
        sha256(await readFile(archive)),
        artifact.digest,
        "Snapshot digest mismatch"
      );
      const receipt = await prepare(archive, source, directory);
      receipt.artifact = { id, digest: artifact.digest };
      await save(join(directory, "receipt.json"), receipt);
    } else if (operation === "publish") {
      const receipt = await json(join(directory, "receipt.json"));
      assert.deepEqual(
        { ...receipt.source, generationAttempt: source.generationAttempt },
        source
      );
      try {
        const { data: pkg } =
          await github.rest.packages.getPackageForOrganization({
            package_type: "container",
            package_name: "bicep-types-radius",
            org: repo.owner,
            request: request()
          });
        assert.equal(
          pkg.visibility,
          "public",
          "Preprovision the public GHCR package before activation"
        );
        const { data: main } = await github.rest.repos.getBranch({
          ...repo,
          branch: "main",
          request: request()
        });
        assert.equal(main.protected, true);
        assert.match(main.commit.sha, /^[0-9a-f]{40}$/);
        await publishPair(receipt, join(directory, "layout"), main.commit.sha);
        core.setOutput("status", receipt.status);
        core.setOutput("manifest_digest", receipt.manifestDigest);
      } finally {
        await save(join(directory, "receipt.json"), receipt);
        await core.summary
          .addCodeBlock(JSON.stringify(receipt, null, 2), "json")
          .write();
      }
    } else {
      throw new Error(`Unknown Bicep operation: ${operation}`);
    }
  } catch (error) {
    core.setFailed(error instanceof Error ? error.message : String(error));
  }
}
