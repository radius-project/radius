// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const script = fileURLToPath(
  new URL("./verify-aws-pin-readers.sh", import.meta.url)
);
const planFile = "${{ steps.aws-pin.outputs.plan-file }}";

test("pinned-source guard requires receipt-aware staging and plan handoffs", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "aws-pin-readers-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync("git", args, { cwd: directory, encoding: "utf8" }).trim();
  git("init", "--quiet", "-b", "main");
  git("config", "user.name", "Radius Test");
  git("config", "user.email", "test@example.com");
  git("config", "commit.gpgsign", "false");

  const files = join(directory, ".github");
  mkdirSync(join(files, "scripts"), { recursive: true });
  mkdirSync(join(files, "workflows"), { recursive: true });
  writeFileSync(join(files, "scripts", "aws-pin-receipt.mjs"), "");
  const staging = join(files, "scripts", "stage-release-bicep.mjs");
  writeFileSync(staging, "plan = await tagPlan({ github, context });\n");
  const reader = { with: { script: 'await import("./aws-pin-receipt.mjs")' } };
  const workflow = {
    jobs: {
      "verify-release": {
        steps: [
          {
            ...reader,
            env: {
              INPUT_PLAN_FILE: planFile,
              RELEASE_VERIFY_PLAN_FILE: planFile
            }
          }
        ]
      },
      "finalize-release": {
        steps: [{ ...reader, env: { INPUT_PLAN_FILE: planFile } }]
      },
      "publish-sibling-releases": {
        steps: [{ ...reader, env: { INPUT_PLAN_FILE: planFile } }]
      }
    }
  };
  const workflowPath = join(files, "workflows", "build-release.yaml");
  writeFileSync(workflowPath, JSON.stringify(workflow));

  const commit = (message) => {
    git("add", "-A");
    git("commit", "-qm", message);
    return git("rev-parse", "HEAD");
  };
  const expectRejected = (sha, message) => {
    let error;
    try {
      execFileSync("bash", [script, sha], {
        cwd: directory,
        encoding: "utf8",
        stdio: "pipe"
      });
    } catch (caught) {
      error = caught;
    }
    assert.ok(error, `expected guard failure: ${message}`);
    assert.match(
      `${error.stdout ?? ""}${error.stderr ?? ""}`,
      new RegExp(message)
    );
  };

  const valid = commit("complete receipt handoff");
  execFileSync("bash", [script, valid], {
    cwd: directory,
    encoding: "utf8",
    stdio: "pipe"
  });

  writeFileSync(staging, "plan = JSON.parse(planText);\n");
  expectRejected(commit("legacy Bicep staging"), "receipt-resolved plan");

  writeFileSync(staging, "plan = await tagPlan({ github, context });\n");
  workflow.jobs["publish-sibling-releases"].steps[0].env.INPUT_PLAN_FILE =
    "dist/verification/plan.json";
  writeFileSync(workflowPath, JSON.stringify(workflow));
  expectRejected(
    commit("missing effective plan handoff"),
    "receipt-resolved plan"
  );
});
