// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";

// These are the inputs the AWS generator reads from its Radius submodule.
const schemas = [
  "pkg/aws/schemas/native",
  "pkg/aws/schemas/handwritten",
  "pkg/aws/extraproperties/extra_properties.json",
];

function git(repo, ...args) {
  return execFileSync("git", ["--no-replace-objects", "-C", repo, ...args], {
    encoding: "utf8",
    env: {
      ...Object.fromEntries(
        Object.entries(process.env).filter(([k]) => !k.startsWith("GIT_")),
      ),
      GIT_NO_LAZY_FETCH: "1",
      GIT_ALLOW_PROTOCOL: "",
      GIT_OPTIONAL_LOCKS: "0",
    },
  });
}

function entries(repo, commit, paths) {
  const tree = git(
    repo,
    "ls-tree",
    "--full-tree",
    "-tz",
    commit,
    "--",
    ...paths,
  );
  return new Map(
    tree
      .split("\0")
      .filter(Boolean)
      .map((row) => {
        const tab = row.indexOf("\t");
        return [row.slice(tab + 1), row.slice(0, tab)];
      }),
  );
}

function changes(repo, from, to) {
  const fields = git(
    repo,
    "diff-tree",
    "-r",
    "-t",
    "-z",
    "--raw",
    "--no-commit-id",
    "--no-renames",
    "--ignore-submodules=none",
    from,
    to,
    "--",
  ).split("\0");
  const result = new Map();
  for (let i = 0; i < fields.length - 1; i += 2) {
    const [before, after] = fields[i].slice(1).split(" ");
    result.set(fields[i + 1], `${before}:${after}`);
  }
  return result;
}

export function radiusSubmodulePin(repo, commit) {
  const tree = entries(repo, commit, ["radius", ".gitmodules"]);
  const entry = tree.get("radius");
  const link = /^160000 commit ([a-f0-9]{40})$/.exec(entry);
  if (entry)
    assert(
      link,
      `AWS commit ${commit} has a radius path that is not a gitlink`,
    );
  else
    assert(
      !git(repo, "rev-list", "--full-history", "-1", commit, "--", "radius"),
      `AWS commit ${commit} removed its radius submodule`,
    );
  if (!link && !tree.has(".gitmodules")) return null;
  assert(
    /^100644 blob /.test(tree.get(".gitmodules")),
    `Malformed radius submodule at ${commit}`,
  );
  const config = git(
    repo,
    "config",
    "--no-includes",
    "--null",
    "--blob",
    `${commit}:.gitmodules`,
    "--list",
  ).split("\0");
  const mapping = config.filter((kv) =>
    /^submodule\..+\.path\nradius$/.test(kv),
  );
  if (!link) {
    assert(
      !mapping.length &&
        !config.some((kv) => kv.startsWith("submodule.radius.")),
      "AWS declares radius without a gitlink",
    );
    return null;
  }
  const section = mapping[0]?.slice(0, -".path\nradius".length);
  const values = (key) =>
    config
      .filter((kv) => kv === key || kv.startsWith(`${key}\n`))
      .map((kv) => kv.slice(key.length + 1));
  const urls = values(`${section}.url`);
  assert(
    mapping.length === 1 &&
      values(`${section}.path`).length === 1 &&
      urls.length === 1 &&
      /^https:\/\/github\.com\/radius-project\/radius(?:\.git)?$/.test(urls[0]),
    `Malformed radius submodule at ${commit}`,
  );
  return link[1];
}

function schemaInputs(repo, commit) {
  const paths = [
    ...new Set(
      schemas.flatMap((path) =>
        path.split("/").map((_, i, parts) => parts.slice(0, i + 1).join("/")),
      ),
    ),
  ];
  const tree = entries(repo, commit, paths);
  for (const path of paths) {
    const entry = tree.get(path);
    if (!entry) continue;
    assert(
      (path.endsWith(".json") ? /^100(?:644|755) blob / : /^040000 tree /).test(
        entry,
      ),
      `Malformed Radius schema input ${path} at ${commit}`,
    );
  }
  return schemas.map((path) => tree.get(path));
}

export function verifyFinalInputs({
  radius,
  aws,
  rcPlan,
  version,
  product,
  landed,
  base,
}) {
  const raw = rcPlan.source.productCommit;
  try {
    for (const [repo, cutoff] of [
      [radius, raw],
      [radius, product],
      [aws, landed],
      [aws, base],
    ]) {
      assert(/^[a-f0-9]{40}$/.test(cutoff), "Expected a full commit SHA");
      assert.equal(
        git(repo, "cat-file", "-t", cutoff).trim(),
        "commit",
        `${cutoff} is not a commit`,
      );
    }
    // Both checkouts are fetch-depth: 0; a shallow history cannot prove a submodule never existed.
    for (const repo of [radius, aws])
      assert.equal(
        git(repo, "rev-parse", "--is-shallow-repository").trim(),
        "false",
        "Expected a full checkout",
      );

    const path = `.github/release-plans/${version}.yaml`;
    assert(
      rcPlan.version === version &&
        rcPlan.releaseType === "rc" &&
        rcPlan.releasePlanPath === path,
      `Invalid committed ${version} RC plan`,
    );
    assert.equal(
      git(radius, "rev-parse", `refs/tags/${version}^{commit}`).trim(),
      product,
      "Final product is not the tagged commit of its validated RC",
    );
    assert.equal(
      git(radius, "show", "-s", "--format=%P", product).trim(),
      raw,
      "RC product must be the only parent",
    );
    const metadata = new Map([
      [path, "000000:100644"],
      [`docs/release-notes/${version}.md`, "000000:100644"],
      ["versions.yaml", "100644:100644"],
    ]);
    if (rcPlan.schemaVersion === 2 && rcPlan.changelogPolicy == null)
      metadata.set("CHANGELOG.md", "100644:100644");
    else
      assert.equal(
        rcPlan.changelogPolicy,
        "stable-only",
        "Unknown RC changelog policy",
      );
    const changed = changes(radius, raw, product);
    for (const dir of [
      ".github",
      ".github/release-plans",
      "docs",
      "docs/release-notes",
    ]) {
      if (["000000:040000", "040000:040000"].includes(changed.get(dir)))
        changed.delete(dir);
    }
    assert.deepEqual(changed, metadata, "Unexpected Radius product change");

    const rcPin = radiusSubmodulePin(aws, landed);
    const awsChanges = changes(aws, landed, base);
    const nextPin = awsChanges.has("radius")
      ? radiusSubmodulePin(aws, base)
      : rcPin;
    assert(
      [...awsChanges.keys()].every((name) => name === "radius"),
      "AWS build input changed",
    );
    // No other change, including .gitmodules, is permitted. Only the gitlink can differ.
    assert.equal(
      Boolean(rcPin),
      Boolean(nextPin),
      "AWS radius gitlink was added or removed",
    );
    if (!rcPin) return;
    assert.equal(rcPin, raw, `AWS RC pin is not RC product ${raw}`);
    // The Radius diff above proves the final product is schema-identical to the RC.
    // A changed AWS base pin is independent: it must not introduce untested generator inputs.
    const tested = schemaInputs(radius, raw);
    if (nextPin !== raw && nextPin !== product) {
      assert.equal(
        git(radius, "cat-file", "-t", nextPin).trim(),
        "commit",
        "AWS base pin is not a Radius commit",
      );
      assert.deepEqual(
        schemaInputs(radius, nextPin),
        tested,
        "AWS gitlink changes Radius schema inputs",
      );
    }
  } catch (error) {
    throw new Error(
      `${error.message}. Cut another RC and validate it before releasing the final.`,
      { cause: error },
    );
  }
}
