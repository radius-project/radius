// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, before, test } from "node:test";
import { verifyFinalInputs } from "./verify-aws-final-inputs.mjs";

const root = mkdtempSync(join(tmpdir(), "radius-aws-final-"));
// Never inherit a caller's index, object directory, hooks or signing settings
// for the Git commands that write fixtures.
const env = {
  ...Object.fromEntries(
    Object.entries(process.env).filter(([name]) => !name.startsWith("GIT_"))
  ),
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_CONFIG_GLOBAL: join(root, "unused.gitconfig"),
  GIT_ALLOW_PROTOCOL: ""
};
const schema = "pkg/aws/schemas/native/type.json";
const handwritten = "pkg/aws/schemas/handwritten/subdir/input.json";
const extra = "pkg/aws/extraproperties/extra_properties.json";
const beforeModules =
  '[submodule "bicep-types"]\n\tpath = bicep-types\n\turl = https://github.com/Azure/bicep-types\n';
const modules = `${beforeModules}[submodule "radius"]\n\tpath = radius\n\turl = https://github.com/radius-project/radius\n\tshallow = true\n`;
const gitlink = (id) => ["160000", id];
let radius, aws, rc, baseline, legacy, landed;
let sequence = 0;

// Build real trees in private fixture indexes. No clone, checkout, submodule
// initialization, remote, or existing release tag is needed.
function repository(name) {
  const dir = join(root, name);
  mkdirSync(dir);
  function git(args, input) {
    return execFileSync(
      "git",
      [
        "--no-pager",
        "-C",
        dir,
        "-c",
        "user.name=Fixture",
        "-c",
        "user.email=fixture@example.invalid",
        "-c",
        "commit.gpgSign=false",
        "-c",
        "tag.gpgSign=false",
        "-c",
        "core.protectNTFS=false",
        ...args
      ],
      { encoding: "utf8", input, env }
    ).trimEnd();
  }
  git(["init", "--quiet", "--template="]);
  function blob(contents) {
    return git(["hash-object", "-w", "--stdin"], contents);
  }
  function commit(parent, files = {}) {
    git(["read-tree", parent ?? "--empty"]);
    const index = Object.entries(files)
      .map(([path, value]) => {
        const [mode, id] =
          value === null ? ["0", "0".repeat(40)]
          : typeof value === "string" ? ["100644", blob(value)]
          : value;
        return `${mode} ${id}\t${path}\0`;
      })
      .join("");
    git(["update-index", "-z", "--index-info"], index);
    return git([
      "commit-tree",
      git(["write-tree"]),
      ...(parent ? ["-p", parent] : []),
      "-m",
      `fixture ${++sequence}`
    ]);
  }
  return { dir, git, blob, commit };
}

function metadata({ files = {}, plan = {}, oldPolicy = false } = {}) {
  const version = `v0.63.0-rc.${++sequence}`;
  const planPath = `.github/release-plans/${version}.yaml`;
  const notes = `docs/release-notes/${version}.md`;
  const rcPlan = {
    schemaVersion: 2,
    version,
    releaseType: "rc",
    releasePlanPath: plan.path ?? planPath,
    source: { productCommit: plan.product ?? rc },
    ...(oldPolicy ? {} : { changelogPolicy: "stable-only" })
  };
  const cutoff = radius.commit(rc, {
    [planPath]: JSON.stringify(rcPlan),
    [notes]: "Curated RC notes\n",
    "versions.yaml": `supported:\n  - channel: "0.63"\n    version: ${version}\n`,
    ...(oldPolicy ?
      { "CHANGELOG.md": "Historical generated RC changelog\n" }
    : {}),
    ...(typeof files === "function" ? files({ notes, planPath }) : files)
  });
  radius.git(["tag", "--no-sign", "-a", version, cutoff, "-m", "validated RC"]);
  return { cutoff, version, rcPlan };
}

before(() => {
  radius = repository("Radius checkout");
  aws = repository("AWS checkout");
  rc = radius.commit(null, {
    "versions.yaml": "supported: []\n",
    "CHANGELOG.md": "Unreleased\n",
    "cmd/rad/main.go": "package main\n",
    [schema]: "native\n",
    [handwritten]: "handwritten\n",
    [extra]: "{}\n"
  });
  baseline = metadata();
  legacy = aws.commit(null, {
    ".gitmodules": beforeModules,
    "bicep-types": gitlink("1".repeat(40)),
    "src/aws-type-generator/src/main.ts": "generator\n",
    "artifacts/bicep/index.json": "types\n",
    "src/aws-type-generator/package-lock.json": "dependencies\n"
  });
  landed = aws.commit(legacy, { ".gitmodules": modules, radius: gitlink(rc) });
  aws.git(["config", "diff.ignoreSubmodules", "all"]);
  writeFileSync(
    join(aws.dir, ".gitmodules"),
    "This is not the committed declaration\n"
  );
});
after(() => rmSync(root, { recursive: true, force: true }));

function verify(
  {
    product = baseline,
    awsRc = landed,
    base = awsRc,
    version = product.version
  } = {},
  failure
) {
  const run = () =>
    verifyFinalInputs({
      radius: radius.dir,
      aws: aws.dir,
      rcPlan: product.rcPlan,
      version,
      product: product.cutoff,
      landed: awsRc,
      base
    });
  if (failure) assert.throws(run, failure);
  else run();
}

test("promotes exactly the tagged RC metadata and a schema-neutral AWS pin, without modifying either checkout", () => {
  const advanced = radius.commit(rc, {
    "pkg/aws/unrelated.go": "not a generator input"
  });
  const base = aws.commit(landed, { radius: gitlink(advanced) });
  const snapshot = () =>
    [radius, aws].map((repo) => [
      repo.git(["for-each-ref"]),
      readFileSync(join(repo.dir, ".git", "index"))
    ]);
  const before = snapshot();
  verify();
  verify({ base });
  assert.deepEqual(snapshot(), before);
  verify({ base: aws.commit(landed, { radius: gitlink(baseline.cutoff) }) });
});

test("accepts historical RC changelog generation and genuinely pre-submodule AWS trees", () => {
  verify({
    product: metadata({ oldPolicy: true }),
    awsRc: legacy,
    base: aws.commit(legacy)
  });
});

test("rejects product code, handwritten docs, other versions' metadata and disallowed modes", () => {
  for (const files of [
    { "cmd/rad/main.go": "changed product\n" },
    { "docs/release-notes/runbook.md": "handwritten\n" },
    { ".github/release-plans/v0.62.0-rc.6.yaml": "another version\n" },
    { "docs/release-notes/v0.62.0-rc.6.md": "another version\n" },
    { "CHANGELOG.md": "not generated by a stable-only RC\n" },
    ({ notes }) => ({
      [`${notes}\nversions.yaml`]: "one path, not two allowed files"
    }),
    ({ notes }) => ({ [notes]: ["120000", radius.blob("../../somewhere")] })
  ]) {
    verify(
      { product: metadata({ files }) },
      /Unexpected Radius product change/
    );
  }
});

test("binds the raw product SHA, plan path and final cutoff to the actual RC tag", () => {
  verify({ version: "v0.63.0-rc.99999" }, /committed .* RC plan/);
  verify(
    { product: { ...baseline, cutoff: radius.commit(baseline.cutoff) } },
    /tagged commit/
  );
  verify(
    { product: metadata({ plan: { product: baseline.cutoff } }) },
    /only parent/
  );
  verify(
    { product: metadata({ plan: { path: "docs/release-notes/extra.md" } }) },
    /committed .* RC plan/
  );
});

test("no AWS generator, artifact, dependency, declaration or second gitlink may drift", () => {
  for (const [file, value] of [
    ["src/aws-type-generator/src/main.ts", "new generator"],
    ["artifacts/bicep/index.json", null],
    ["src/aws-type-generator/package-lock.json", "new dependency"],
    [".gitmodules", `${modules}\n`],
    ["bicep-types", gitlink("2".repeat(40))]
  ]) {
    verify(
      {
        base: aws.commit(landed, {
          [file]: value,
          radius: gitlink(baseline.cutoff)
        })
      },
      /AWS build input changed/
    );
  }
});

test("compares all generator inputs at the actual AWS gitlinks, not just at the product cutoffs", () => {
  for (const path of [schema, handwritten, extra]) {
    const changed = radius.commit(rc, {
      [path]: path === extra ? null : "changed schema"
    });
    verify(
      { base: aws.commit(landed, { radius: gitlink(changed) }) },
      /gitlink changes Radius schema input/
    );
  }
  verify(
    { base: aws.commit(landed, { radius: gitlink(landed) }) },
    /not get object info|not a valid object/i
  );
  verify(
    { awsRc: aws.commit(landed, { radius: gitlink(baseline.cutoff) }) },
    /not RC product/
  );
});

test("never treats a removed, malformed, newly introduced or dangling submodule as legacy", () => {
  const removed = aws.commit(landed, {
    radius: null,
    ".gitmodules": beforeModules
  });
  verify({ base: removed }, /removed its radius submodule/);
  verify({ awsRc: removed }, /removed its radius submodule/);
  verify(
    { awsRc: aws.commit(legacy, { radius: "plain file" }) },
    /not a gitlink/
  );
  verify(
    { awsRc: aws.commit(legacy, { ".gitmodules": modules }) },
    /without a gitlink/
  );
  verify(
    { awsRc: aws.commit(legacy, { radius: gitlink(rc) }) },
    /Malformed radius submodule/
  );
  verify(
    {
      awsRc: aws.commit(landed, {
        ".gitmodules": modules.replace(
          "radius-project/radius",
          "unrelated/radius"
        )
      })
    },
    /Malformed radius submodule/
  );
  verify(
    { awsRc: legacy, base: landed },
    /AWS build input changed|added or removed/
  );
});

test("fails closed on refs instead of full SHAs, non-commits and shallow history", () => {
  verify({ base: "HEAD" }, /full commit SHA/);
  verify(
    { base: aws.git(["rev-parse", `${landed}^{tree}`]) },
    /is not a commit/
  );
  const shallow = join(aws.dir, ".git", "shallow");
  writeFileSync(shallow, `${legacy}\n`);
  try {
    verify({ awsRc: legacy }, /full checkout/);
  } finally {
    rmSync(shallow);
  }
});
