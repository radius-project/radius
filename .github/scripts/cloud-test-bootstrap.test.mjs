// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";
import { pathToFileURL } from "node:url";

// Keep this base dependency inventory current when helper imports or tools
// change. Workflow checkout selection and isolation belong in activation tests.
const modules = [
  [".github/scripts/cloud-test-context.mjs", ["setup", "testStatus"]],
  [
    ".github/scripts/cloud-test-artifacts.mjs",
    ["capture", "select", "prepare", "publish", "testTypesRegistry"]
  ],
  [".github/scripts/azure-oidc-login.mjs", ["login"]]
];
const registryAction = ".github/actions/create-local-registry";
const root = resolve(import.meta.dirname, "../..");

async function preparedBase(t) {
  const base = await mkdtemp(join(tmpdir(), "cloud-bootstrap-"));
  t.after(() => rm(base, { recursive: true, force: true }));
  for (const file of [
    ...modules.map(([file]) => file),
    registryAction,
    "Makefile",
    "build"
  ]) {
    await mkdir(dirname(join(base, file)), { recursive: true });
    await cp(join(root, file), join(base, file), { recursive: true });
  }
  return base;
}

async function checkDependencies(base) {
  for (const [file, exports] of modules) {
    execFileSync(
      process.execPath,
      [
        "--input-type=module",
        "-e",
        `const helper = await import(${JSON.stringify(pathToFileURL(join(base, file)).href)});
         for (const name of ${JSON.stringify(exports)}) {
           if (typeof helper[name] !== "function") throw new Error("Missing export: " + name);
         }`
      ],
      { stdio: "pipe" }
    );
  }
  const action = JSON.parse(
    execFileSync("yq", [
      "-o=json",
      ".",
      join(base, registryAction, "action.yaml")
    ])
  );
  assert.equal(action.runs.using, "composite");
  for (const step of action.runs.steps) {
    if (step.run)
      execFileSync("bash", ["-n", "-c", step.run], { stdio: "pipe" });
  }
  const pull = action.runs.steps.find((step) => step.env?.PULL_SCRIPT);
  assert.equal(pull.env.PULL_SCRIPT, "${{ github.action_path }}/pull-image.sh");
  for (const file of [
    `${registryAction}/pull-image.sh`,
    "build/scripts/install-oras.sh",
    "build/scripts/retry-curl.sh"
  ]) {
    await readFile(join(base, file));
    execFileSync("bash", ["-n", join(base, file)], { stdio: "pipe" });
  }
  assert.match(
    await readFile(join(base, "build/scripts/install-oras.sh"), "utf8"),
    /source .*retry-curl\.sh/
  );
  const command = execFileSync("make", ["-n", "install-oras"], {
    cwd: base,
    encoding: "utf8",
    stdio: "pipe",
    env: {
      ...process.env,
      GIT_COMMIT: "a".repeat(40),
      GIT_VERSION: "bootstrap-fixture"
    }
  });
  assert.match(command, /ORAS_VERSION="v[0-9.]+"/);
  assert.match(command, /\.\/build\/scripts\/install-oras\.sh/);
}

test("prepared base loads helper exports and contains action and installer dependencies", async (t) => {
  await checkDependencies(await preparedBase(t));
});

for (const missing of [
  ...modules.map(([file]) => file),
  `${registryAction}/pull-image.sh`,
  "build/tools.generated.mk",
  "build/scripts/install-oras.sh",
  "build/scripts/retry-curl.sh"
]) {
  test(`prepared-base dependency check fails when ${missing} is missing`, async (t) => {
    const base = await preparedBase(t);
    await rm(join(base, missing));
    await assert.rejects(checkDependencies(base), (error) => {
      const output = `${error.message}\n${error.stderr ?? ""}`;
      assert.match(output, /Cannot find module|ENOENT|No such file/);
      assert.ok(output.includes(missing), output);
      if (missing.endsWith(".mjs"))
        assert.match(output, /ERR_MODULE_NOT_FOUND/);
      return true;
    });
  });
}
