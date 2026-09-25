// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { runInNewContext } from "node:vm";

const main = readFileSync(".github/workflows/build-main.yaml", "utf8");
const direct = readFileSync(
  ".github/workflows/__publish-bicep-types.yaml",
  "utf8"
);

function job(workflow, name) {
  const lines = workflow.split("\n");
  const start = lines.indexOf(`  ${name}:`);
  assert.ok(start >= 0);
  const end = lines.findIndex(
    (line, i) => i > start && /^  [a-z0-9-]+:$/.test(line)
  );
  return lines.slice(start, end < 0 ? lines.length : end).join("\n");
}

function block(text, marker, indent) {
  const lines = text.split("\n");
  const start = lines.findIndex((line) => line.trim() === marker);
  assert.ok(start >= 0);
  const result = [];
  for (const line of lines.slice(start + 1)) {
    if (line.trim() && !line.startsWith(" ".repeat(indent))) break;
    result.push(line.slice(indent));
  }
  return result.join("\n").trim();
}

const routeScript = block(job(main, "bicep-publish-mode"), "script: |", 12);
const legacyIf = job(main, "build-and-push-bicep-types").match(
  /^    if: (.+)$/m
)[1];
const directIf = job(main, "publish-bicep-types-ghcr").match(
  /^    if: (.+)$/m
)[1];

function route({
  flag = "",
  changed = "false",
  changesResult = "success",
  repository = "radius-project/radius",
  ref = "refs/heads/main",
  event = "push",
  protectedRef = true
} = {}) {
  let mode = "";
  let failed = false;
  const [owner, repo] = repository.split("/");
  runInNewContext(`(() => {${routeScript}})()`, {
    context: { repo: { owner, repo }, ref, eventName: event },
    process: {
      env: {
        PUBLISH_ENABLED: flag,
        ONLY_CHANGED: changed,
        CHANGES_RESULT: changesResult
      }
    },
    core: {
      setOutput: (_, value) => {
        mode = value;
      },
      setFailed: () => {
        failed = true;
      },
      notice: () => {}
    }
  });
  const scope = {
    github: { repository, ref, event_name: event, ref_protected: protectedRef },
    needs: { mode: { outputs: { mode } } }
  };
  const evaluate = (expression) =>
    runInNewContext(
      `Boolean(${expression.replaceAll("needs.bicep-publish-mode", "needs.mode")})`,
      scope
    );
  const legacyRuns = !failed && evaluate(legacyIf);
  const directRuns = !failed && evaluate(directIf);
  if (directRuns) {
    for (const name of ["bundle", "publish"]) {
      assert.equal(evaluate(block(job(direct, name), "if: >-", 6)), true);
    }
  }
  assert.equal(legacyRuns && directRuns, false, "Never select both publishers");
  const result = spawnSync(
    "python3",
    ["-B", ".github/scripts/bicep-types.py", "summary"],
    {
      encoding: "utf8",
      env: {
        ...process.env,
        BICEP_MODE: mode,
        BICEP_MODE_RESULT: failed ? "failure" : "success",
        BICEP_LEGACY_RESULT: legacyRuns ? "success" : "skipped",
        BICEP_DIRECT_RESULT: directRuns ? "success" : "skipped",
        BICEP_PUBLICATION_STATUS: directRuns ? "published" : ""
      }
    }
  );
  return { mode, legacyRuns, directRuns, exit: result.status };
}

for (const flag of [
  "",
  "false",
  "FALSE",
  "TRUE",
  "yes",
  "1",
  "true ",
  " true"
]) {
  test(`unset/invalid activation ${JSON.stringify(flag)} retains only legacy main publishing`, () => {
    assert.deepEqual(route({ flag }), {
      mode: "legacy",
      legacyRuns: true,
      directRuns: false,
      exit: 0
    });
  });
}

for (const event of ["push", "workflow_dispatch"]) {
  test(`protected official main ${event} selects direct only`, () => {
    assert.deepEqual(route({ flag: "true", event }), {
      mode: "direct",
      legacyRuns: false,
      directRuns: true,
      exit: 0
    });
  });
  test(`unprotected main ${event} fails rather than accepting a skip`, () => {
    assert.deepEqual(route({ flag: "true", event, protectedRef: false }), {
      mode: "direct",
      legacyRuns: false,
      directRuns: false,
      exit: 1
    });
  });
}

for (const [name, options] of [
  ["fork", { repository: "fork/radius" }],
  ["PR", { event: "pull_request" }],
  ["PR target", { event: "pull_request_target" }],
  ["merge group", { event: "merge_group" }],
  ["tag", { ref: "refs/tags/v0.61.0" }],
  ["manual feature", { event: "workflow_dispatch", ref: "refs/heads/topic" }]
]) {
  test(`${name} cannot enter either main publishing path`, () => {
    assert.deepEqual(route({ flag: "true", ...options }), {
      mode: "skip",
      legacyRuns: false,
      directRuns: false,
      exit: 0
    });
  });
}

for (const flag of ["", "true", "invalid"]) {
  test(`docs-only with flag=${flag} intentionally skips both publishers`, () => {
    assert.deepEqual(route({ flag, changed: "true" }), {
      mode: "skip",
      legacyRuns: false,
      directRuns: false,
      exit: 0
    });
  });
  for (const [changesResult, changed] of [
    ["failure", "true"],
    ["cancelled", "false"],
    ["skipped", ""],
    ["", ""],
    ["success", ""],
    ["success", "unknown"]
  ]) {
    test(`flag=${flag} detection ${changesResult}/${changed} fails closed`, () => {
      assert.deepEqual(route({ flag, changesResult, changed }), {
        mode: "",
        legacyRuns: false,
        directRuns: false,
        exit: 1
      });
    });
  }
}

test("credential separation, bundle reuse, and summary dependencies are wired", () => {
  const bundle = job(direct, "bundle");
  const publish = job(direct, "publish");
  assert.doesNotMatch(
    bundle,
    /packages: write|id-token: write|secrets\.|login-action|azure\/login/
  );
  assert.match(
    bundle,
    /permissions:\n      contents: read\n      actions: read/
  );
  assert.match(bundle, /steps\.select\.outputs\.reuse == 'false'/);
  assert.match(bundle, /make generate-bicep-types VERSION=edge/);
  assert.match(bundle, /compression-level: 0/);
  assert.match(
    publish,
    /concurrency:\n      group: radius-bicep-types-main-publish\n      cancel-in-progress: false/
  );
  assert.match(publish, /environment: publish-bicep/);
  assert.doesNotMatch(
    publish,
    /make generate|publish-extension|download-artifact|secrets: inherit/
  );
  assert.match(
    publish,
    /ARTIFACT_ID: \$\{\{ needs\.bundle\.outputs\.artifact_id \}\}/
  );
  assert.match(
    publish,
    /ARTIFACT_DIGEST: \$\{\{ needs\.bundle\.outputs\.artifact_digest \}\}/
  );
  assert.match(publish, /az acr login --name biceptypes/);
  assert.match(publish, /steps\.prepare\.outcome == 'success'/);
  assert.match(main, /cancel-in-progress: false/);
  const summary = job(main, "build-summary");
  for (const dependency of [
    "changes",
    "bicep-publish-mode",
    "build-and-push-bicep-types",
    "publish-bicep-types-ghcr"
  ]) {
    assert.ok(summary.match(/^    needs: (.+)$/m)[1].includes(dependency));
  }
  assert.match(summary, /python3 \.github\/scripts\/bicep-types\.py summary/);
});

test("tag publishing and external legacy payload remain independent", () => {
  const release = readFileSync(".github/workflows/build-release.yaml", "utf8");
  const legacy = readFileSync(
    ".github/workflows/__build-bicep-types.yaml",
    "utf8"
  );
  const caller = job(release, "build-and-push-bicep-types");
  assert.match(caller, /__build-bicep-types.yaml/);
  assert.doesNotMatch(caller, /packages:|id-token:|__publish-bicep-types/);
  assert.match(legacy, /repository: azure-octo\/radius-publisher/);
  assert.match(legacy, /"rel_channel": "\$\{\{ env.REL_CHANNEL \}\}"/);
  assert.match(legacy, /"registry_target": "radius"/);
  assert.doesNotMatch(legacy, /BICEP_GHCR_PUBLISH_ENABLED|bicep-types\.py/);
});
