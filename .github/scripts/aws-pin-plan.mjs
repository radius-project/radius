// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { radiusSubmodulePin } from "./verify-aws-final-inputs.mjs";

const repo = "radius-project/bicep-types-aws";
const oid = /^[0-9a-f]{40}$/;
const verifier = fileURLToPath(
  new URL("./verify-bicep-types-aws-radius-pin.sh", import.meta.url)
);

export async function requireCheck(github, targetRepo, branch, name) {
  const rules = await github.paginate(
    "GET /repos/{owner}/{repo}/rules/branches/{branch}",
    { owner: "radius-project", repo: targetRepo, branch, per_page: 100 }
  );
  if (
    !rules.some(
      (rule) =>
        rule.type === "required_status_checks" &&
        rule.parameters.required_status_checks.some(
          (check) => check.context === name && check.integration_id === 15368
        )
    )
  )
    throw new Error(
      `Require the GitHub Actions check "${name}" on radius-project/${targetRepo}:${branch} before release preparation`
    );
}

export function git(directory, ...args) {
  return execFileSync("git", ["-C", directory, ...args], {
    encoding: "utf8"
  }).trim();
}

export function awsSource(plan) {
  const entries = plan.siblingRepositories.filter(
    (s) => s.name === "bicep-types-aws"
  );
  if (
    entries.length !== 1 ||
    entries[0].repository !== repo ||
    !oid.test(entries[0].sourceCommit) ||
    !oid.test(plan.source.productCommit) ||
    !/^(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(plan.channel) ||
    !["main", `release/${plan.channel}`].includes(entries[0].sourceRef)
  ) {
    throw new Error("Invalid frozen AWS or Radius source");
  }
  return entries[0];
}

// Compute the reviewed Git tree without changing the checkout or its index.
export function intent(directory, base, product) {
  if (
    !oid.test(base) ||
    !oid.test(product) ||
    git(directory, "cat-file", "-t", base) !== "commit"
  )
    throw new Error(
      "Expected an AWS base commit and a full Radius product SHA"
    );
  const pinned = radiusSubmodulePin(directory, base);
  if (!pinned) {
    execFileSync("bash", [verifier, directory, base, product], {
      stdio: "inherit"
    });
    return null; // Only a source that never had the submodule can get here.
  }
  const scratch = mkdtempSync(join(tmpdir(), "radius-pin-index-"));
  try {
    const options = {
      encoding: "utf8",
      env: { ...process.env, GIT_INDEX_FILE: join(scratch, "index") }
    };
    const index = (...args) =>
      execFileSync("git", ["-C", directory, ...args], options).trim();
    index("read-tree", base);
    index("update-index", "--cacheinfo", `160000,${product},radius`);
    return { productCommit: product, expectedTree: index("write-tree") };
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

export function verifyIntent(directory, source, product) {
  const expected = intent(directory, source.sourceCommit, product);
  const approved = source.radiusPin;
  if (
    !expected ||
    !approved ||
    Object.keys(approved).length !== 2 ||
    expected.productCommit !== approved.productCommit ||
    expected.expectedTree !== approved.expectedTree
  )
    throw new Error(
      "The approved AWS pin intent does not match the frozen base and Radius product"
    );
}

export function capture(plan, remote = `https://github.com/${repo}.git`) {
  const source = awsSource(plan);
  const directory = mkdtempSync(join(tmpdir(), "radius-pin-source-"));
  try {
    git(directory, "init", "--quiet");
    git(
      directory,
      "fetch",
      "--quiet",
      "--no-tags",
      remote,
      `refs/heads/${source.sourceRef}`
    );
    git(
      directory,
      "merge-base",
      "--is-ancestor",
      source.sourceCommit,
      "FETCH_HEAD"
    );
    if (Object.hasOwn(source, "radiusPin"))
      verifyIntent(directory, source, plan.source.productCommit);
    else {
      const pin = intent(
        directory,
        source.sourceCommit,
        plan.source.productCommit
      );
      if (pin) source.radiusPin = pin;
    }
    return plan.siblingRepositories;
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

if (
  process.argv[1] &&
  pathToFileURL(process.argv[1]).href === import.meta.url
) {
  const [mode, file, product, value] = process.argv.slice(2);
  if (mode === "capture") {
    const siblings = JSON.parse(readFileSync(file, "utf8"));
    const updated = capture({
      channel: value,
      source: { productCommit: product },
      siblingRepositories: siblings
    });
    writeFileSync(file, `${JSON.stringify(updated, null, 2)}\n`);
  } else if (mode === "verify") {
    verifyIntent(file, JSON.parse(value), product);
  } else {
    throw new Error(
      "Expected capture <siblings.json> <product> <channel> or verify <checkout> <product> <AWS entry>"
    );
  }
}
