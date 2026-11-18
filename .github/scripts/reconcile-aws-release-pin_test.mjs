// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { capture, verifyIntent } from "./aws-pin-plan.mjs";
import { reconcile } from "./reconcile-aws-release-pin.mjs";

const product = "b934a5984d5a6798f52643c8c4d0ad47bf535962";
const wrong = "6816fbfe9e9aeb84f7aba1046967061ce90f76c0";
const aws = "radius-project/bicep-types-aws";
const channel = "0.63",
  version = "v0.63.0-rc.1",
  target = `release/${channel}`;
const actor = { login: "radius-release[bot]", type: "Bot", id: 123 };

// Use actual Git objects for the input, signed proposal and rebase merge.
// Only GitHub's identity/signature/rules and PR transport are simulated.
function fixture(t, state = "wrong") {
  const dir = mkdtempSync(join(tmpdir(), "aws-pin-test-"));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync(
      "git",
      [
        "-c",
        "user.name=Fixture",
        "-c",
        "user.email=fixture@example.invalid",
        "-c",
        "commit.gpgsign=false",
        "-C",
        dir,
        ...args
      ],
      { encoding: "utf8" }
    ).trim();
  git("init", "--quiet", "-b", "main");
  writeFileSync(join(dir, "keep"), "original\n");
  git("add", "keep");
  if (state !== "legacy" && state !== "file")
    git(
      "update-index",
      "--add",
      "--cacheinfo",
      `160000,${state === "match" ? product : wrong},radius`
    );
  if (state === "file") {
    writeFileSync(join(dir, "radius"), "not a gitlink\n");
    git("add", "radius");
  }
  git("commit", "-qm", "base");
  if (state === "removed") {
    git("update-index", "--force-remove", "radius");
    git("commit", "-qm", "remove submodule");
  }
  const base = git("rev-parse", "HEAD");
  const siblings = [
    { name: "recipes", sourceCommit: "2".repeat(40) },
    { name: "dashboard", sourceCommit: "3".repeat(40) },
    {
      name: "bicep-types-aws",
      repository: aws,
      sourceRef: "main",
      sourceCommit: base
    }
  ];
  const config = {
    signed: true,
    actor,
    committer: "web-flow",
    rules: true,
    build: "success",
    blockedPolls: 0
  };
  const calls = [];
  let pull;
  const result = (data) => ({ data });
  const github = {
    paginate: async () =>
      config.rules ?
        [
          {
            type: "required_status_checks",
            parameters: {
              required_status_checks: [
                { context: "Build and test", integration_id: 15368 }
              ]
            }
          }
        ]
      : [],
    rest: {
      checks: {
        async listForRef({ ref, check_name }) {
          assert.equal(ref, pull.head.sha);
          assert.equal(check_name, "Build and test");
          if (config.blockedPolls-- > 0)
            return result({ total_count: 0, check_runs: [] });
          return result({
            total_count: 1,
            check_runs: [
              {
                app: { id: 15368 },
                name: check_name,
                head_sha: ref,
                status: "completed",
                conclusion: config.build,
                html_url: "https://example.invalid/build"
              }
            ]
          });
        }
      },
      git: {
        async getRef({ ref }) {
          const exists = git(
            "for-each-ref",
            "--format=%(objectname)",
            `refs/${ref}`
          );
          if (!exists)
            throw Object.assign(new Error("not found"), { status: 404 });
          return result({ ref: `refs/${ref}`, object: { sha: exists } });
        },
        async createRef({ ref, sha }) {
          calls.push("ref");
          git("update-ref", ref, sha, "0".repeat(40));
        },
        async createTree({ base_tree, tree }) {
          assert.deepEqual(tree, [
            { path: "radius", type: "commit", mode: "160000", sha: product }
          ]);
          git("read-tree", base_tree);
          git("update-index", "--cacheinfo", `160000,${product},radius`);
          return result({ sha: git("write-tree") });
        },
        async createCommit({ tree, parents, message, ...args }) {
          calls.push("commit");
          assert.deepEqual(args, {
            owner: "radius-project",
            repo: "bicep-types-aws"
          });
          assert.match(message, /Signed-off-by: radius-release\[bot\]/);
          return result({
            sha: git("commit-tree", tree, "-p", parents[0], "-m", message)
          });
        }
      },
      users: { getByUsername: async () => result(actor) },
      repos: {
        getCommit: async ({ ref }) =>
          result({
            sha: ref,
            author: config.actor,
            committer: { login: config.committer },
            parents: [{ sha: git("rev-parse", `${ref}^`) }],
            commit: {
              tree: { sha: git("rev-parse", `${ref}^{tree}`) },
              verification: { verified: config.signed }
            }
          })
      },
      pulls: {
        list: async () => result(pull ? [pull] : []),
        async create({ head, base }) {
          calls.push("pr");
          pull = {
            user: actor,
            state: "open",
            number: 42,
            mergeable_state: "clean",
            head: {
              ref: head,
              sha: git("rev-parse", head),
              repo: { full_name: aws }
            },
            base: { ref: base, repo: { full_name: aws } }
          };
          return result(pull);
        },
        get: async () => result(pull),
        async merge({ sha, merge_method }) {
          assert.equal(sha, pull.head.sha);
          assert.equal(merge_method, "rebase");
          calls.push("merge");
          const tree = git(
            "rev-parse",
            `${config.badMerge ? base : sha}^{tree}`
          );
          const landed = git(
            "commit-tree",
            tree,
            "-p",
            base,
            "-m",
            "rebased by GitHub"
          );
          git("update-ref", `refs/heads/${target}`, landed, base);
          Object.assign(pull, {
            merged: true,
            merged_at: "now",
            merge_commit_sha: landed,
            state: "closed"
          });
          if (config.dropResponse) throw new Error("response lost");
          return result({ merged: true });
        }
      }
    }
  };
  const plan = () =>
    capture(
      {
        channel,
        source: { productCommit: product },
        siblingRepositories: structuredClone(siblings)
      },
      dir
    );
  const approve = async (input = plan()) => {
    const source = input[2];
    verifyIntent(dir, source, product);
    if (!source.radiusPin) return source.sourceCommit;
    return reconcile({
      github,
      base,
      baseTree: git("rev-parse", `${base}^{tree}`),
      product,
      tree: source.radiusPin.expectedTree,
      channel,
      version,
      sourceRef: source.sourceRef,
      log() {},
      wait: async () => {}
    });
  };
  return { plan, approve, config, calls, git, siblings, base };
}

test("first RC corrects the wrong-repository SHA; replay adopts the exact rebase result", async (t) => {
  const f = fixture(t);
  f.config.dropResponse = true;
  f.config.blockedPolls = 2;
  const frozen = f.plan();
  assert.equal(frozen[2].sourceCommit, f.base);
  assert.equal(frozen[2].radiusPin.productCommit, product);
  assert.deepEqual(f.calls, []);
  const landed = await f.approve(frozen);
  assert.equal(landed, f.git("rev-parse", target));
  assert.equal(f.git("rev-parse", `${landed}:radius`), product);
  assert.equal(f.git("rev-parse", `${landed}^`), f.base);
  assert.equal(f.git("diff", "--name-only", f.base, landed), "radius");
  assert.deepEqual(frozen.slice(0, 2), f.siblings.slice(0, 2));
  assert.equal(frozen[2].sourceRef, "main");
  assert.equal(
    frozen[2].radiusPin.expectedTree,
    f.git("rev-parse", `${landed}^{tree}`)
  );
  // The PR head may have been deleted, and the protected branch may have
  // legitimately advanced after merge. Neither changes the approved tree.
  f.git(
    "update-ref",
    "-d",
    `refs/heads/automation/radius-pin-${version}-${product}`
  );
  const next = f.git(
    "commit-tree",
    `${landed}^{tree}`,
    "-p",
    landed,
    "-m",
    "later change"
  );
  f.git("update-ref", `refs/heads/${target}`, next);
  assert.equal(await f.approve(frozen), landed);
  assert.deepEqual(f.calls, ["ref", "commit", "ref", "pr", "merge"]);
});

for (const state of ["match", "legacy", "file", "removed"]) {
  test(`frozen ${state} source cannot trigger an AWS write`, async (t) => {
    const f = fixture(t, state);
    if (state === "file" || state === "removed") assert.throws(f.plan);
    else {
      const frozen = f.plan();
      assert.equal(frozen[2].sourceCommit, f.base);
      if (state === "legacy") assert.equal(frozen[2].radiusPin, undefined);
      else {
        assert.deepEqual(f.calls, []);
        f.git("branch", target, f.base);
        assert.equal(await f.approve(frozen), f.base);
      }
    }
    assert.deepEqual(f.calls, []);
  });
}

for (const [description, options, failure] of [
  ["unsigned proposal", { signed: false }, /not signed by the release App/],
  [
    "wrong author",
    { actor: { login: "someone", type: "User" } },
    /not signed by the release App/
  ],
  [
    "unexpected signer",
    { committer: "contributor" },
    /not signed by the release App/
  ],
  ["skipped required build", { build: "skipped" }, /Build and test skipped/],
  ["failed required build", { build: "failure" }, /Build and test failure/],
  [
    "missing required check",
    { rules: false },
    /Require the GitHub Actions check/
  ],
  ["landed tree differs", { badMerge: true }, /exactly the planned pin/]
]) {
  test(`no tagged AWS source on ${description}`, async (t) => {
    const f = fixture(t);
    Object.assign(f.config, options);
    await assert.rejects(f.approve(), failure);
    assert.equal(f.siblings[2].sourceCommit, f.base);
    if (!options.badMerge) assert.ok(!f.calls.includes("merge"));
    if (options.rules === false) assert.deepEqual(f.calls, []);
  });
}

test("tampered product or expected tree and intervening backports cannot cause a pin write", async (t) => {
  const f = fixture(t);
  const frozen = f.plan();
  for (const key of ["productCommit", "expectedTree"]) {
    const edited = structuredClone(frozen);
    edited[2].radiusPin[key] = "e".repeat(40);
    await assert.rejects(f.approve(edited), /approved AWS pin intent/);
  }
  const later = f.git(
    "commit-tree",
    `${f.base}^{tree}`,
    "-p",
    f.base,
    "-m",
    "backport"
  );
  f.git("branch", target, later);
  await assert.rejects(
    f.approve(frozen),
    /changed since its source was captured/
  );
  assert.deepEqual(f.calls, []);
});
