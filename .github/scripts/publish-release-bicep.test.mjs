// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { capture, tool } from "./bicep-types.mjs";
import {
  bicepExtensionTargets,
  plannedBicepExtensions,
  validateBicepExtensionLock
} from "./release-bicep-extensions.mjs";
import script, {
  approvedRelease,
  existingManifest,
  generationSource,
  prepareRelease,
  publishRelease,
  releaseIdentity,
  selectSnapshot,
  verifyRun,
  verifySnapshot
} from "./publish-release-bicep.mjs";

const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
const context = {
  repo: { owner: "radius-project", repo: "radius" },
  ref: "refs/tags/v0.62.0-rc.1",
  eventName: "push",
  sha: "a".repeat(40),
  runId: 123
};
const env = {
  GITHUB_RUN_ATTEMPT: "1",
  GITHUB_WORKFLOW_REF:
    "radius-project/radius/.github/workflows/build-release.yaml@refs/tags/v0.62.0-rc.1"
};
const plan = {
  schemaVersion: 2,
  version: "v0.62.0-rc.1",
  releaseType: "rc",
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
const source = releaseIdentity(context, env, plan);
const expected = plannedBicepExtensions(plan, context.sha)[1];
const artifact = {
  id: 7,
  name: "bicep-types-radius-123.tar",
  digest: hash("archive"),
  size_in_bytes: 7,
  expired: false,
  created_at: "2026-10-01T10:01:00Z",
  workflow_run: { id: 123, head_sha: context.sha, head_branch: "v0.62.0-rc.1" }
};
const job = {
  name: "Stage Bicep / Capture versioned Radius Bicep types",
  status: "completed",
  conclusion: "success",
  started_at: "2026-10-01T10:00:00Z",
  completed_at: "2026-10-01T10:02:00Z"
};
const workflowRun = {
  id: 123,
  run_attempt: 1,
  head_sha: source.commit,
  head_branch: "v0.62.0-rc.1",
  path: source.workflow,
  event: "push",
  repository: { full_name: source.repository },
  head_repository: { full_name: source.repository }
};
function client(run = workflowRun, jobs = [job]) {
  return {
    paginate: async () => jobs,
    rest: {
      repos: {
        getCommit: async () => ({ data: { sha: source.commit } })
      },
      actions: {
        getWorkflowRunAttempt: async (args) => {
          assert.equal(args.run_id, run.id);
          assert.equal(args.attempt_number, run.run_attempt);
          return { data: run };
        },
        listJobsForWorkflowRunAttempt: () => {}
      },
      packages: {
        getPackageForOrganization: async () => ({
          data: { visibility: "public" }
        })
      }
    }
  };
}
async function temp(t) {
  const directory = await mkdtemp(join(tmpdir(), "release-bicep-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}
async function fixture(t) {
  const directory = await temp(t);
  const archive = join(directory, "snapshot.tar");
  await writeFile(archive, "archive");
  const blob = (mediaType) => ({ mediaType, digest: hash("blob"), size: 4 });
  const prefix = "application/vnd.ms.bicep.provider.";
  const manifest = {
    schemaVersion: 2,
    mediaType: "application/vnd.oci.image.manifest.v1+json",
    artifactType: `${prefix}artifact`,
    config: blob(`${prefix}config.v1+json`),
    layers: [blob(`${prefix}layer.v1.tar+gzip`)],
    annotations: { "bicep.serialization.format": "v1" }
  };
  const metadata = { source, manifestDigest: hash(JSON.stringify(manifest)) };
  const commands = [];
  const run = async (args) => {
    commands.push(args);
    if (args[0] === "tar") return JSON.stringify(metadata);
    if (args[2] === "fetch") return JSON.stringify(manifest);
    if (args[2] === "fetch-config") return "{}";
    return "";
  };
  return { directory, archive, manifest, metadata, commands, run };
}

test("release identity requires the canonical approved stable/RC plan and tag, not current main", () => {
  for (const [version, releaseType] of [
    ["v0.62.0-rc.1", "rc"],
    ["v0.62.0", "final"],
    ["v0.62.1", "patch"]
  ]) {
    const ref = `refs/tags/${version}`;
    const identity = releaseIdentity(
      { ...context, ref },
      {
        ...env,
        GITHUB_WORKFLOW_REF: `radius-project/radius/${source.workflow}@${ref}`
      },
      { ...plan, version, releaseType }
    );
    assert.equal(identity.commit, context.sha);
  }
  for (const override of [
    { eventName: "pull_request" },
    { eventName: "repository_dispatch" },
    { ref: "refs/heads/main" },
    { sha: "0".repeat(40) },
    { repo: { owner: "fork", repo: "radius" } },
    { runId: 0 }
  ])
    assert.throws(() =>
      releaseIdentity({ ...context, ...override }, env, plan)
    );
  for (const override of [
    { schemaVersion: 1 },
    { version: "v0.62.0-rc1" },
    { version: "v0.62.0" },
    { releaseType: "patch" },
    { siblingRepositories: [] },
    {
      expectedOutputs: {
        ...plan.expectedOutputs,
        bicepExtensionsContract: undefined
      }
    }
  ])
    assert.throws(() =>
      releaseIdentity(context, env, { ...plan, ...override })
    );
  assert.throws(() =>
    releaseIdentity(context, { ...env, GITHUB_WORKFLOW_REF: "untrusted" }, plan)
  );
});

test("run and snapshot evidence reject wrong source, workflow, attempt, ownership and missing/expired data", async () => {
  await verifyRun(client(), source);
  for (const override of [
    { head_sha: "b".repeat(40) },
    { head_branch: "main" },
    { path: ".github/workflows/build-main.yaml" },
    { event: "pull_request" },
    { repository: { full_name: "fork/radius" } },
    { head_repository: { full_name: "fork/radius" } }
  ])
    await assert.rejects(
      verifyRun(client({ ...workflowRun, ...override }), source)
    );
  verifySnapshot(artifact, source);
  for (const override of [
    { expired: true },
    { name: "other" },
    { digest: "" },
    { id: 0 },
    { size_in_bytes: 65 * 1024 * 1024 },
    { workflow_run: { ...artifact.workflow_run, id: 321 } },
    { workflow_run: { ...artifact.workflow_run, head_branch: "main" } },
    { workflow_run: { ...artifact.workflow_run, head_sha: "b".repeat(40) } }
  ])
    assert.throws(() => verifySnapshot({ ...artifact, ...override }, source));
  assert.equal(selectSnapshot([], source), "");
  assert.equal(
    selectSnapshot([artifact], { ...source, generationAttempt: 2 }),
    7
  );
  assert.throws(
    () => selectSnapshot([artifact, artifact], source),
    /Ambiguous/
  );
  assert.throws(
    () => selectSnapshot([], { ...source, generationAttempt: 2 }),
    /never regenerate/
  );
  assert.throws(() => selectSnapshot([], source, true), /never regenerate/);
  assert.deepEqual(generationSource(source, "321", "2"), {
    ...source,
    runId: 321,
    generationAttempt: 2
  });
  for (const args of [
    ["321", ""],
    ["", "1"],
    ["1e3", "1"],
    ["1", "0"]
  ])
    assert.throws(() => generationSource(source, ...args));
});

test("approval reuses the merged-release resolver and committed-plan validator", async (t) => {
  const directory = await temp(t);
  const github = client();
  const approved = "c".repeat(40);
  github.rest.pulls = { list: () => {} };
  github.paginate = async () => [
    {
      number: 19,
      head: {
        ref: "automation/prepare-release-0.62.0-rc.1",
        repo: { full_name: source.repository }
      },
      base: { ref: "main" },
      merged_at: "2026-10-01",
      merge_commit_sha: approved
    }
  ];
  const commands = [];
  const run = async (args) => {
    commands.push(args);
    if (args[0] === "yq") return JSON.stringify(plan);
    if (args[1] === "rev-parse") return source.commit;
    if (args[1] === "show") return "approved YAML";
    if (args[0] === "bash") {
      assert.ok(args.includes(approved));
      assert.ok(args.includes(source.commit));
      assert.equal(
        args[1],
        ".github/scripts/validate-release-controller-plan.sh"
      );
      await mkdir(join(directory, "approved"));
      await writeFile(join(directory, "approved/ready.txt"), "true\n");
    }
    return "";
  };
  assert.deepEqual(
    await approvedRelease({ github, context }, directory, env, run),
    { source, expected }
  );
  assert.ok(
    commands.some(
      (args) =>
        args[1] === "show" &&
        args[2] === `${approved}:.github/release-plans/${plan.version}.yaml`
    )
  );
  await assert.rejects(
    approvedRelease({ github, context }, directory, env, async (args) => {
      if (args[0] === "bash")
        throw new Error("committed plan differs from approved plan");
      return run(args);
    }),
    /committed plan differs/
  );
  github.paginate = async () => [];
  await assert.rejects(
    approvedRelease({ github, context }, directory, env, run),
    /Expected one merged/
  );
  github.rest.repos.getCommit = async () => ({ data: { sha: "b".repeat(40) } });
  await assert.rejects(
    approvedRelease({ github, context }, directory, env, run),
    /Release tag no longer/
  );
});

test("original snapshot produces the exact Radius-only R1 record and preserves its attempt on retry", async (t) => {
  const f = await fixture(t);
  const first = await prepareRelease(
    client(),
    f.archive,
    source,
    expected,
    artifact,
    f.directory,
    f.run
  );
  const retry = await prepareRelease(
    client(),
    f.archive,
    { ...source, generationAttempt: 3 },
    expected,
    artifact,
    f.directory,
    f.run
  );
  assert.deepEqual(first, retry);
  assert.deepEqual(first.record, {
    ...expected,
    digest: hash(await readFile(join(f.directory, "manifest.json"))),
    generation: {
      workflow: source.workflow,
      runId: 123,
      runAttempt: 1,
      artifactId: 7,
      artifactDigest: artifact.digest
    }
  });
  const stamped = JSON.parse(
    await readFile(join(f.directory, "manifest.json"))
  );
  assert.equal(
    stamped.annotations["org.opencontainers.image.version"],
    expected.version
  );
  assert.deepEqual(stamped.layers, f.manifest.layers);
  assert.throws(
    () =>
      validateBicepExtensionLock(plan, source.commit, {
        schemaVersion: 1,
        version: plan.version,
        releaseSourceCommit: source.commit,
        artifacts: [first.record]
      }),
    /exactly the AWS and Radius/
  );
  assert.ok(f.commands.every((args) => !args.includes("--force")));
});

test("cross-run recovery authenticates and preserves the explicitly selected original run and attempt", async (t) => {
  const f = await fixture(t);
  const original = generationSource(source, "321", "2");
  f.metadata.source = original;
  const originalArtifact = {
    ...artifact,
    name: "bicep-types-radius-321.tar",
    workflow_run: { ...artifact.workflow_run, id: 321 }
  };
  const github = client({ ...workflowRun, id: 321, run_attempt: 2 });
  assert.equal(selectSnapshot([originalArtifact], original, true), artifact.id);
  const receipt = await prepareRelease(
    github,
    f.archive,
    original,
    expected,
    originalArtifact,
    f.directory,
    f.run
  );
  assert.equal(receipt.record.generation.runId, 321);
  assert.equal(receipt.record.generation.runAttempt, 2);
  await assert.rejects(
    prepareRelease(
      client({
        ...workflowRun,
        id: 321,
        run_attempt: 2,
        head_sha: "b".repeat(40)
      }),
      f.archive,
      original,
      expected,
      originalArtifact,
      f.directory,
      f.run
    ),
    /does not match/
  );
});

test("corrupt archives, source substitution, failed or forged generation attempts fail before publication", async (t) => {
  for (const kind of [
    "bytes",
    "manifest",
    "source",
    "failed job",
    "missing job",
    "forged attempt",
    "executable"
  ]) {
    await t.test(kind, async (t) => {
      const f = await fixture(t);
      let github = client();
      if (kind === "bytes") await writeFile(f.archive, "corrupt");
      if (kind === "manifest") f.metadata.manifestDigest = hash("other");
      if (kind === "source")
        f.metadata.source = { ...source, commit: "b".repeat(40) };
      if (kind === "failed job")
        github = client(workflowRun, [{ ...job, conclusion: "failure" }]);
      if (kind === "missing job") github = client(workflowRun, []);
      if (kind === "forged attempt")
        github = client(workflowRun, [
          { ...job, started_at: "2026-10-02T00:00:00Z" }
        ]);
      if (kind === "executable") {
        f.manifest.layers.push(f.manifest.layers[0]);
        f.metadata.manifestDigest = hash(JSON.stringify(f.manifest));
      }
      await assert.rejects(
        prepareRelease(
          github,
          f.archive,
          source,
          expected,
          artifact,
          f.directory,
          f.run
        )
      );
      await assert.rejects(
        readFile(join(f.directory, "radius-bicep-extension.json")),
        /ENOENT/
      );
    });
  }
});

test("only exact ORAS resolver absence permits a write, never auth/transport/ambiguous errors", async () => {
  const missing = Object.assign(new Error("missing"), {
    code: 1,
    stderr: `Error response from registry: failed to fetch the content of "${expected.reference}": ${expected.reference}: not found\n`
  });
  assert.equal(
    await existingManifest(expected.reference, async () => {
      throw missing;
    }),
    null
  );
  for (const stderr of [
    "HTTP 401 Unauthorized",
    "HTTP 403 Forbidden",
    "HTTP 500 Internal server error",
    "network timeout",
    "404 proxy not found",
    "executable not found",
    "MANIFEST_UNKNOWN",
    `Error: ${expected.reference}: not found\nHTTP 403`
  ]) {
    const error = Object.assign(new Error(stderr), { code: 1, stderr });
    await assert.rejects(
      existingManifest(expected.reference, async () => {
        throw error;
      }),
      (actual) => actual === error
    );
  }
});

test("stable and RC full versions publish once; interrupted writes, conflicts and bootstrap remain visible", async (t) => {
  for (const outcome of [
    "RC",
    "stable",
    "retry",
    "conflict",
    "auth",
    "copy failure",
    "verification failure",
    "private",
    "visibility failure"
  ]) {
    await t.test(outcome, async (t) => {
      const f = await fixture(t);
      const target =
        outcome === "stable" ?
          plannedBicepExtensions(
            { ...plan, version: "v0.62.0", releaseType: "final" },
            source.commit
          )[1]
        : expected;
      const receipt = await prepareRelease(
        client(),
        f.archive,
        source,
        target,
        artifact,
        f.directory,
        f.run
      );
      const raw = await readFile(join(f.directory, "manifest.json"), "utf8");
      let remote =
        outcome === "retry" ? raw
        : outcome === "conflict" ? "different"
        : null;
      const writes = [];
      const run = async (args) => {
        assert.equal(args.at(-1), target.reference);
        if (args[1] === "cp") {
          writes.push(args);
          remote = raw;
          if (outcome === "copy failure") throw new Error("upload interrupted");
          return "";
        }
        if (outcome === "auth")
          throw Object.assign(new Error("HTTP 403"), {
            code: 1,
            stderr: "HTTP 403"
          });
        if (remote === null)
          throw Object.assign(new Error("missing"), {
            code: 1,
            stderr: `Error response from registry: failed to fetch the content of "${target.reference}": ${target.reference}: not found`
          });
        return outcome === "verification failure" ? "corrupt" : remote;
      };
      const github = client();
      github.rest.packages.getPackageForOrganization = async () => {
        assert.ok(remote, "Bootstrap checks visibility only after upload");
        if (outcome === "visibility failure")
          throw new Error("visibility HTTP 403");
        return {
          data: { visibility: outcome === "private" ? "private" : "public" }
        };
      };
      const succeeds = ["RC", "stable", "retry"].includes(outcome);
      if (succeeds) {
        assert.deepEqual(
          await publishRelease(github, receipt, f.directory, run),
          receipt.record
        );
        assert.deepEqual(
          JSON.parse(
            await readFile(join(f.directory, "radius-bicep-extension.json"))
          ),
          receipt.record
        );
        assert.equal(receipt.status, "published");
        assert.equal(writes.length, outcome === "retry" ? 0 : 1);
      } else {
        await assert.rejects(publishRelease(github, receipt, f.directory, run));
        await assert.rejects(
          readFile(join(f.directory, "radius-bicep-extension.json")),
          /ENOENT/
        );
        const saved = JSON.parse(
          await readFile(join(f.directory, "receipt.json"))
        );
        assert.equal(
          saved.status,
          ["auth", "conflict"].includes(outcome) ? "prepared"
          : ["private", "visibility failure"].includes(outcome) ? "uploaded"
          : "partial"
        );
        if (["auth", "conflict"].includes(outcome))
          assert.equal(writes.length, 0);
        if (outcome === "copy failure") {
          await publishRelease(client(), receipt, f.directory, async (args) => {
            assert.equal(
              args[2],
              "fetch",
              "Retry must not overwrite the partial successful upload"
            );
            return remote;
          });
          assert.equal(receipt.status, "published");
        }
      }
    });
  }
});

test("script surfaces API/auth failures instead of returning success or regenerating", async (t) => {
  const failures = [];
  const directory = await temp(t);
  await script({
    context: { ...context, ref: "refs/heads/main" },
    github: client(),
    core: {
      getInput: () => directory,
      setFailed: (message) => failures.push(message)
    }
  });
  assert.equal(failures.length, 1);
  await assert.rejects(
    verifyRun(
      {
        rest: {
          actions: {
            getWorkflowRunAttempt: async () => {
              throw new Error("HTTP 403");
            }
          }
        }
      },
      source
    ),
    /HTTP 403/
  );
});

test("reusable workflow stays preparatory, separates generation credentials, and serializes release writers", async () => {
  const yaml = await readFile(
    new URL("../workflows/__publish-release-bicep.yaml", import.meta.url),
    "utf8"
  );
  const active = await readFile(
    new URL("../workflows/build-release.yaml", import.meta.url),
    "utf8"
  );
  const [, bundle, publisher] = yaml.split(/^  (?:bundle|publish):\s*$/m);
  assert.doesNotMatch(active, /uses:.*__publish-release-bicep/);
  assert.doesNotMatch(
    yaml,
    /secrets:|id-token:|--force|azure\/login|ACR|create-github-app-token/
  );
  assert.doesNotMatch(bundle, /packages:\s*write|secrets\.|environment:/);
  assert.match(publisher, /packages:\s*write/);
  assert.match(
    publisher,
    /group: radius-bicep-types-release-\$\{\{ github.ref \}\}/
  );
  assert.match(publisher, /cancel-in-progress: false/);
  assert.match(publisher, /skip-decompress: true/);
  assert.match(publisher, /digest-mismatch: error/);
  assert.doesNotMatch(
    publisher,
    /setup-go|setup-node|generate-bicep-types|publish-extension/
  );
  assert.ok(
    publisher.indexOf("Authenticate original generation") <
      publisher.indexOf("Login to GHCR")
  );
});

test(
  "native Bicep/ORAS snapshot and immutable full-version retry on an isolated local registry",
  {
    skip: !process.env.BICEP_RELEASE_NATIVE_REGISTRY
  },
  async (t) => {
    const registry = process.env.BICEP_RELEASE_NATIVE_REGISTRY;
    assert.match(registry, /^localhost:[0-9]+$/);
    const directory = await temp(t);
    await capture(source, directory, registry);
    const archive = join(directory, artifact.name);
    const nativeArtifact = {
      ...artifact,
      digest: hash(await readFile(archive))
    };
    const writes = [];
    const run = async (args, options) => {
      const mapped = args.map((arg) =>
        arg.replace(
          "ghcr.io/radius-project/bicep-types-radius",
          `${registry}/release-radius`
        )
      );
      if (args[1] === "cp") {
        mapped.splice(2, 0, "--to-plain-http");
        writes.push(args);
      } else {
        mapped.splice(3, 0, "--plain-http");
      }
      try {
        return await tool(mapped, options);
      } catch (error) {
        if (error.stderr)
          error.stderr = error.stderr.replaceAll(
            `${registry}/release-radius`,
            "ghcr.io/radius-project/bicep-types-radius"
          );
        throw error;
      }
    };
    for (const target of [
      expected,
      plannedBicepExtensions(
        { ...plan, version: "v0.62.0", releaseType: "final" },
        source.commit
      )[1]
    ]) {
      const receipt = await prepareRelease(
        client(),
        archive,
        source,
        target,
        nativeArtifact,
        directory
      );
      const before = writes.length;
      await publishRelease(client(), receipt, directory, run);
      await publishRelease(client(), receipt, directory, run);
      const restore = join(directory, target.version);
      await mkdir(restore);
      await mkdir(join(restore, "docker"));
      await writeFile(join(restore, "docker/config.json"), "{}");
      await writeFile(
        join(restore, "bicepconfig.json"),
        JSON.stringify({
          experimentalFeaturesEnabled: { ociEnabled: true },
          cacheRootDirectory: join(restore, "cache"),
          extensions: {
            radius: `br:${registry}/release-radius:${target.version}`
          }
        })
      );
      await writeFile(join(restore, "main.bicep"), "extension radius\n");
      await tool(["bicep", "build", join(restore, "main.bicep")], {
        cwd: restore,
        env: {
          ...process.env,
          DOCKER_CONFIG: join(restore, "docker"),
          BICEP_TRUSTED_REGISTRIES: "localhost"
        }
      });
      assert.equal(
        JSON.parse(await readFile(join(restore, "main.json"))).languageVersion,
        "2.0"
      );
      assert.equal(writes.length, before + 1);
      assert.equal(receipt.status, "published");
      const conflict = {
        ...receipt,
        record: { ...receipt.record, digest: hash("different") }
      };
      await assert.rejects(
        publishRelease(client(), conflict, directory, run),
        /conflicts/
      );
      assert.equal(writes.length, before + 1);
    }
  }
);
