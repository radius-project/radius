// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
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
import { fileURLToPath, pathToFileURL } from "node:url";
import test from "node:test";

import changes from "./changes.mjs";

const workflow = (
  await readFile(new URL("../workflows/__changes.yml", import.meta.url), "utf8")
).replace(/\r\n/g, "\n");

function createCore(
  baseSha = "",
  onlyChanged = "true",
  releaseMetadata = "false"
) {
  const outputs = [];
  const failures = [];
  return {
    outputs,
    failures,
    getInput: (name) =>
      ({
        BASE_SHA: baseSha,
        ALL_FILES_COUNT: "2",
        IGNORED_FILES_COUNT: onlyChanged === "true" ? "2" : "1",
        RELEASE_FILES_COUNT: releaseMetadata === "true" ? "1" : "0",
        WORKFLOW_FILES_COUNT: "0"
      })[name],
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
      for (const releaseMetadata of ["true", "false"]) {
        test(`${eventName}: base=${baseSha || "absent"}, filter=${onlyChanged}, release=${releaseMetadata}`, async () => {
          const core = createCore(baseSha, onlyChanged, releaseMetadata);
          await changes({ context: { eventName }, core });
          const filtered =
            filteringEvents.includes(eventName) || Boolean(baseSha);
          assert.deepEqual(core.outputs, [
            ["only_changed", filtered ? onlyChanged : "false"],
            ["release_metadata", filtered ? releaseMetadata : "false"]
          ]);
          assert.deepEqual(core.failures, []);
        });
      }
    }
  }
}

const patterns = workflow
  .match(/^        default: \|\n((?:          .+\n)+)/m)[1]
  .trim()
  .split("\n")
  .map((pattern) => pattern.trim());

function filterGroup(name) {
  const match = workflow.match(
    new RegExp(`^            ${name}:\\n((?:              - .+\\n)+)`, "m")
  );
  assert.ok(match, `missing filter group ${name}`);
  return match[1]
    .trim()
    .split("\n")
    .map((line) => line.trim().slice(2));
}

const releasePatterns = filterGroup("release_metadata");
const workflowPatterns = filterGroup("workflows");

function countMatchingFiles(files, filePatterns = patterns) {
  const included = filePatterns.filter((pattern) => !pattern.startsWith("!"));
  const excluded = filePatterns
    .filter((pattern) => pattern.startsWith("!"))
    .map((pattern) => pattern.slice(1));
  return String(
    files.filter(
      (file) =>
        included.some((pattern) => path.posix.matchesGlob(file, pattern)) &&
        !excluded.some((pattern) => path.posix.matchesGlob(file, pattern))
    ).length
  );
}

// The complete changed-file set from the first Prepare Release PR, #13391.
const releaseMetadata = [
  ".github/release-plans/v0.62.0-rc.1.yaml",
  "CHANGELOG.md",
  "docs/release-notes/v0.62.0-rc.1.md",
  "versions.yaml"
];
const consumerWorkflows = [
  "build-main.yaml",
  "build-validation.yaml",
  "functional-test-cloud.yaml",
  "functional-test-noncloud.yaml",
  "lint.yaml",
  "publish-docs.yaml",
  "unit-tests.yaml",
  "validate-bicep.yaml"
];
const relevantInputs = [
  "cmd/rad/main.go",
  "pkg/version/version.go",
  "go.mod",
  "go.sum",
  "deploy/Chart/values.yaml",
  "test/functional-portable/ucp/noncloud/ucp_test.go",
  "build/test.mk",
  "cliff.toml",
  ".github/scripts/prepare-release.sh",
  ".github/scripts/changes.mjs",
  ".github/workflows/__changes.yml",
  ".github/workflows/release-plan.yaml",
  ".github/workflows/build-release.yaml",
  ...consumerWorkflows.map((file) => `.github/workflows/${file}`)
];

for (const eventName of filteringEvents) {
  for (const [files, expected] of [
    [releaseMetadata, true],
    ...releaseMetadata.map((file) => [[file], true]),
    ...relevantInputs.map((file) => [[...releaseMetadata, file], false]),
    [[...releaseMetadata, "cmd/rad/main.go", "docs/renamed-source.md"], false],
    [[...releaseMetadata, ".github/workflows/purge-old-images.yaml"], false],
    [[...releaseMetadata, "docs/contributing/README.md"], true],
    [["README.md"], true],
    [["docs/contributing/README.md"], true],
    [[".github/skills/radius-code-review/SKILL.md"], true],
    [["docs/diagram.svg"], true],
    [["LICENSE"], true],
    [[".github/linters/config.yaml"], true],
    [[".github/configs/label-actions.yaml"], true],
    [[".github/workflows/publish-docs.yaml"], true],
    [["README.md", ".github/workflows/publish-docs.yaml"], true],
    [[".github/workflows/purge-old-images.yaml"], true],
    [["cmd/rad/main.go"], false],
    [["README.md", "cmd/rad/main.go"], false],
    [["go.mod"], false],
    [["deploy/Chart/values.yaml"], false],
    [[".github/workflows/unit-tests.yaml"], false],
    [[".github/workflows/build-release.yaml"], false],
    [[], false]
  ]) {
    test(`${eventName}: classify ${files.join(", ") || "empty changes"}`, async () => {
      const core = createCore();
      const releaseFiles = countMatchingFiles(files, releasePatterns);
      core.getInput = (name) =>
        ({
          BASE_SHA: "",
          ALL_FILES_COUNT: String(files.length),
          IGNORED_FILES_COUNT: countMatchingFiles(files),
          RELEASE_FILES_COUNT: releaseFiles,
          WORKFLOW_FILES_COUNT: countMatchingFiles(files, workflowPatterns)
        })[name];
      await changes({ context: { eventName }, core });
      assert.deepEqual(core.outputs, [
        ["only_changed", String(expected)],
        ["release_metadata", String(Number(releaseFiles) > 0)]
      ]);
      assert.deepEqual(core.failures, []);
    });
  }
}

for (const eventName of [...filteringEvents, "workflow_run"]) {
  for (const [allFiles, ignoredFiles, releaseFiles, workflowFiles] of [
    ["", "2", "0", "0"],
    ["2", undefined, "0", "0"],
    ["2", "unknown", "0", "0"],
    ["2", "-1", "0", "0"],
    ["1", "2", "0", "0"],
    ["2.5", "2", "0", "0"],
    ["9007199254740992", "9007199254740992", "0", "0"],
    ["2", "2", "", "0"],
    ["2", "2", "0", undefined],
    ["2", "2", "3", "0"],
    ["2", "2", "0", "3"]
  ]) {
    test(`${eventName}: rejects invalid file counts ${allFiles}/${ignoredFiles}/${releaseFiles}/${workflowFiles}`, async () => {
      const core = createCore();
      core.getInput = (name) =>
        ({
          BASE_SHA: "base-sha",
          ALL_FILES_COUNT: allFiles,
          IGNORED_FILES_COUNT: ignoredFiles,
          RELEASE_FILES_COUNT: releaseFiles,
          WORKFLOW_FILES_COUNT: workflowFiles
        })[name];
      await changes({ context: { eventName }, core });
      assert.deepEqual(core.outputs, []);
      assert.deepEqual(core.failures, [
        "Change filter returned invalid file counts"
      ]);
    });
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
const [comparison, fetchBase, filter, trusted, result] = steps;

test("comparison precedes trusted restoration and preserves the caller inputs", () => {
  assert.deepEqual(
    steps.map((step) => step.split("\n")[0].trim()),
    [
      "Checkout",
      "Fetch comparison base",
      "Filter",
      "Checkout trusted changes helper",
      "Set result"
    ]
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
    'all: "**"',
    "ignored: ${{ toJSON(inputs.files) }}",
    "base_sha: ${{ inputs.base_sha }}",
    "output_renamed_files_as_deleted_and_added: true",
    "fail_on_initial_diff_error: true",
    "fail_on_submodule_diff_error: true"
  ]) {
    assert.ok(filter.includes(input), input);
  }
  assert.doesNotMatch(comparison + filter, /^\s+(?:path|working-directory):/m);
  assert.doesNotMatch(comparison + filter, /^\s+(?:run|script):/m);
});

test("base fetching uses upstream identity and does not execute checkout content", () => {
  const baseExpression =
    "inputs.base_sha || github.event.pull_request.base.sha || github.event.merge_group.base_sha";
  assert.ok(fetchBase.includes(`if: ${baseExpression}`));
  assert.ok(fetchBase.includes(`BASE_SHA: \${{ ${baseExpression} }}`));
  for (const input of [
    "shell: bash",
    "BASE_REPOSITORY: ${{ github.repository }}",
    "BASE_SERVER_URL: ${{ github.server_url }}",
    "GH_TOKEN: ${{ github.token }}"
  ]) {
    assert.ok(fetchBase.includes(input), input);
  }
  assert.doesNotMatch(
    fetchBase,
    /inputs\.repository|head\.repo|^\s+(?:uses|working-directory):/m
  );
});

const fetchMatch = fetchBase.match(/        run: \|\n([\s\S]*)$/);
assert.ok(fetchMatch, "Fetch comparison base must have an inline run command");
const fetchBody = fetchMatch[1].replace(/^          /gm, "");
assert.doesNotMatch(fetchBody, /\$\{\{|\.github\/|git config/);

function runGit(cwd, args) {
  const result = spawnSync("git", args, {
    cwd,
    encoding: "utf8",
    env: {
      ...process.env,
      GIT_CONFIG_NOSYSTEM: "1",
      GIT_CONFIG_GLOBAL: process.platform === "win32" ? "NUL" : "/dev/null"
    }
  });
  assert.equal(result.status, 0, result.error?.message || result.stderr);
  return result.stdout.trim();
}

async function createForkCheckout(t) {
  const { workspace } = await createWorkspace(t);
  const upstream = path.join(workspace, "upstream.git");
  const fork = path.join(workspace, "fork");
  const checkout = path.join(workspace, "checkout");
  runGit(workspace, ["init", "--quiet", upstream]);
  runGit(upstream, ["config", "user.name", "Workflow test"]);
  runGit(upstream, ["config", "user.email", "workflow-test@example.invalid"]);
  await writeFile(path.join(upstream, "shared.txt"), "shared\n");
  runGit(upstream, ["add", "."]);
  runGit(upstream, ["commit", "--quiet", "-s", "-m", "shared base"]);
  const sharedBase = runGit(upstream, ["rev-parse", "HEAD"]);
  runGit(workspace, ["clone", "--quiet", "--no-local", upstream, fork]);
  await writeFile(path.join(upstream, "upstream.txt"), "upstream only\n");
  runGit(upstream, ["add", "."]);
  runGit(upstream, ["commit", "--quiet", "-s", "-m", "upstream advance"]);
  const baseSha = runGit(upstream, ["rev-parse", "HEAD"]);
  runGit(fork, ["config", "user.name", "Workflow test"]);
  runGit(fork, ["config", "user.email", "workflow-test@example.invalid"]);
  await writeFile(path.join(fork, "pr.txt"), "PR only\n");
  runGit(fork, ["add", "."]);
  runGit(fork, ["commit", "--quiet", "-s", "-m", "PR change"]);
  runGit(workspace, ["clone", "--quiet", "--no-local", fork, checkout]);
  const headSha = runGit(checkout, ["rev-parse", "HEAD"]);
  runGit(checkout, ["checkout", "--quiet", "--detach", headSha]);
  return { workspace, checkout, sharedBase, baseSha, headSha };
}

function runFetchBase({ workspace, checkout, baseSha }) {
  return spawnSync(
    "bash",
    ["--noprofile", "--norc", "-eo", "pipefail", "-c", fetchBody],
    {
      cwd: checkout,
      encoding: "utf8",
      env: {
        ...process.env,
        BASE_SHA: baseSha,
        BASE_REPOSITORY: "upstream",
        BASE_SERVER_URL: pathToFileURL(workspace).href,
        GH_TOKEN: "test-token",
        GIT_CONFIG_NOSYSTEM: "1",
        GIT_CONFIG_GLOBAL: process.platform === "win32" ? "NUL" : "/dev/null"
      }
    }
  );
}

test("fetches a missing upstream base into a full fork checkout without changing HEAD", async (t) => {
  const fixture = await createForkCheckout(t);
  const { checkout, baseSha, headSha, sharedBase } = fixture;
  assert.equal(
    runGit(checkout, ["rev-parse", "--is-shallow-repository"]),
    "false"
  );
  const before = spawnSync("git", ["merge-base", baseSha, headSha], {
    cwd: checkout,
    encoding: "utf8"
  });
  assert.notEqual(before.status, 0);
  assert.match(before.stderr, /Not a valid commit name/);
  const origin = runGit(checkout, ["remote", "get-url", "origin"]);
  const fetched = runFetchBase(fixture);
  assert.equal(fetched.status, 0, fetched.error?.message || fetched.stderr);
  assert.equal(runGit(checkout, ["rev-parse", "HEAD"]), headSha);
  assert.equal(runGit(checkout, ["remote", "get-url", "origin"]), origin);
  assert.equal(runGit(checkout, ["merge-base", baseSha, headSha]), sharedBase);
  assert.equal(
    runGit(checkout, ["diff", "--name-only", `${baseSha}...${headSha}`]),
    "pr.txt"
  );
  assert.equal(runGit(checkout, ["status", "--porcelain"]), "");
  assert.doesNotMatch(
    await readFile(path.join(checkout, ".git", "config"), "utf8"),
    /test-token|extraheader/
  );
});

test("an existing base skips fetching even when the upstream URL is unavailable", async (t) => {
  const fixture = await createForkCheckout(t);
  const fetched = runFetchBase({
    ...fixture,
    workspace: path.join(fixture.workspace, "missing"),
    baseSha: fixture.sharedBase
  });
  assert.equal(fetched.status, 0, fetched.error?.message || fetched.stderr);
  assert.equal(
    runGit(fixture.checkout, ["rev-parse", "HEAD"]),
    fixture.headSha
  );
});

test("an unavailable comparison commit fails instead of allowing filtering", async (t) => {
  const fixture = await createForkCheckout(t);
  const fetched = runFetchBase({ ...fixture, baseSha: "f".repeat(40) });
  assert.notEqual(fetched.status, 0);
  assert.match(
    fetched.stderr,
    /not our ref|unadvertised object|couldn't find remote ref/
  );
  assert.equal(
    runGit(fixture.checkout, ["rev-parse", "HEAD"]),
    fixture.headSha
  );
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
      "INPUT_ALL_FILES_COUNT: ${{ steps.filter.outputs.all_all_changed_and_modified_files_count }}"
    )
  );
  assert.ok(
    result.includes(
      "INPUT_IGNORED_FILES_COUNT: ${{ steps.filter.outputs.ignored_all_changed_and_modified_files_count }}"
    )
  );
  assert.ok(
    result.includes(
      "INPUT_RELEASE_FILES_COUNT: ${{ steps.filter.outputs.release_metadata_all_changed_and_modified_files_count }}"
    )
  );
  assert.ok(
    result.includes(
      "INPUT_WORKFLOW_FILES_COUNT: ${{ steps.filter.outputs.workflows_all_changed_and_modified_files_count }}"
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
  assert.ok(
    workflow.includes(
      "release_metadata: ${{ steps.set-result.outputs.release_metadata }}"
    )
  );
  assert.ok(
    workflow.includes("value: ${{ jobs.changes.outputs.release_metadata }}")
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
  assert.deepEqual(core.outputs, [
    ["only_changed", "false"],
    ["release_metadata", "false"]
  ]);
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

const consumers = Object.fromEntries(
  await Promise.all(
    consumerWorkflows.map(async (file) => [
      file,
      (
        await readFile(new URL(`../workflows/${file}`, import.meta.url), "utf8")
      ).replace(/\r\n/g, "\n")
    ])
  )
);

function job(source, name) {
  const block = source
    .split(/^  (?=[a-z][\w-]*:\n)/m)
    .find((part) => part.startsWith(`${name}:\n`));
  assert.ok(block, `missing job ${name}`);
  return block;
}

function step(source, name) {
  const block = source
    .split(/^ {6}- name: |^ {10}- name: /m)
    .find((part) => part.startsWith(`${name}\n`));
  assert.ok(block, `missing step ${name}`);
  return block;
}

test("every heavy workflow consumer uses the shared filter without overriding its paths", () => {
  for (const [file, source] of Object.entries(consumers)) {
    const selection = job(source, "changes");
    assert.match(
      selection,
      /uses: \.\/\.github\/workflows\/__changes\.yml/,
      file
    );
    assert.doesNotMatch(selection, /^\s+files:/m, file);
    assert.match(
      source,
      file === "functional-test-cloud.yaml" ?
        /needs\.changes\.outputs\.only_changed == 'false'/
      : /needs\.changes\.outputs\.only_changed != 'true'/,
      file
    );
  }
});

test("Build Check includes change detection failures and cancellations", () => {
  for (const file of ["build-main.yaml", "build-validation.yaml"]) {
    const buildCheck = job(consumers[file], "build-check");
    assert.match(buildCheck, /^    if: always\(\)$/m);
    assert.match(buildCheck, /^    name: Build Check$/m);
    assert.match(
      buildCheck,
      /^    needs: \[changes, build-snapshot, build-and-push-cli, build-and-push-images\]$/m
    );
    assert.match(
      step(buildCheck, "Check build matrix results"),
      /if: needs\.changes\.result != 'success' \|\| contains\(needs\.\*\.result, 'failure'\) \|\| contains\(needs\.\*\.result, 'cancelled'\)/
    );
    assert.match(
      step(buildCheck, "Build matrix succeeded"),
      /needs\.changes\.result == 'success' && !contains\(needs\.\*\.result, 'failure'\) && !contains\(needs\.\*\.result, 'cancelled'\)/
    );
  }
});

test("Lint keeps its required status and release tests without running unrelated tools", () => {
  const lint = job(consumers["lint.yaml"], "lint");
  assert.match(lint, /^    name: Lint$/m);
  assert.match(lint, /^    if: always\(\) && /m);
  const guard = step(lint, "Check change detection");
  assert.match(guard, /if: needs\.changes\.result != 'success'/);
  assert.match(guard, /exit 1/);
  const metadata = step(lint, "Test release metadata");
  assert.match(metadata, /if: needs\.changes\.outputs\.only_changed == 'true'/);
  for (const target of [
    "test-workflow-changes",
    "test-release-version-format",
    "test-changelog-range",
    "test-changelog-config",
    "test-prepare-release",
    "test-release-plan"
  ]) {
    assert.ok(metadata.includes(target), target);
  }
  for (const name of [
    "Setup Go",
    "Download Go modules",
    "Install helm",
    "Install golangci-lint",
    "Install shellcheck",
    "Go lint",
    "Helm lint and unit tests",
    "pnpm-based checks",
    "Shell lint",
    "Run make generate",
    "Check if generated code is up to date",
    "Check failures"
  ]) {
    assert.match(
      step(lint, name),
      /if: needs\.changes\.outputs\.only_changed != 'true'/,
      name
    );
  }
});

const lintCondition = job(consumers["lint.yaml"], "lint").match(
  /^    if: (.+)$/m
)[1];
const runLintCondition = new Function(
  "needs",
  "always",
  `return ${lintCondition}`
);

for (const [name, result, onlyChanged, releaseMetadata, expected] of [
  ["source PR", "success", "false", "false", true],
  ["mixed source and metadata", "success", "false", "true", true],
  ["release metadata only", "success", "true", "true", true],
  ["ordinary docs/config/publisher only", "success", "true", "false", false],
  ["failed classification", "failure", "", "", true],
  ["cancelled classification", "cancelled", "", "", true],
  ["skipped classification", "skipped", "", "", true]
]) {
  test(`Lint selection: ${name}`, () => {
    assert.equal(
      runLintCondition(
        {
          changes: {
            result,
            outputs: {
              only_changed: onlyChanged,
              release_metadata: releaseMetadata
            }
          }
        },
        () => true
      ),
      expected
    );
  });
}

const cloud = consumers["functional-test-cloud.yaml"];
const report = job(cloud, "report-test-results");

test("cloud skips and final status require successful change detection", () => {
  for (const name of ["build", "skip-tests"]) {
    assert.match(
      job(cloud, name),
      /if: always\(\) && needs\.setup\.result == 'success' && needs\.changes\.result == 'success'/
    );
  }
  const dependencies = report
    .match(/    needs:\s*\[([\s\S]*?)\]/)[1]
    .split(",")
    .map((name) => name.trim());
  for (const name of [
    "authorize",
    "setup",
    "changes",
    "build",
    "skip-tests",
    "tests",
    "announce",
    "upload-ghcr",
    "upload-test-types"
  ]) {
    assert.ok(
      dependencies.includes(name),
      `Missing status dependency: ${name}`
    );
  }
  assert.match(
    report,
    /CHECKOUT_REF: \$\{\{ needs\.setup\.outputs\.CHECKOUT_REF \|\| github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/
  );
  assert.match(report, /name: Functional Test Run/);
  assert.match(
    report,
    /conclusion: \$\{\{ steps\.get_test_status\.outputs\.test_status \|\| 'failure' \}\}/
  );
});

const aggregate = step(report, "Aggregate functional test status")
  .match(/          script: \|\n([\s\S]*?)        env:/)[1]
  .replace(/^            /gm, "");

for (const [name, overrides, expected] of [
  ["source checks pass", {}, "success"],
  [
    "metadata skips",
    {
      ONLY_CHANGED: "true",
      BUILD_RESULT: "skipped",
      SKIP_RESULT: "success",
      TESTS_RESULT: "skipped"
    },
    "success"
  ],
  ["classification fails", { CHANGES_RESULT: "failure" }, "failure"],
  ["classification is cancelled", { CHANGES_RESULT: "cancelled" }, "cancelled"],
  ["classification is skipped", { CHANGES_RESULT: "skipped" }, "failure"],
  ["filter output is missing", { ONLY_CHANGED: "" }, "failure"],
  ["authorization fails", { AUTHORIZE_RESULT: "failure" }, "failure"],
  ["authorization is skipped", { AUTHORIZE_RESULT: "skipped" }, "failure"],
  ["setup fails", { SETUP_RESULT: "failure" }, "failure"],
  ["setup is skipped", { SETUP_RESULT: "skipped" }, "failure"],
  ["build fails", { BUILD_RESULT: "failure" }, "failure"],
  ["source build is skipped", { BUILD_RESULT: "skipped" }, "failure"],
  ["matrix fails", { TESTS_RESULT: "failure" }, "failure"],
  ["source matrix is skipped", { TESTS_RESULT: "skipped" }, "failure"],
  ["matrix is cancelled", { TESTS_RESULT: "cancelled" }, "cancelled"],
  ["skip reporting fails", { SKIP_RESULT: "failure" }, "failure"],
  [
    "metadata skip reporting is absent",
    { ONLY_CHANGED: "true", BUILD_RESULT: "skipped", TESTS_RESULT: "skipped" },
    "failure"
  ],
  [
    "failure takes precedence over cancellation",
    { CHANGES_RESULT: "failure", TESTS_RESULT: "cancelled" },
    "failure"
  ]
]) {
  test(`Functional Test Run: ${name}`, async () => {
    const values = {
      AUTHORIZE_RESULT: "success",
      SETUP_RESULT: "success",
      CHANGES_RESULT: "success",
      ONLY_CHANGED: "false",
      BUILD_RESULT: "success",
      SKIP_RESULT: "skipped",
      TESTS_RESULT: "success",
      ...overrides
    };
    const needs = {
      authorize: { result: values.AUTHORIZE_RESULT },
      setup: { result: values.SETUP_RESULT },
      changes: {
        result: values.CHANGES_RESULT,
        outputs: { only_changed: values.ONLY_CHANGED }
      },
      build: { result: values.BUILD_RESULT },
      "skip-tests": { result: values.SKIP_RESULT },
      tests: { result: values.TESTS_RESULT }
    };
    for (const phase of ["announce", "upload-ghcr", "upload-test-types"]) {
      needs[phase] = {
        result: values.ONLY_CHANGED === "true" ? "skipped" : "success"
      };
    }
    const outputs = [];
    const summary = {
      addHeading() {
        return this;
      },
      addRaw() {
        return this;
      },
      addTable() {
        return this;
      },
      async write() {}
    };
    const run = new (Object.getPrototypeOf(async () => {}).constructor)(
      "core",
      "process",
      aggregate
    );
    await run(
      { setOutput: (...args) => outputs.push(args), summary },
      {
        env: {
          GITHUB_WORKSPACE: fileURLToPath(new URL("../../", import.meta.url)),
          NEEDS_JSON: JSON.stringify(needs)
        }
      }
    );
    assert.deepEqual(outputs, [["test_status", expected]]);
  });
}
