// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const source = (path) => fileURLToPath(new URL(`../${path}`, import.meta.url));
const script = source("scripts/verify-aws-pin-readers.sh");
const readers = [
  "scripts/aws-pin-receipt.mjs",
  "scripts/stage-release-bicep.mjs",
  "scripts/verify-release-publication.sh"
];
const released = JSON.parse(
  execFileSync("yq", ["-o=json", ".", source("workflows/build-release.yaml")], {
    encoding: "utf8"
  })
);
const step = (workflow, job, name) =>
  workflow.jobs[job].steps.find((candidate) => candidate.name === name);

test("pinned-source guard requires every named consumer to read the receipt plan", async (t) => {
  const directory = mkdtempSync(join(tmpdir(), "aws-pin-readers-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync("git", args, { cwd: directory, encoding: "utf8" }).trim();
  git("init", "--quiet", "-b", "main");
  git("config", "user.name", "Radius Test");
  git("config", "user.email", "test@example.com");
  git("config", "commit.gpgsign", "false");
  mkdirSync(join(directory, ".github", "scripts"), { recursive: true });
  mkdirSync(join(directory, ".github", "workflows"), { recursive: true });
  const install = (workflow, files = {}) => {
    for (const path of readers) {
      const target = join(directory, ".github", path);
      if (path in files) writeFileSync(target, files[path]);
      else copyFileSync(source(path), target);
    }
    writeFileSync(
      join(directory, ".github", "workflows", "build-release.yaml"),
      JSON.stringify(workflow)
    );
    git("add", "-A");
    git("commit", "--allow-empty", "-qm", "install readers");
    return git("rev-parse", "HEAD");
  };
  const guard = (sha) =>
    execFileSync("bash", [script, sha], {
      cwd: directory,
      encoding: "utf8",
      stdio: "pipe"
    });

  guard(install(released));

  const cases = [
    {
      name: "promotion reads the base plan while the RC recheck reads the receipt",
      error: "finalize-release does not pass",
      mutate: (workflow) => {
        step(
          workflow,
          "finalize-release",
          "Promote locked Bicep extensions"
        ).env.INPUT_PLAN_FILE = "dist/verification/plan.json";
      }
    },
    {
      name: "post-approval output recheck drops the receipt plan",
      error: "finalize-release does not pass",
      mutate: (workflow) => {
        delete step(
          workflow,
          "finalize-release",
          "Recheck outputs after approval"
        ).env.RELEASE_VERIFY_PLAN_FILE;
      }
    },
    {
      name: "pre-approval output verification drops the receipt plan",
      error: "verify-release does not pass",
      mutate: (workflow) => {
        delete step(
          workflow,
          "verify-release",
          "Verify required outputs and installation"
        ).env.RELEASE_VERIFY_PLAN_FILE;
      }
    },
    {
      name: "the receipt step imports the reader under another id",
      error: "publish-sibling-releases does not pass",
      mutate: (workflow) => {
        step(
          workflow,
          "publish-sibling-releases",
          "Resolve tag-owned AWS pin"
        ).id = "receipt";
      }
    },
    {
      name: "the receipt resolves after its consumer",
      error: "publish-sibling-releases does not pass",
      mutate: (workflow) => {
        const steps = workflow.jobs["publish-sibling-releases"].steps;
        const reader = steps.findIndex(
          (candidate) => candidate.id === "aws-pin"
        );
        steps.push(...steps.splice(reader, 1));
      }
    },
    {
      name: "Bicep staging reads the base plan",
      error: "stage-release-bicep.mjs does not use",
      files: {
        "scripts/stage-release-bicep.mjs": "plan = JSON.parse(planText);\n"
      }
    },
    {
      name: "publication verification ignores the receipt plan",
      error: "verify-release-publication.sh does not read",
      files: {
        "scripts/verify-release-publication.sh":
          'PLAN="${DIRECTORY}/plan.json"\n'
      }
    }
  ];
  for (const { name, error, mutate, files } of cases) {
    await t.test(name, () => {
      const workflow = structuredClone(released);
      mutate?.(workflow);
      assert.throws(
        () => guard(install(workflow, files)),
        (caught) => new RegExp(error).test(`${caught.stderr ?? ""}`)
      );
    });
  }
});
