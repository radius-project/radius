// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import { execFileSync } from "node:child_process";

const sha = /^[0-9a-f]{40}$/;
const rcTag = /^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-rc\.?[1-9]\d*$/;
const env = {
  ...process.env,
  GIT_NO_REPLACE_OBJECTS: "1",
  GIT_NO_LAZY_FETCH: "1",
  GIT_ALLOW_PROTOCOL: "",
  GIT_OPTIONAL_LOCKS: "0"
};
// In particular, a caller's GIT_DIR or GIT_SHALLOW_FILE must not override -C
// or turn removed-submodule history into apparent pre-submodule history.
for (const key of [
  "GIT_DIR",
  "GIT_WORK_TREE",
  "GIT_COMMON_DIR",
  "GIT_OBJECT_DIRECTORY",
  "GIT_ALTERNATE_OBJECT_DIRECTORIES",
  "GIT_GRAFT_FILE",
  "GIT_SHALLOW_FILE"
])
  delete env[key];

function requireThat(condition, message) {
  if (!condition) throw new Error(message);
}

function git(repo, ...args) {
  return execFileSync(
    "git",
    ["--no-pager", "--literal-pathspecs", "-C", repo, ...args],
    {
      env,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"]
    }
  );
}

function requireCommit(repo, id) {
  requireThat(
    sha.test(id),
    `Expected a full commit SHA, got ${JSON.stringify(id)}`
  );
  requireThat(
    git(repo, "cat-file", "-t", id).trimEnd() === "commit",
    `${id} is not a commit in ${repo}`
  );
}

function treeEntries(repo, commit, paths) {
  const raw = git(
    repo,
    "ls-tree",
    "--full-tree",
    "-tz",
    commit,
    "--",
    ...paths
  );
  const entries = new Map();
  for (const entry of raw.split("\0").filter(Boolean)) {
    const tab = entry.indexOf("\t");
    entries.set(entry.slice(tab + 1), entry.slice(0, tab));
  }
  return entries;
}

// With rename detection off, each pair is a raw header and one NUL-terminated
// path. Include tree entries so even a directory replacing a file is visible.
function* changes(repo, from, to) {
  const fields = git(
    repo,
    "diff-tree",
    "-r",
    "-t",
    "-z",
    "--raw",
    "--no-commit-id",
    "--no-renames",
    "--no-ext-diff",
    "--no-textconv",
    "--ignore-submodules=none",
    from,
    to,
    "--"
  ).split("\0");
  for (let i = 0; i < fields.length - 1; i += 2) {
    const [before, after] = fields[i].slice(1).split(" ");
    yield { path: fields[i + 1], before, after };
  }
}

function verifyProduct(radius, rc, final, requestedVersion) {
  const tags = git(
    radius,
    "tag",
    "--list",
    "--format=%(refname:lstrip=2)",
    "--points-at",
    final,
    "v*-rc*"
  )
    .split(/\r?\n/)
    .filter((tag) => rcTag.test(tag));
  const version = requestedVersion ?? (tags.length === 1 ? tags[0] : undefined);
  requireThat(
    tags.includes(version),
    `Final product ${final} must be the tagged commit of the validated RC`
  );
  requireThat(
    git(radius, "rev-list", "--parents", "-1", final).trimEnd() ===
      `${final} ${rc}`,
    `The tagged ${version} metadata commit must have the raw RC product ${rc} as its only parent`
  );

  const planPath = `.github/release-plans/${version}.yaml`;
  const plan = JSON.parse(
    execFileSync("yq", ["-o=json", ".", "-"], {
      input: git(radius, "cat-file", "blob", `${final}:${planPath}`),
      encoding: "utf8"
    })
  );
  requireThat(
    plan.version === version &&
      plan.releaseType === "rc" &&
      plan.releasePlanPath === planPath &&
      plan.source?.productCommit === rc,
    `The committed ${version} RC plan does not match the requested product and plan path`
  );

  // These are the file operations of the already validated RC, not path
  // prefixes to exempt from an arbitrary later commit.
  const expected = new Map([
    [planPath, "000000"],
    [`docs/release-notes/${version}.md`, "000000"],
    ["versions.yaml", "100644"]
  ]);
  if (plan.schemaVersion === 2 && plan.changelogPolicy == null) {
    expected.set("CHANGELOG.md", "100644"); // Pre-stable-only RC plans.
  } else {
    requireThat(
      plan.changelogPolicy === "stable-only",
      `Unknown changelog policy in ${planPath}`
    );
  }
  const seen = new Set();
  for (const { path, before, after } of changes(radius, rc, final)) {
    if (
      after === "040000" &&
      ["000000", "040000"].includes(before) &&
      [...expected.keys()].some((file) => file.startsWith(`${path}/`))
    )
      continue;
    requireThat(
      after === "100644" && expected.get(path) === before,
      `Unexpected Radius product change at ${JSON.stringify(path)} (${before} -> ${after})`
    );
    seen.add(path);
  }
  for (const path of expected.keys()) {
    requireThat(
      seen.has(path),
      `Missing generated ${version} metadata change: ${path}`
    );
  }
  return version;
}

function radiusPin(aws, commit) {
  const tree = treeEntries(aws, commit, ["radius", ".gitmodules"]);
  const entry = tree.get("radius");
  const link = /^160000 commit ([a-f0-9]{40})$/.exec(entry);
  if (entry) {
    requireThat(
      link,
      `AWS commit ${commit} has a radius path that is not a gitlink`
    );
  } else {
    requireThat(
      !git(aws, "rev-list", "--full-history", "-1", commit, "--", "radius"),
      `AWS commit ${commit} removed its radius submodule`
    );
  }
  verifyDeclaration(aws, commit, tree.get(".gitmodules"), Boolean(link));
  return link?.[1] ?? null;
}

function verifyDeclaration(aws, commit, entry, pin) {
  if (!entry && !pin) return;
  requireThat(
    /^100644 blob [a-f0-9]{40}$/.test(entry),
    `Missing or malformed .gitmodules at AWS commit ${commit}`
  );
  // Parse the committed blob, not checkout config. Never follow its URL.
  const fields = git(
    aws,
    "config",
    "--no-includes",
    "--null",
    "--blob",
    `${commit}:.gitmodules`,
    "--list"
  ).split("\0");
  const mappings = fields.filter((field) =>
    /^submodule\..+\.path\nradius$/.test(field)
  );
  if (!pin) {
    requireThat(
      !mappings.length &&
        !fields.some((field) => field.startsWith("submodule.radius.")),
      `AWS commit ${commit} declares radius without a gitlink`
    );
    return;
  }
  const malformed = `Malformed radius submodule declaration at AWS commit ${commit}`;
  requireThat(mappings.length === 1, malformed);
  const section = mappings[0].slice(0, -".path\nradius".length);
  const values = (key) =>
    fields
      .filter((field) => field === key || field.startsWith(`${key}\n`))
      .map((field) => field.slice(key.length + 1));
  const urls = values(`${section}.url`);
  requireThat(
    values(`${section}.path`).length === 1 &&
      urls.length === 1 &&
      /^https:\/\/github\.com\/radius-project\/radius(?:\.git)?$/.test(urls[0]),
    malformed
  );
}

// defaultRadiusPaths in bicep-types-aws/src/aws-type-generator/src/radius.ts.
// Compare the entire schema directories (recursively by tree ID) and the extra
// properties blob. A link in place of an input or its parent isn't evidence.
const schemaPaths = [
  "pkg/aws/schemas/native",
  "pkg/aws/schemas/handwritten",
  "pkg/aws/extraproperties/extra_properties.json"
];
const schemaPathsAndParents = [
  ...new Set(
    schemaPaths.flatMap((path) => {
      const parts = path.split("/");
      return parts.map((_, i) => parts.slice(0, i + 1).join("/"));
    })
  )
];

function schemaInputs(radius, commit) {
  const entries = treeEntries(radius, commit, schemaPathsAndParents);
  for (const path of schemaPathsAndParents) {
    if (!entries.has(path)) continue; // An older tree can lack a schema directory.
    const expected =
      path.endsWith(".json") ? /^100(?:644|755) blob / : /^040000 tree /;
    requireThat(
      expected.test(entries.get(path)),
      `Malformed Radius schema input ${path} at ${commit}`
    );
  }
  return entries;
}

function verifyAws(radius, aws, rcProduct, finalProduct, rcAws, finalAws) {
  const rcPin = radiusPin(aws, rcAws);
  const finalPin = rcAws === finalAws ? rcPin : radiusPin(aws, finalAws);
  for (const { path } of changes(aws, rcAws, finalAws)) {
    requireThat(
      path === "radius",
      `AWS build input changed: ${JSON.stringify(path)}`
    );
  }
  requireThat(
    (rcPin === null) === (finalPin === null),
    "The AWS radius gitlink was added or removed"
  );
  if (!rcPin) return;
  requireThat(
    rcPin === rcProduct,
    `RC AWS commit ${rcAws} pins ${rcPin}, not RC product ${rcProduct}`
  );
  // The frozen base pin need not be the pin that will land. Check both using
  // Radius objects; never resolve a gitlink in the AWS object database.
  const rcInputs = schemaInputs(radius, rcPin);
  for (const next of new Set([finalProduct, finalPin])) {
    if (next === rcPin) continue;
    if (next !== finalProduct) requireCommit(radius, next);
    const inputs = schemaInputs(radius, next);
    for (const path of schemaPaths) {
      requireThat(
        rcInputs.get(path) === inputs.get(path),
        `AWS gitlink changes Radius schema input ${path} (${rcPin} -> ${next})`
      );
    }
  }
}

try {
  const args = process.argv.slice(2);
  requireThat(
    args.length === 6 || args.length === 7,
    "Usage: verify-aws-final-inputs.sh <radius-dir> <aws-dir> <rc-product-sha> <final-product-sha> <rc-aws-landed-sha> <final-aws-base-sha> [rc-version]"
  );
  const [radius, aws, rc, final, rcAws, finalAws, version] = args;
  for (const repo of [radius, aws]) {
    requireThat(
      repo &&
        git(
          repo,
          "rev-parse",
          "--is-shallow-repository",
          "--show-prefix"
        ).trimEnd() === "false",
      `Expected a full checkout at its repository root: ${repo}`
    );
  }
  for (const [repo, id] of [
    [radius, rc],
    [radius, final],
    [aws, rcAws],
    [aws, finalAws]
  ]) {
    requireCommit(repo, id);
  }
  const verified = verifyProduct(radius, rc, final, version);
  verifyAws(radius, aws, rc, final, rcAws, finalAws);
  console.log(`Final Radius product and AWS build inputs match ${verified}.`);
} catch (error) {
  console.error(
    `Error: ${error.message}\nPlease cut another RC and validate it before releasing the final.`
  );
  process.exitCode = 1;
}
