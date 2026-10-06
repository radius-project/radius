// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import {
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";

import changes from "./changes.mjs";

const workflow = (
  await readFile(new URL("../workflows/__changes.yml", import.meta.url), "utf8")
).replace(/\r\n/g, "\n");

function createCore(baseSha = "", onlyChanged = "true") {
  const outputs = [];
  const failures = [];
  return {
    outputs,
    failures,
    getInput: (name) =>
      ({ BASE_SHA: baseSha, ONLY_CHANGED: onlyChanged })[name],
    info() {},
    setOutput: (name, value) => outputs.push([name, value]),
    setFailed: (message) => failures.push(message)
  };
}

const filteringEvents = ["pull_request", "pull_request_target", "merge_group"];
const otherEvents = [
  "workflow_run",
  "push",
  "schedule",
  "repository_dispatch",
  "workflow_dispatch",
  "other"
];

for (const eventName of [...filteringEvents, ...otherEvents]) {
  for (const baseSha of ["", "base-sha"]) {
    for (const onlyChanged of ["true", "false"]) {
      test(`${eventName}: base=${baseSha || "absent"}, filter=${onlyChanged}`, async () => {
        const core = createCore(baseSha, onlyChanged);
        await changes({ context: { eventName }, core });
        const expected =
          filteringEvents.includes(eventName) || baseSha ?
            onlyChanged
          : "false";
        assert.deepEqual(core.outputs, [["only_changed", expected]]);
        assert.deepEqual(core.failures, []);
      });
    }
  }
}

for (const context of [undefined, null, {}, { eventName: "" }]) {
  test(`invalid context: ${JSON.stringify(context)}`, async () => {
    const core = createCore();
    await changes({ context, core });
    assert.deepEqual(core.outputs, []);
    assert.deepEqual(core.failures, ["GitHub context is missing or invalid"]);
  });
}

for (const error of [new Error("input unavailable"), "input unavailable"]) {
  test(`reports ${error instanceof Error ? "Error" : "non-Error"} failures`, async () => {
    const core = createCore();
    core.getInput = () => {
      throw error;
    };
    await changes({ context: { eventName: "pull_request_target" }, core });
    assert.deepEqual(core.outputs, []);
    assert.deepEqual(core.failures, [
      error instanceof Error ? error.message : `Unexpected error: ${error}`
    ]);
  });
}

// These assertions deliberately follow this workflow's small, fixed step list.
const steps = workflow.split(/^      - name: /m).slice(1);
const [comparison, filter, trusted, result] = steps;

test("comparison precedes trusted restoration and preserves the caller inputs", () => {
  assert.deepEqual(
    steps.map((step) => step.split("\n")[0].trim()),
    ["Checkout", "Filter", "Checkout trusted changes helper", "Set result"]
  );
  for (const input of [
    "fetch-depth: 0",
    "persist-credentials: false",
    "ref: ${{ inputs.ref }}",
    "repository: ${{ inputs.repository }}",
    "allow-unsafe-pr-checkout: true"
  ]) {
    assert.ok(comparison.includes(input), input);
  }
  for (const input of [
    "id: filter",
    "json: true",
    "files: ${{ inputs.files }}",
    "base_sha: ${{ inputs.base_sha }}"
  ]) {
    assert.ok(filter.includes(input), input);
  }
  assert.doesNotMatch(comparison + filter, /^\s+(?:path|working-directory):/m);
  assert.doesNotMatch(comparison + filter, /^\s+(?:run|script):/m);
});

test("only privileged PR-derived events restore the trusted triggering revision", () => {
  assert.match(
    trusted,
    /^\s+if: github\.event_name == 'pull_request_target' \|\| github\.event_name == 'workflow_run'$/m
  );
  assert.match(
    trusted,
    /uses: actions\/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1/
  );
  for (const input of [
    "repository: ${{ github.repository }}",
    "ref: ${{ github.sha }}",
    "sparse-checkout: |\n            .github/scripts/changes.mjs",
    "sparse-checkout-cone-mode: false",
    "persist-credentials: false"
  ]) {
    assert.ok(trusted.includes(input), input);
  }
  assert.doesNotMatch(
    trusted,
    /inputs\.|head\.sha|head_sha|allow-unsafe-pr-checkout|^\s+(?:path|clean):/m
  );
});

test("restoration failures cannot bypass the normal success gate or change outputs", () => {
  assert.doesNotMatch(workflow, /continue-on-error:|always\(\)|failure\(\)/);
  assert.doesNotMatch(result, /^\s+if:/m);
  assert.ok(
    result.includes(
      "INPUT_ONLY_CHANGED: ${{ steps.filter.outputs.only_changed }}"
    )
  );
  assert.ok(result.includes("INPUT_BASE_SHA: ${{ inputs.base_sha }}"));
  assert.ok(
    workflow.includes(
      "only_changed: ${{ steps.set-result.outputs.only_changed }}"
    )
  );
  assert.ok(
    workflow.includes("value: ${{ jobs.changes.outputs.only_changed }}")
  );
});

const scriptMatch = result.match(
  /          script: \|\r?\n([\s\S]*?)        env:/
);
assert.ok(scriptMatch, "Set result must have a script and input environment");
const scriptBody = scriptMatch[1].replace(/^            /gm, "");
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const runResult = new AsyncFunction("context", "core", "process", scriptBody);

async function createWorkspace(t) {
  const workspace = await mkdtemp(path.join(tmpdir(), "radius-changes-"));
  t.after(() => rm(workspace, { recursive: true, force: true }));
  const helper = path.join(workspace, ".github", "scripts", "changes.mjs");
  await mkdir(path.dirname(helper), { recursive: true });
  return { workspace, helper };
}

test("without restoration the result script would execute the PR helper", async (t) => {
  const { workspace, helper } = await createWorkspace(t);
  await writeFile(helper, 'throw new Error("PR helper executed");\n');
  const core = createCore("base-sha");
  await assert.rejects(
    runResult({ eventName: "pull_request_target" }, core, {
      env: { GITHUB_WORKSPACE: pathToFileURL(workspace).href }
    }),
    { message: "PR helper executed" }
  );
  assert.deepEqual(core.outputs, []);
});

test("the actual result script imports the restored helper, not PR content", async (t) => {
  const { workspace, helper } = await createWorkspace(t);
  await writeFile(helper, 'throw new Error("PR helper must not execute");\n');
  // Simulate restoration only; GitHub's checkout action is not run by this fixture.
  await copyFile(new URL("./changes.mjs", import.meta.url), helper);
  const core = createCore("base-sha", "false");
  await runResult({ eventName: "pull_request_target" }, core, {
    env: { GITHUB_WORKSPACE: pathToFileURL(workspace).href }
  });
  assert.deepEqual(core.outputs, [["only_changed", "false"]]);
  assert.deepEqual(core.failures, []);
});

test("a missing restored helper fails import without a fallback or output", async (t) => {
  const { workspace } = await createWorkspace(t);
  const core = createCore("base-sha");
  await assert.rejects(
    runResult({ eventName: "workflow_run" }, core, {
      env: { GITHUB_WORKSPACE: pathToFileURL(workspace).href }
    }),
    { code: "ERR_MODULE_NOT_FOUND" }
  );
  assert.deepEqual(core.outputs, []);
});
