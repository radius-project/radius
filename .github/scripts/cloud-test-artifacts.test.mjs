// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { cp, mkdtemp, mkdir, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { test } from "node:test";
import { setup } from "./cloud-test-context.mjs";

const readYaml = (file) =>
  JSON.parse(execFileSync("yq", ["-o=json", ".", file], { encoding: "utf8" }));
const workflow = readYaml(".github/workflows/functional-test-cloud.yaml");
const cloud = workflow.jobs;
const step = (job, name) => job.steps.find((item) => item.name === name);
const tag = "test-func0123456789-101-1";
async function temporary(t) {
  const directory = await mkdtemp(join(tmpdir(), "cloud-contract-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}

function trustedDependencies(jobs) {
  const manifest = [];
  for (const [job, value] of Object.entries(jobs)) {
    const active = new Map();
    const dependency = (path) => {
      const prefix = [...active.keys()]
        .filter((key) => key === "" || path.startsWith(`${key}/`))
        .sort((a, b) => b.length - a.length)[0];
      assert.notEqual(prefix, undefined, `No checkout for ${job}: ${path}`);
      return {
        checkout: active.get(prefix),
        file: prefix ? path.slice(prefix.length + 1) : path
      };
    };
    const addImport = (path, names) => {
      const { checkout, file } = dependency(path);
      assert.ok(
        checkout,
        `Executable helper came from candidate checkout: ${job}/${path}`
      );
      if (checkout.sparse) assert.equal(file, checkout.sparse);
      checkout.imports ??= [];
      let entry = checkout.imports.find((item) => item.file === file);
      if (!entry) {
        entry = { file, exports: [] };
        checkout.imports.push(entry);
      }
      entry.exports.push(...names);
    };
    for (const item of value.steps ?? []) {
      if (item.uses?.startsWith("actions/checkout@")) {
        const path = item.with.path ?? "";
        if (item.with["allow-unsafe-pr-checkout"]) {
          active.set(path, null);
          continue;
        }
        assert.ok(
          ["radius-project/radius", "${{ github.repository }}"].includes(
            item.with.repository
          )
        );
        assert.equal(item.with["persist-credentials"], false);
        const trusted = "${{ needs.setup.outputs.TRUSTED_SHA }}";
        const expectedRef =
          job === "setup" || (job === "build" && !path) ? "refs/heads/main"
          : job === "tests" ? "${{ github.sha }}"
          : job === "report-test-results" ?
            "${{ needs.setup.outputs.TRUSTED_SHA || 'refs/heads/main' }}"
          : trusted;
        assert.equal(
          item.with.ref,
          expectedRef,
          `Untrusted checkout in ${job}: ${path}`
        );
        const checkout = { job, ref: item.with.ref };
        if (path) checkout.path = path;
        if (item.with["sparse-checkout"]) {
          checkout.sparse = item.with["sparse-checkout"];
          assert.equal(item.with["sparse-checkout-cone-mode"], false);
        }
        active.set(path, checkout);
        manifest.push(checkout);
      }
      if (item.uses?.startsWith("./")) {
        const { checkout } = dependency(item.uses.slice(2));
        if (checkout) (checkout.actions ??= []).push(item.uses.slice(2));
      }
      const script = item.with?.script ?? "";
      if (script.includes("import(")) {
        const match =
          /const \{ ([^}]+) \} = await import\(`\$\{process\.env\.GITHUB_WORKSPACE\}\/([^`]+)`\)/.exec(
            script
          );
        assert.ok(match, `Unrecognized helper import in ${job}/${item.name}`);
        assert.equal(script.match(/import\(/g).length, 1);
        addImport(
          match[2],
          match[1].split(",").map((name) => name.trim())
        );
      }
      if (item.env?.AZURE_LOGIN_SCRIPT) {
        assert.equal(item.run, 'node "${AZURE_LOGIN_SCRIPT}"');
        const prefix = "${{ github.workspace }}/";
        assert.ok(item.env.AZURE_LOGIN_SCRIPT.startsWith(prefix));
        addImport(item.env.AZURE_LOGIN_SCRIPT.slice(prefix.length), ["login"]);
      }
      if (item.run === "make install-oras") {
        const { checkout } = dependency("Makefile");
        assert.ok(checkout);
        checkout.make = "install-oras";
      }
    }
  }
  return manifest;
}

async function checkPreparedDependencies(manifest, base) {
  for (const checkout of manifest) {
    for (const entry of checkout.imports ?? []) {
      execFileSync(
        process.execPath,
        [
          "--input-type=module",
          "-e",
          `const module = await import(${JSON.stringify(pathToFileURL(join(base, entry.file)).href)});
         for (const name of ${JSON.stringify(entry.exports)}) {
           if (typeof module[name] !== "function") throw new Error("Missing helper export: " + name);
         }`
        ],
        { stdio: "pipe" }
      );
    }
    for (const action of checkout.actions ?? []) {
      assert.equal(
        readYaml(join(base, action, "action.yaml")).runs.using,
        "composite"
      );
    }
    if (checkout.make) {
      const command = execFileSync("make", ["-n", checkout.make], {
        cwd: base,
        encoding: "utf8",
        stdio: "pipe",
        env: {
          ...process.env,
          GIT_COMMIT: "a".repeat(40),
          GIT_VERSION: "prepared-base"
        }
      });
      assert.match(command, /build\/scripts\/install-oras\.sh/);
    }
  }
}

test("actual activation checkout refs and helper imports work against a separate prepared base", async (t) => {
  const manifest = trustedDependencies(cloud);
  const source =
    process.env.CLOUD_TEST_PREPARED_BASE ??
    resolve(import.meta.dirname, "../..");
  const base = await temporary(t);
  const files = new Set(["Makefile", "build"]);
  for (const checkout of manifest) {
    for (const entry of checkout.imports ?? []) files.add(entry.file);
    for (const action of checkout.actions ?? []) files.add(action);
  }
  for (const file of files) {
    await mkdir(dirname(join(base, file)), { recursive: true });
    await cp(join(source, file), join(base, file), { recursive: true });
  }
  await checkPreparedDependencies(manifest, base);
  for (const [job, value] of Object.entries(cloud)) {
    if (job === "tests") continue; // Its workflow-SHA helper is intentionally not an uploader.
    for (const [index, item] of (value.steps ?? []).entries()) {
      if (
        !item.uses?.startsWith("actions/checkout@") ||
        item.with["allow-unsafe-pr-checkout"]
      )
        continue;
      for (const ref of ["${{ env.CHECKOUT_REF }}", "${{ github.sha }}"]) {
        const unsafe = structuredClone(cloud);
        unsafe[job].steps[index].with.ref = ref;
        assert.throws(() => trustedDependencies(unsafe), /Untrusted checkout/);
      }
    }
  }
  const missing = ".github/scripts/cloud-test-context.mjs";
  await rm(join(base, missing));
  await assert.rejects(checkPreparedDependencies(manifest, base), (error) => {
    const output = `${error.message}\n${error.stderr ?? ""}`;
    assert.match(output, /ERR_MODULE_NOT_FOUND/);
    assert.ok(output.includes(missing));
    return true;
  });
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
    "${{ steps.private-module-token.outputs.token }}"
  );
});

test("approved tests share Test ACR write access while GHCR publication stays isolated", () => {
  const uploader = cloud["upload-test-types"];
  const login = step(uploader, "Login to Test-tenant Azure");
  assert.equal(uploader.environment, undefined);
  assert.equal(
    login.env.AZURE_CLIENT_ID,
    "${{ secrets.AZURE_SP_TESTS_APPID }}"
  );
  assert.equal(
    login.env.AZURE_TENANT_ID,
    "${{ secrets.AZURE_SP_TESTS_TENANTID }}"
  );
  assert.equal(
    login.env.AZURE_SUBSCRIPTION_ID,
    "${{ secrets.AZURE_SUBSCRIPTIONID_TESTS }}"
  );
  assert.equal(login.if, undefined);
  assert.equal(login["continue-on-error"], undefined);
  const ordered = [
    "Select the exact producing artifact",
    "Download immutable OCI data",
    "Validate source and import OCI data",
    "Login to Test-tenant Azure",
    "Login only to the private test registry",
    "Copy validated Radius test types"
  ].map((name) => uploader.steps.indexOf(step(uploader, name)));
  assert.ok(
    ordered.every(
      (index, i) => index >= 0 && (i === 0 || index > ordered[i - 1])
    )
  );
  assert.equal(
    step(uploader, "Validate source and import OCI data").if,
    undefined
  );
  assert.equal(
    step(uploader, "Validate source and import OCI data")["continue-on-error"],
    undefined
  );
  assert.equal(
    step(cloud.tests, "Login to Azure").env.AZURE_CLIENT_ID,
    "${{ secrets.AZURE_SP_TESTS_APPID }}"
  );
  assert.equal(cloud.tests.environment, undefined);
  assert.doesNotMatch(
    JSON.stringify(cloud),
    /AZURE_TEST_TYPES_PUBLISHER_CLIENT_ID|cloud-test-types-publish|Require dedicated Test ACR publisher identity/
  );
  assert.equal(cloud.tests.permissions.packages, "read");
  assert.equal(cloud["upload-ghcr"].permissions.packages, "write");
  assert.deepEqual(
    Object.entries(cloud)
      .filter(([, job]) => job.permissions?.packages === "write")
      .map(([name]) => name),
    ["upload-ghcr"]
  );
  assert.equal(
    cloud["approval-gate"].environment,
    "external-contributor-approval"
  );
  assert.doesNotMatch(JSON.stringify(uploader), /BICEPTYPES_CLIENT_ID/);
});

test("upload jobs use repository registry settings without candidate overrides", () => {
  const settings = {
    FUNCTIONAL_TEST_CONTAINER_REGISTRY: "ghcr.io/radius-project/dev",
    FUNCTIONAL_TEST_BICEP_RECIPE_REGISTRY: "ghcr.io/radius-project/dev",
    TEST_BICEP_TYPES_REGISTRY: "testregistry123.azurecr.io"
  };
  const expected = {
    CONTAINER_REGISTRY: settings.FUNCTIONAL_TEST_CONTAINER_REGISTRY,
    BICEP_RECIPE_REGISTRY: settings.FUNCTIONAL_TEST_BICEP_RECIPE_REGISTRY,
    TEST_BICEP_TYPES_REGISTRY: settings.TEST_BICEP_TYPES_REGISTRY
  };
  for (const job of ["upload-ghcr", "upload-test-types"]) {
    const effective = { ...workflow.env, ...cloud[job].env };
    for (const [name, value] of Object.entries(expected)) {
      const match = /^\$\{\{ vars\.([A-Z_]+) \}\}$/.exec(effective[name]);
      assert.ok(match, `${job} must read ${name} from repository settings`);
      assert.equal(settings[match[1]], value);
    }
  }
  assert.equal(
    step(cloud["upload-ghcr"], "Copy validated images and recipes").env
      .INPUT_DESTINATION,
    "ghcr"
  );
  assert.equal(
    step(cloud["upload-test-types"], "Copy validated Radius test types").env
      .INPUT_DESTINATION,
    "acr"
  );
});

test("actual Test ACR login validates and derives the configured registry name", () => {
  const login = step(
    cloud["upload-test-types"],
    "Login only to the private test registry"
  );
  assert.equal(login.env?.TEST_BICEP_TYPES_REGISTRY, undefined);
  assert.doesNotMatch(login.run, /\$\{\{/);
  const script = `az() { printf 'az-argument:%s\\n' "$@"; }\n${login.run}`;
  for (const name of [
    "testregistry123",
    "anotherregistry456",
    "abcde",
    "a".repeat(50)
  ]) {
    const result = spawnSync("bash", ["-e", "-c", script], {
      env: { ...process.env, TEST_BICEP_TYPES_REGISTRY: `${name}.azurecr.io` },
      encoding: "utf8"
    });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(
      result.stdout,
      `az-argument:acr\naz-argument:login\naz-argument:--name\naz-argument:${name}\n`
    );
  }
  for (const hostname of [
    undefined,
    "",
    "abcd.azurecr.io",
    `${"a".repeat(51)}.azurecr.io`,
    "https://testregistry123.azurecr.io",
    "testregistry123.azurecr.io/test/radius",
    "testregistry123.azurecr.io:443",
    "UPPER.azurecr.io",
    "test-registry.azurecr.io",
    "testregistry123.azurecr.io.evil.invalid",
    "testregistry123.azurecr.io\n",
    "$(echo injected).azurecr.io",
    "testregistry123.azurecr.cn"
  ]) {
    const env = { ...process.env };
    delete env.TEST_BICEP_TYPES_REGISTRY;
    if (hostname !== undefined) env.TEST_BICEP_TYPES_REGISTRY = hostname;
    const result = spawnSync("bash", ["-e", "-c", script], {
      env,
      encoding: "utf8"
    });
    assert.equal(result.status, 1, hostname);
    assert.match(result.stdout, /::error::TEST_BICEP_TYPES_REGISTRY must be/);
    assert.doesNotMatch(result.stdout, /az-argument:|injected/);
  }
});

test("merge-group Test ACR uploader executes the trusted helper, not the candidate revision", async () => {
  const trustedSHA = "c".repeat(40);
  const candidateSHA = "d".repeat(40);
  const outputs = {};
  await setup({
    context: {
      eventName: "merge_group",
      sha: candidateSHA,
      ref: "refs/heads/gh-readonly-queue/main/pr-123",
      payload: {
        repository: { full_name: "radius-project/radius" },
        merge_group: { base_ref: "refs/heads/main", head_sha: candidateSHA }
      }
    },
    github: {
      rest: {
        repos: {
          getBranch: async () => ({
            data: { protected: true, commit: { sha: trustedSHA } }
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
  assert.equal(outputs.CHECKOUT_REF, candidateSHA);
  assert.equal(outputs.TRUSTED_SHA, trustedSHA);
  assert.notEqual(outputs.TRUSTED_SHA, outputs.CHECKOUT_REF);
  const uploader = cloud["upload-test-types"];
  const uploaderCheckouts = uploader.steps.filter((item) =>
    item.uses?.startsWith("actions/checkout@")
  );
  assert.equal(uploaderCheckouts.length, 2);
  for (const checkout of uploaderCheckouts) {
    assert.equal(checkout.with.ref, "${{ needs.setup.outputs.TRUSTED_SHA }}");
    assert.equal(checkout.with["persist-credentials"], false);
    assert.equal(checkout.with["allow-unsafe-pr-checkout"], undefined);
  }
  const helper = step(uploader, "Checkout workflow Azure login script");
  assert.equal(helper.with.path, ".workflow-azure-login");
  assert.equal(
    helper.with["sparse-checkout"],
    ".github/scripts/azure-oidc-login.mjs"
  );
  const login = step(uploader, "Login to Test-tenant Azure");
  assert.equal(login.run, 'node "${AZURE_LOGIN_SCRIPT}"');
  assert.equal(
    login.env.AZURE_LOGIN_SCRIPT,
    "${{ github.workspace }}/.workflow-azure-login/.github/scripts/azure-oidc-login.mjs"
  );
  assert.ok(uploader.steps.indexOf(helper) < uploader.steps.indexOf(login));
  assert.equal(
    step(cloud.tests, "Checkout workflow Azure login script").with.ref,
    "${{ github.sha }}"
  );
});

test("private-module App token is read-only, job-local and minted immediately before tests", () => {
  const steps = cloud.tests.steps;
  const token = step(cloud.tests, "Create private-module read token");
  const check = step(cloud.tests, "Require private-module App credentials");
  const run = step(cloud.tests, "Run functional tests");
  assert.deepEqual(check.env, {
    MODULE_CLIENT_ID:
      "${{ secrets.RADIUS_TERRAFORM_PRIVATE_MODULES_CLIENT_ID }}",
    MODULE_PRIVATE_KEY:
      "${{ secrets.RADIUS_TERRAFORM_PRIVATE_MODULES_PRIVATE_KEY }}"
  });
  assert.equal(check.if, undefined);
  assert.equal(check["continue-on-error"], undefined);
  assert.equal(
    token.uses,
    "actions/create-github-app-token@64ab729a6294000f5aa4a4bc0b73d1f32bc45510"
  );
  assert.deepEqual(token.with, {
    "client-id": "${{ secrets.RADIUS_TERRAFORM_PRIVATE_MODULES_CLIENT_ID }}",
    "private-key":
      "${{ secrets.RADIUS_TERRAFORM_PRIVATE_MODULES_PRIVATE_KEY }}",
    owner: "radius-project",
    repositories: "terraform-private-modules",
    "permission-contents": "read"
  });
  assert.equal(token.id, "private-module-token");
  assert.equal(token.if, undefined);
  assert.equal(token["continue-on-error"], undefined);
  assert.equal(token.with["skip-token-revoke"], undefined);
  assert.equal(steps.indexOf(check) + 1, steps.indexOf(token));
  assert.equal(steps.indexOf(token) + 1, steps.indexOf(run));
  assert.ok(
    steps.indexOf(check) >
      steps.indexOf(
        step(
          cloud.tests,
          "Restore Bicep artifacts before running functional tests"
        )
      )
  );
  assert.equal(cloud.tests["timeout-minutes"], 60);
  assert.equal(
    run.env.GH_TOKEN,
    "${{ steps.private-module-token.outputs.token }}"
  );
  assert.equal(cloud.tests.outputs, undefined);
  assert.equal(cloud.tests.env.GH_TOKEN, undefined);
  assert.doesNotMatch(
    JSON.stringify(cloud),
    /FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN/
  );
  for (const [name, job] of Object.entries(cloud)) {
    if (name !== "tests")
      assert.doesNotMatch(
        JSON.stringify(job),
        /RADIUS_TERRAFORM_PRIVATE_MODULES|private-module-token/
      );
  }
  for (const item of steps.filter(
    (item) => ![check, token, run].includes(item)
  )) {
    assert.doesNotMatch(
      JSON.stringify(item),
      /RADIUS_TERRAFORM_PRIVATE_MODULES|private-module-token/
    );
  }
  for (const settings of [
    { MODULE_CLIENT_ID: "", MODULE_PRIVATE_KEY: "" },
    { MODULE_CLIENT_ID: "test-client", MODULE_PRIVATE_KEY: "" },
    { MODULE_CLIENT_ID: "", MODULE_PRIVATE_KEY: "test-key" },
    { MODULE_CLIENT_ID: "test-client", MODULE_PRIVATE_KEY: "test-key" }
  ]) {
    const result = spawnSync("bash", ["-e", "-c", check.run], {
      env: { ...process.env, ...settings },
      encoding: "utf8"
    });
    const complete = settings.MODULE_CLIENT_ID && settings.MODULE_PRIVATE_KEY;
    assert.equal(result.status, complete ? 0 : 1);
    if (!complete)
      assert.match(
        result.stdout,
        /::error::Configure repository Actions secrets/
      );
    assert.doesNotMatch(result.stdout + result.stderr, /test-client|test-key/);
  }
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
        REL_VERSION: tag,
        BICEP_TYPES_REGISTRY: "biceptypes.azurecr.io"
      }
    }
  );
  const config = JSON.parse(
    await readFile(join(directory, "test/bicepconfig.json"))
  );
  assert.equal(
    config.extensions.radius,
    `br:localhost:5000/test/radius:${tag}`
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
