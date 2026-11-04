// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { test } from "node:test";

// Trusted-checkout manifest from functional-test-cloud.yaml at
// 0eeb04408e441ccc84030aaa6cf04988fd6b67ba. Only executable base dependencies,
// not the candidate workflow's permissions, secrets, or publishing job graph.
const contextHelper = ".github/scripts/cloud-test-context.mjs";
const artifactHelper = ".github/scripts/cloud-test-artifacts.mjs";
const azureHelper = ".github/scripts/azure-oidc-login.mjs";
const registryAction = ".github/actions/create-local-registry";
const trustedSHA = "${{ needs.setup.outputs.TRUSTED_SHA }}";
const checkouts = [
  {
    job: "setup",
    ref: "refs/heads/main",
    imports: [{ file: contextHelper, exports: ["setup"] }]
  },
  {
    job: "build",
    ref: "refs/heads/main",
    actions: [registryAction]
  },
  {
    job: "build",
    ref: trustedSHA,
    path: ".cloud-test-tools",
    sparse: artifactHelper,
    imports: [{ file: artifactHelper, exports: ["capture"] }]
  },
  ...["upload-ghcr", "upload-test-types"].map((job) => ({
    job,
    ref: trustedSHA,
    imports: [
      { file: artifactHelper, exports: ["select", "prepare", "publish"] }
    ],
    make: "install-oras"
  })),
  {
    job: "upload-test-types",
    ref: trustedSHA,
    path: ".workflow-azure-login",
    sparse: azureHelper,
    imports: [{ file: azureHelper, exports: ["login"] }]
  },
  {
    job: "tests",
    ref: "${{ github.sha }}",
    path: ".workflow-azure-login",
    sparse: azureHelper,
    imports: [{ file: azureHelper, exports: ["login"] }]
  },
  {
    job: "report-test-results",
    ref: "${{ needs.setup.outputs.TRUSTED_SHA || 'refs/heads/main' }}",
    imports: [{ file: contextHelper, exports: ["testStatus"] }]
  }
];
const root = resolve(import.meta.dirname, "../..");
const dependencies = [
  contextHelper,
  artifactHelper,
  azureHelper,
  registryAction,
  "Makefile",
  "build"
];

async function fixture(t) {
  const directory = await mkdtemp(join(tmpdir(), "cloud-bootstrap-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const base = join(directory, "prepared-base");
  const candidate = join(directory, "candidate");
  for (const file of dependencies) {
    await mkdir(dirname(join(base, file)), { recursive: true });
    await cp(join(root, file), join(base, file), { recursive: true });
  }
  for (const file of [contextHelper, artifactHelper, azureHelper]) {
    await mkdir(dirname(join(candidate, file)), { recursive: true });
    await writeFile(
      join(candidate, file),
      'throw new Error("Candidate helpers must never execute");\n'
    );
  }
  await cp(join(base, registryAction), join(candidate, registryAction), {
    recursive: true
  });
  return { directory, base, candidate };
}

async function checkBootstrap({ directory, base, candidate }) {
  for (const [index, checkout] of checkouts.entries()) {
    assert.ok(
      [
        "refs/heads/main",
        trustedSHA,
        "${{ github.sha }}",
        "${{ needs.setup.outputs.TRUSTED_SHA || 'refs/heads/main' }}"
      ].includes(checkout.ref),
      `${checkout.job} must select a controller revision, not CHECKOUT_REF`
    );
    const workspace = join(directory, `job-${index}`);
    const destination = join(workspace, checkout.path ?? ".");
    await mkdir(destination, { recursive: true });
    // The activation fixture is separate from the prepared base. No source
    // comes from the candidate even when its missing helper is available there.
    if (checkout.sparse) {
      await mkdir(dirname(join(destination, checkout.sparse)), {
        recursive: true
      });
      await cp(join(base, checkout.sparse), join(destination, checkout.sparse));
    } else {
      await cp(base, destination, { recursive: true });
    }
    for (const entry of checkout.imports ?? []) {
      execFileSync(
        process.execPath,
        [
          "--input-type=module",
          "-e",
          `const helper = await import(process.env.GITHUB_WORKSPACE + "/" + ${JSON.stringify(
            join(checkout.path ?? ".", entry.file)
          )});
           for (const name of ${JSON.stringify(entry.exports)}) {
             if (typeof helper[name] !== "function") throw new Error("Missing export: " + name);
           }`
        ],
        {
          cwd: candidate,
          env: { ...process.env, GITHUB_WORKSPACE: workspace },
          stdio: "pipe"
        }
      );
    }
    for (const actionPath of checkout.actions ?? []) {
      const action = JSON.parse(
        execFileSync("yq", [
          "-o=json",
          ".",
          join(destination, actionPath, "action.yaml")
        ])
      );
      assert.equal(action.runs.using, "composite");
      for (const step of action.runs.steps) {
        if (step.run)
          execFileSync("bash", ["-n", "-c", step.run], { stdio: "pipe" });
      }
      const pull = action.runs.steps.find((step) => step.env?.PULL_SCRIPT);
      assert.equal(
        pull.env.PULL_SCRIPT,
        "${{ github.action_path }}/pull-image.sh"
      );
      execFileSync(
        "bash",
        ["-n", join(destination, actionPath, "pull-image.sh")],
        {
          stdio: "pipe"
        }
      );
    }
    if (checkout.make) {
      const command = execFileSync("make", ["-n", checkout.make], {
        cwd: destination,
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
      const installer = join(destination, "build/scripts/install-oras.sh");
      assert.match(
        await readFile(installer, "utf8"),
        /source .*retry-curl\.sh/
      );
      execFileSync("bash", ["-n", installer], { stdio: "pipe" });
      execFileSync(
        "bash",
        ["-n", join(destination, "build/scripts/retry-curl.sh")],
        {
          stdio: "pipe"
        }
      );
    }
  }
}

test("activation trusted imports, actions and tools resolve from the prepared base, not the candidate", async (t) => {
  await checkBootstrap(await fixture(t));
});

for (const missing of [
  contextHelper,
  artifactHelper,
  azureHelper,
  `${registryAction}/pull-image.sh`,
  "build/tools.generated.mk",
  "build/scripts/install-oras.sh",
  "build/scripts/retry-curl.sh"
]) {
  test(`bootstrap fails when the base lacks ${missing}, even with candidate helpers present`, async (t) => {
    const trees = await fixture(t);
    await rm(join(trees.base, missing));
    await assert.rejects(checkBootstrap(trees), (error) => {
      assert.match(
        `${error.message}\n${error.stderr ?? ""}`,
        /Cannot find module|ENOENT|No such file/
      );
      assert.doesNotMatch(
        `${error.message}\n${error.stderr ?? ""}`,
        /Candidate helpers must never execute/
      );
      return true;
    });
  });
}
