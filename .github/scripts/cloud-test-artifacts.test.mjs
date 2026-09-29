// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import * as bundle from "./cloud-test-artifacts.mjs";
import { setup, testStatus } from "./cloud-test-context.mjs";

const context = {
  repo: { owner: "radius-project", repo: "radius" },
  eventName: "pull_request_target",
  sha: "a".repeat(40),
  runId: 101,
  payload: {
    pull_request: {
      head: { repo: { full_name: "fork/radius" }, sha: "b".repeat(40) }
    }
  }
};
const env = {
  GITHUB_RUN_ATTEMPT: "2",
  GITHUB_REPOSITORY_ID: "340522752",
  CHECKOUT_REPO: "fork/radius",
  CHECKOUT_REF: "b".repeat(40),
  UNIQUE_ID: "func0123456789",
  REL_VERSION: "test-func0123456789-101-1",
  CONTAINER_REGISTRY: bundle.registries.images,
  BICEP_RECIPE_REGISTRY: bundle.registries.recipes,
  TEST_BICEP_TYPES_REGISTRY: bundle.registries.types
};
const source = bundle.sourceIdentity(context, env, 1);
const hash = (data) =>
  `sha256:${createHash("sha256").update(data).digest("hex")}`;
const artifact = {
  id: 7,
  name: "cloud-test-inputs-101-1.tar",
  expired: false,
  size_in_bytes: 10,
  digest: hash("raw"),
  created_at: "2026-09-25T10:10:00Z",
  workflow_run: {
    id: 101,
    repository_id: 340522752,
    head_sha: source.runHeadSHA
  }
};
const readYaml = (file) =>
  JSON.parse(execFileSync("yq", ["-o=json", ".", file], { encoding: "utf8" }));
const workflow = (name) => readYaml(`.github/workflows/${name}`).jobs;
const cloud = workflow("functional-test-cloud.yaml");
const step = (job, name) => job.steps.find((item) => item.name === name);
const save = (file, value) => writeFile(file, JSON.stringify(value));
const core = (inputs) => ({
  getInput: (name) => inputs[name] ?? "",
  setOutput() {},
  summary: {
    addCodeBlock() {
      return this;
    },
    async write() {}
  }
});
async function temporary(t) {
  const directory = await mkdtemp(join(tmpdir(), "cloud-contract-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}

test("run identity distinguishes PR candidates from controllers and manual source branches", () => {
  for (const eventName of [
    "pull_request_target",
    "workflow_dispatch",
    "merge_group"
  ]) {
    const identity = bundle.sourceIdentity({ ...context, eventName }, env);
    assert.equal(identity.commit, env.CHECKOUT_REF);
    assert.equal(identity.workflowSHA, context.sha);
    assert.equal(
      identity.runHeadSHA,
      eventName === "pull_request_target" ? env.CHECKOUT_REF : context.sha
    );
  }
  assert.throws(() =>
    bundle.sourceIdentity(context, { ...env, CHECKOUT_REF: context.sha })
  );
  const verify = (value) =>
    bundle.verifyArtifact(
      value,
      bundle.sourceIdentity(context, env),
      env.GITHUB_REPOSITORY_ID
    );
  assert.equal(verify(artifact), 1);
  for (const override of [
    { name: "../inputs.tar" },
    { expired: true },
    { size_in_bytes: 4 * 1024 ** 3 + 1 },
    { digest: "" },
    { name: "cloud-test-inputs-101-3.tar" },
    ...[{ id: 99 }, { repository_id: 99 }, { head_sha: context.sha }].map(
      (run) => ({
        workflow_run: { ...artifact.workflow_run, ...run }
      })
    )
  ])
    assert.throws(() => verify({ ...artifact, ...override }));
});

test("setup resolves source identity without owning the producer tag", async () => {
  const outputs = {};
  await setup({
    context: {
      ...context,
      eventName: "workflow_dispatch",
      ref: "refs/heads/main",
      payload: {
        repository: { full_name: "radius-project/radius" },
        inputs: { branch: "candidate" }
      }
    },
    github: {
      rest: {
        repos: {
          getBranch: async ({ branch }) => ({
            data: {
              protected: branch === "main",
              commit: {
                sha: branch === "main" ? context.sha : env.CHECKOUT_REF
              }
            }
          })
        }
      }
    },
    core: {
      setOutput: (name, value) => {
        outputs[name] = value;
      }
    }
  });
  assert.match(outputs.UNIQUE_ID, /^func[a-f0-9]{10}$/);
  assert.equal(outputs.CHECKOUT_REF, env.CHECKOUT_REF);
  assert.equal(outputs.TRUSTED_SHA, context.sha);
  assert.equal(Object.hasOwn(outputs, "REL_VERSION"), false);
});

test("producer tags bind the unique ID, run and producing attempt", () => {
  assert.equal(bundle.producerTag(env.UNIQUE_ID, source), env.REL_VERSION);
  assert.equal(
    bundle.producerTag(env.UNIQUE_ID, { ...source, generationAttempt: 2 }),
    "test-func0123456789-101-2"
  );
  for (const uniqueID of ["", "funcABCDEF0123", "functional", "../escape"])
    assert.throws(() => bundle.producerTag(uniqueID, source));
  for (const value of [0, -1, "01", "x", Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() =>
      bundle.producerTag(env.UNIQUE_ID, { ...source, runId: value })
    );
    assert.throws(() =>
      bundle.producerTag(env.UNIQUE_ID, { ...source, generationAttempt: value })
    );
  }
});

test("selection authenticates attempts, reuses earlier successful inputs and surfaces API failures", async (t) => {
  const directory = await temporary(t);
  const run = {
    id: 101,
    run_attempt: 1,
    head_sha: env.CHECKOUT_REF,
    event: context.eventName,
    repository: { full_name: "radius-project/radius" },
    path: ".github/workflows/functional-test-cloud.yaml",
    run_started_at: "2026-09-25T10:00:00Z"
  };
  const actions = {
    getArtifact: async () => ({ data: artifact }),
    getWorkflowRunAttempt: async ({ attempt_number }) => ({
      data:
        attempt_number === 1 ? run : (
          { ...run, run_started_at: "2026-09-25T11:00:00Z" }
        )
    })
  };
  const args = {
    context,
    core: core({ artifact_id: "7", directory }),
    github: { rest: { actions } }
  };
  await bundle.select(args, env);
  assert.deepEqual(
    JSON.parse(await readFile(join(directory, "selected.json"))).source,
    source
  );
  for (const override of [
    { REL_VERSION: "pr-func0123456789" },
    { REL_VERSION: "test-func0123456789-101-2" },
    { REL_VERSION: "test-func0123456789-102-1" },
    { UNIQUE_ID: "func9876543210" }
  ]) {
    await assert.rejects(
      bundle.select(args, { ...env, ...override }),
      /authenticated producer/
    );
  }
  for (const field of ["event", "path"]) {
    const original = run[field];
    run[field] = "unexpected";
    await assert.rejects(bundle.select(args, env), /attempt mismatch/);
    run[field] = original;
  }
  run.head_sha = context.sha;
  await assert.rejects(bundle.select(args, env), /attempt mismatch/);
  actions.getArtifact = async () => {
    throw new Error("HTTP 403");
  };
  await assert.rejects(bundle.select(args, env), /HTTP 403/);
});

function payload() {
  const roots = [],
    manifests = new Map();
  const names = [
    ...bundle.images.map((name) => `images-${name}`),
    "types-radius",
    "recipes-dynamicrp_recipe"
  ];
  for (const name of names) {
    const kind = name.split("-")[0],
      prefix = `application/vnd.ms.bicep.${kind === "types" ? "provider" : "module"}.`;
    const blob = (mediaType) => ({ mediaType, digest: hash("{}"), size: 2 });
    const manifest = {
      schemaVersion: 2,
      mediaType: "application/vnd.oci.image.manifest.v1+json",
      config: blob(
        kind === "images" ?
          "application/vnd.oci.image.config.v1+json"
        : `${prefix}config.v1+json`
      ),
      layers: [
        blob(
          kind === "images" ?
            "application/vnd.oci.image.layer.v1.tar+gzip"
          : `${prefix}layer.v1${kind === "types" ? ".tar+gzip" : "+json"}`
        )
      ]
    };
    if (kind === "types") manifest.artifactType = `${prefix}artifact`;
    const raw = JSON.stringify(manifest),
      digest = hash(raw);
    manifests.set(digest, raw);
    roots.push({
      mediaType: manifest.mediaType,
      digest,
      size: raw.length,
      annotations: { "org.opencontainers.image.ref.name": name }
    });
  }
  return { roots, manifests };
}

test("closed targets/media types preserve images and underscore recipes, excluding foreign/executable data", () => {
  const { roots, manifests } = payload();
  const targets = bundle.destinations(roots, env.REL_VERSION);
  assert.equal(targets.filter((item) => item.kind === "images").length, 8);
  assert.match(
    targets.at(-1).reference,
    /\/test\/testrecipes\/test-bicep-recipes\/dynamicrp_recipe:test-func0123456789-101-1$/
  );
  for (const name of [
    "images-unknown",
    "types-aws",
    "recipes-../escape",
    "production-radius"
  ]) {
    const changed = structuredClone(roots);
    changed[0].annotations["org.opencontainers.image.ref.name"] = name;
    assert.throws(() => bundle.destinations(changed, env.REL_VERSION));
  }
  assert.throws(() => bundle.destinations(roots, "latest"));
  for (const tag of [
    "edge",
    "pr-func0123456789",
    "test-local",
    "test-func0123456789-0-1"
  ])
    assert.throws(() => bundle.destinations(roots, tag));
  assert.throws(() => bundle.destinations(roots.slice(1), env.REL_VERSION));
  for (const entry of targets)
    bundle.validateManifest(manifests.get(entry.digest), entry);
  const entry = targets.find((item) => item.kind === "types");
  for (const override of [
    { subject: {} },
    { config: { digest: hash("executable") } },
    { layers: [{ mediaType: "application/executable" }] },
    {
      layers: [
        {
          mediaType: "application/vnd.ms.bicep.provider.layer.v1.tar+gzip",
          digest: hash("{}"),
          size: 2,
          urls: ["https://foreign.invalid/blob"]
        }
      ]
    }
  ]) {
    const raw = JSON.stringify({
      ...JSON.parse(manifests.get(entry.digest)),
      ...override
    });
    assert.throws(() =>
      bundle.validateManifest(raw, { ...entry, digest: hash(raw) })
    );
  }
});

test("capture assigns a new tag and archive to each producer attempt with retained setup identity", async (t) => {
  const directory = await temporary(t);
  const { roots } = payload();
  for (const attempt of [1, 2]) {
    const producer = bundle.sourceIdentity(context, env, attempt);
    const tag = bundle.producerTag(env.UNIQUE_ID, producer);
    const calls = [];
    const run = async (args) => {
      calls.push(args);
      if (args[0] === "oras") {
        const layout = args.at(-1).split(":")[0];
        await save(join(layout, "index.json"), { manifests: roots });
      } else {
        assert.equal(args[0], "tar");
        assert.equal(
          args[2],
          join(directory, `cloud-test-inputs-101-${attempt}.tar`)
        );
        assert.deepEqual(
          JSON.parse(await readFile(join(args[4], "source.json"))),
          { source: producer, tag }
        );
      }
    };
    await bundle.capture(
      { core: core({ directory }), context },
      {
        ...env,
        GITHUB_RUN_ATTEMPT: String(attempt),
        REL_VERSION: tag
      },
      run
    );
    assert.ok(calls.length > 1);
    assert.ok(
      calls
        .filter((args) => args[0] === "oras")
        .every(
          (args) =>
            args[3].startsWith("localhost:5000/") && args[3].endsWith(`:${tag}`)
        )
    );
  }
  await assert.rejects(
    bundle.capture({ core: core({ directory }), context }, env, async () =>
      assert.fail("Wrong producing-attempt tag must fail before tool execution")
    ),
    /authenticated producer/
  );
});

test("preparation rejects mismatched sources before writes and publication records only verified copies", async (t) => {
  const directory = await temporary(t),
    { roots, manifests } = payload(),
    calls = [];
  await mkdir(join(directory, "download"));
  await writeFile(
    join(directory, "download", artifact.name),
    "x".repeat(artifact.size_in_bytes)
  );
  await save(join(directory, "selected.json"), { source, artifact });
  const metadata = { source, tag: env.REL_VERSION };
  const run = async (args) => {
    calls.push(args);
    if (args[0] === "tar")
      return JSON.stringify(
        args.at(-1) === "source.json" ?
          metadata
        : { schemaVersion: 2, manifests: roots }
      );
    const target = bundle
      .destinations(roots, env.REL_VERSION)
      .find((item) => item.reference === args.at(-1));
    return args[2] === "fetch" ?
        manifests.get(target?.digest ?? args.at(-1).split("@")[1])
      : "";
  };
  const args = { context, core: core({ directory, destination: "ghcr" }) };
  metadata.source = { ...source, commit: context.sha };
  await assert.rejects(
    bundle.prepare(args, env, run),
    /source metadata mismatch/
  );
  assert.equal(calls.filter((cmd) => cmd[1] === "cp").length, 0);
  metadata.source = source;
  for (const tag of [
    "pr-func0123456789",
    "test-func0123456789-101-2",
    "test-func0123456789-102-1",
    "test-func9876543210-101-1"
  ]) {
    metadata.tag = tag;
    await assert.rejects(
      bundle.prepare(args, env, run),
      /authenticated producer/
    );
  }
  metadata.tag = env.REL_VERSION;
  await assert.rejects(
    bundle.prepare(
      args,
      { ...env, REL_VERSION: "test-func0123456789-101-2" },
      run
    ),
    /output tag mismatch/
  );
  assert.equal(calls.filter((cmd) => cmd[1] === "cp").length, 0);
  await bundle.prepare(args, env, run);
  assert.ok(
    calls
      .filter((cmd) => cmd[1] === "cp")
      .every((cmd) => cmd.includes("--to-oci-layout"))
  );
  const receiptPath = join(directory, "receipt.json");
  const receipt = JSON.parse(await readFile(receiptPath));
  await save(receiptPath, { ...receipt, tag: "test-func0123456789-101-2" });
  await assert.rejects(
    bundle.publish(args, env, async () =>
      assert.fail("No writes for a modified receipt")
    ),
    /authenticated producer/
  );
  await save(receiptPath, receipt);
  for (const destination of ["ghcr", "acr"]) {
    args.core = core({ directory, destination });
    await bundle.publish(args, env, run);
  }
  assert.equal(
    JSON.parse(await readFile(join(directory, "receipt.json"))).published
      .length,
    roots.length
  );
  assert.ok(
    calls
      .filter((cmd) => cmd[1] === "cp" && !cmd.includes("--to-oci-layout"))
      .every((cmd) => cmd.at(-1).endsWith(`:${env.REL_VERSION}`))
  );
  await assert.rejects(
    bundle.publish(args, env, async () => {
      throw new Error("copy failed");
    }),
    /copy failed/
  );
  await assert.rejects(
    bundle.publish(args, env, async () => "{}"),
    /digest changed/
  );
});

test("uploads retain the authenticated producer tag across full and downstream-only reruns", async (t) => {
  for (const [producingAttempt, retryAttempt] of [
    [1, 1],
    [1, 2],
    [2, 2],
    [2, 3]
  ]) {
    await t.test(
      `producer ${producingAttempt}, current attempt ${retryAttempt}`,
      async (t) => {
        const directory = await temporary(t);
        const producer = bundle.sourceIdentity(context, env, producingAttempt);
        const tag = bundle.producerTag(env.UNIQUE_ID, producer);
        const settings = {
          ...env,
          GITHUB_RUN_ATTEMPT: String(retryAttempt),
          REL_VERSION: tag
        };
        const input = {
          ...artifact,
          name: `cloud-test-inputs-101-${producingAttempt}.tar`
        };
        const args = {
          context,
          core: core({ artifact_id: "7", directory, destination: "ghcr" }),
          github: {
            rest: {
              actions: {
                getArtifact: async () => ({ data: input }),
                getWorkflowRunAttempt: async ({ attempt_number }) => ({
                  data: {
                    id: 101,
                    run_attempt: attempt_number,
                    head_sha: env.CHECKOUT_REF,
                    event: context.eventName,
                    repository: { full_name: "radius-project/radius" },
                    path: ".github/workflows/functional-test-cloud.yaml",
                    run_started_at:
                      attempt_number === producingAttempt ?
                        "2026-09-25T10:00:00Z"
                      : "2026-09-25T11:00:00Z"
                  }
                })
              }
            }
          }
        };
        await bundle.select(args, settings);
        await mkdir(join(directory, "download"));
        await writeFile(
          join(directory, "download", input.name),
          "x".repeat(input.size_in_bytes)
        );
        const { roots, manifests } = payload();
        const copies = [];
        const targets = bundle.destinations(roots, tag);
        const run = async (cmd) => {
          if (cmd[0] === "tar")
            return JSON.stringify(
              cmd.at(-1) === "source.json" ?
                { source: producer, tag }
              : { schemaVersion: 2, manifests: roots }
            );
          if (cmd[1] === "cp") {
            if (!cmd.includes("--to-oci-layout")) copies.push(cmd.at(-1));
            return "";
          }
          const target = targets.find((item) => item.reference === cmd.at(-1));
          return manifests.get(target?.digest ?? cmd.at(-1).split("@")[1]);
        };
        await bundle.prepare(args, settings, run);
        for (const destination of ["ghcr", "acr"]) {
          args.core = core({ directory, destination });
          await bundle.publish(args, settings, run);
        }
        assert.deepEqual(
          copies.sort(),
          targets.map((item) => item.reference).sort()
        );
        const receipt = JSON.parse(
          await readFile(join(directory, "receipt.json"))
        );
        assert.equal(receipt.tag, tag);
        assert.deepEqual(receipt.source, producer);
        assert.equal(receipt.published.length, roots.length);
      }
    );
  }
});

test("summary fails closed on incomplete or cancelled work", () => {
  const phases = [
    "authorize",
    "setup",
    "changes",
    "announce",
    "build",
    "upload-ghcr",
    "upload-test-types",
    "tests"
  ];
  const needs = Object.fromEntries(
    phases.map((key) => [
      key,
      { result: "success", outputs: { only_changed: "false" } }
    ])
  );
  assert.equal(testStatus(needs), "success");
  for (const result of ["failure", "cancelled", "skipped"]) {
    needs["upload-ghcr"].result = result;
    assert.equal(
      testStatus(needs),
      result === "cancelled" ? "cancelled" : "failure"
    );
  }
  needs.changes.outputs.only_changed = "true";
  for (const key of phases.slice(3)) needs[key].result = "skipped";
  assert.equal(testStatus(needs), "success");
  needs.changes.result = "failure";
  assert.equal(testStatus(needs), "failure");
});

test("real workflows preserve credential boundaries and require native digest verification", () => {
  assert.deepEqual(cloud.build.permissions, { contents: "read" });
  assert.doesNotMatch(
    JSON.stringify(cloud.build),
    /secrets\.|azure\/login|docker\/login/
  );
  assert.deepEqual(cloud.tests.permissions, {
    "id-token": "write",
    contents: "read",
    checks: "write",
    packages: "read",
    "pull-requests": "write"
  });
  assert.doesNotMatch(
    JSON.stringify(cloud.tests),
    /FUNCTIONAL_TEST_APP_PRIVATE_KEY|BICEPTYPES_CLIENT_ID/
  );
  assert.equal(
    step(cloud.tests, "Process Functional Test Results").uses,
    "./.github/actions/process-test-results"
  );
  assert.match(
    JSON.stringify(cloud.announce),
    /FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN/
  );
  for (const name of ["upload-ghcr", "upload-test-types"]) {
    assert.equal(
      cloud[name].steps[0].with.ref,
      "${{ needs.setup.outputs.TRUSTED_SHA }}"
    );
    const download = cloud[name].steps.find((item) =>
      item.uses?.startsWith("actions/download-artifact")
    );
    assert.equal(download.with["skip-decompress"], true);
    assert.equal(download.with["digest-mismatch"], "error");
    assert.equal(
      download.with["artifact-ids"],
      "${{ steps.select.outputs.artifact_id }}"
    );
  }
  assert.equal(cloud["upload-ghcr"].permissions["id-token"], undefined);
  assert.equal(cloud["upload-test-types"].permissions.packages, undefined);
  assert.deepEqual(cloud["upload-ghcr"].permissions, {
    contents: "read",
    actions: "read",
    packages: "write"
  });
  assert.deepEqual(cloud["upload-test-types"].permissions, {
    contents: "read",
    actions: "read",
    "id-token": "write"
  });
  assert.doesNotMatch(
    JSON.stringify(cloud["upload-ghcr"]),
    /secrets\.|azure\/login/
  );
  assert.doesNotMatch(
    JSON.stringify(cloud["upload-test-types"]),
    /FUNCTIONAL_TEST_APP_PRIVATE_KEY|BICEPTYPES_CLIENT_ID|docker\/login/
  );
  for (const job of ["upload-ghcr", "upload-test-types"]) {
    assert.ok(cloud[job].needs.includes("build"));
    assert.match(cloud[job].if, /needs\.build\.result == 'success'/);
  }
  for (const job of ["upload-ghcr", "upload-test-types"]) {
    assert.ok(cloud.tests.needs.includes(job));
    assert.ok(cloud.tests.if.includes(`needs.${job}.result == 'success'`));
    assert.ok(cloud["report-test-results"].needs.includes(job));
  }
  assert.equal(
    step(cloud.tests, "Run functional tests").env.GH_TOKEN,
    "${{ secrets.FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN }}"
  );
});

test("workflow consumers use producer outputs, not setup or the retry attempt", () => {
  assert.equal(Object.hasOwn(cloud.setup.outputs, "REL_VERSION"), false);
  assert.equal(
    cloud.build.env.REL_VERSION,
    "test-${{ needs.setup.outputs.UNIQUE_ID }}-${{ github.run_id }}-${{ github.run_attempt }}"
  );
  assert.equal(
    cloud.build.outputs.REL_VERSION,
    "${{ steps.test-image-version.outputs.REL_VERSION }}"
  );
  const record = step(cloud.build, "Record immutable test image version");
  assert.ok(
    cloud.build.steps.indexOf(record) <
      cloud.build.steps.findIndex(
        (item) => item.with?.["allow-unsafe-pr-checkout"]
      )
  );
  for (const job of ["upload-ghcr", "upload-test-types", "tests"]) {
    assert.equal(
      cloud[job].env.REL_VERSION,
      "${{ needs.build.outputs.REL_VERSION }}"
    );
    assert.equal(
      cloud[job].env.UNIQUE_ID,
      "${{ needs.setup.outputs.UNIQUE_ID }}"
    );
  }
  assert.equal(
    cloud.tests.env.BICEP_RECIPE_TAG_VERSION,
    "${{ needs.build.outputs.REL_VERSION }}"
  );
  assert.doesNotMatch(JSON.stringify(cloud.announce), /REL_VERSION/);
  assert.equal(
    step(cloud["report-test-results"], "Aggregate functional test status").env
      .REL_VERSION,
    "${{ needs.build.outputs.REL_VERSION }}"
  );
  assert.match(
    step(cloud["report-test-results"], "Comment final status").with.message,
    /needs\.build\.outputs\.REL_VERSION/
  );
  assert.doesNotMatch(
    JSON.stringify(cloud),
    /needs\.setup\.outputs\.REL_VERSION/
  );
});

test("TLS-local setup precedes the candidate and keeps the existing generated config", async (t) => {
  const steps = cloud.build.steps,
    directory = await temporary(t);
  const registry = step(cloud.build, "Create a job-local registry");
  assert.equal(steps[0].with.ref, "refs/heads/main");
  assert.equal(registry.with.secure, "true");
  assert.ok(
    steps.indexOf(registry) <
      steps.findIndex((item) => item.with?.["allow-unsafe-pr-checkout"])
  );
  await mkdir(join(directory, "test"));
  execFileSync(
    "bash",
    ["-e", "-c", step(cloud.build, "Generate test bicepconfig.json").run],
    {
      cwd: directory,
      env: {
        ...process.env,
        REL_VERSION: env.REL_VERSION,
        BICEP_TYPES_REGISTRY: "biceptypes.azurecr.io"
      }
    }
  );
  const config = JSON.parse(
    await readFile(join(directory, "test/bicepconfig.json"))
  );
  assert.equal(
    config.extensions.radius,
    `br:localhost:5000/test/radius:${env.REL_VERSION}`
  );
  assert.notEqual(config.experimentalFeaturesEnabled?.ociEnabled, true);
});

test("certificate properties and invalid-hostname rejection remain intact", async (t) => {
  const directory = await temporary(t);
  const action = readYaml(".github/actions/create-local-registry/action.yaml");
  const script = step(
    { steps: action.runs.steps },
    "Create certificates for local registry"
  ).run;
  const settings = {
    ...process.env,
    INPUT_REGISTRY_NAME: "radius-registry",
    INPUT_REGISTRY_SERVER: "localhost",
    STEPS_CREATE_TEMP_CERT_DIR_OUTPUTS_TEMP_CERT_DIR: directory
  };
  execFileSync("bash", ["-e", "-c", script], { env: settings, stdio: "pipe" });
  const cert = execFileSync(
    "openssl",
    [
      "x509",
      "-in",
      join(directory, "certs/localhost/client.crt"),
      "-noout",
      "-text"
    ],
    { encoding: "utf8" }
  );
  for (const property of [
    /CN\s*=\s*localhost/,
    /DNS:radius-registry, DNS:localhost/,
    /4096 bit/,
    /sha256WithRSAEncryption/,
    /Basic Constraints: critical\s+CA:TRUE/,
    /Digital Signature, Certificate Sign, CRL Sign/
  ])
    assert.match(cert, property);
  assert.notEqual(
    spawnSync("bash", ["-e", "-c", script], {
      env: {
        ...settings,
        INPUT_REGISTRY_NAME: "localhost\n::set-output name=bad::true"
      }
    }).status,
    0
  );
});
