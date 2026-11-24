// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { join } from "node:path";
import test from "node:test";

import materializeTagPlan, {
  readReceipt,
  writeReceipt
} from "./aws-pin-receipt.mjs";
import { mandatoryChecks } from "./publish-draft-release.mjs";
import { releaseTargets } from "./publish-sibling-releases.mjs";

const version = "v0.63.0-rc.2";
const product = "a".repeat(40),
  base = "b".repeat(40),
  landed = "c".repeat(40);
const expectedTree = "d".repeat(40),
  source = "e".repeat(40),
  other = "f".repeat(40);
const ref = `heads/automation/release-state-aws-0.63.0-rc.2`;
const receiptPath = ".github/release-state/aws-pin.json";
const planPath = `.github/release-plans/${version}.yaml`;
const bot = { login: "radius-release[bot]", type: "Bot", id: 123 };
const radius = { owner: "radius-project", repo: "radius" };
const aws = { owner: "radius-project", repo: "bicep-types-aws" };
const notFound = () => Object.assign(new Error("not found"), { status: 404 });

function file(path, text) {
  const bytes = Buffer.from(text);
  return {
    type: "file",
    path,
    encoding: "base64",
    content: bytes.toString("base64"),
    sha: createHash("sha1")
      .update(`blob ${bytes.length}\0`)
      .update(bytes)
      .digest("hex")
  };
}

function commit(sha, tree, parents, files = []) {
  return {
    sha,
    commit: { tree: { sha: tree } },
    parents: parents.map((sha) => ({ sha })),
    files
  };
}

function fixture() {
  const plan = {
    schemaVersion: 2,
    version,
    channel: "0.63",
    releaseType: "rc",
    previousVersion: "v0.63.0-rc.1",
    source: { productCommit: product },
    siblingRepositories: ["recipes", "dashboard", aws.repo].map((name) => ({
      name,
      repository: `radius-project/${name}`,
      sourceRef: "main",
      sourceCommit: name === aws.repo ? base : other,
      ...(name === aws.repo ?
        { radiusPin: { productCommit: product, expectedTree } }
      : {})
    }))
  };
  const raw = `# Approved cutoff — do not re-render\r\n${JSON.stringify(plan)}\r\n`;
  const planBlob = execFileSync("git", ["hash-object", "--stdin"], {
    input: raw,
    encoding: "utf8"
  }).trim();
  const record = {
    schemaVersion: 1,
    version,
    planBlob,
    baseCommit: base,
    productCommit: product,
    expectedTree,
    landedCommit: landed
  };
  const productTree = "1".repeat(40);
  const f = {
    plan,
    planBlob,
    record,
    raw,
    head: null,
    writes: [],
    stateReads: 0,
    tree: {
      sha: expectedTree,
      truncated: false,
      tree: [{ path: "radius", type: "commit", mode: "160000", sha: product }]
    },
    awsCommits: new Map([
      [base, commit(base, productTree, [other])],
      ...[landed, other].map((sha) => [
        sha,
        commit(
          sha,
          expectedTree,
          [base],
          [{ filename: "radius", status: "modified" }]
        )
      ])
    ]),
    radiusCommits: new Map([
      [product, commit(product, productTree, [])],
      [source, commit(source, other, [product])]
    ]),
    files: new Map([[source, file(planPath, raw)]])
  };
  const trees = new Map();
  const signed = (content, tree) => {
    const sha = (f.radiusCommits.size + 10).toString(16).padStart(40, "0");
    const receipt = file(receiptPath, content);
    const lock = commit(
      sha,
      tree,
      [product],
      [{ status: "added", filename: receiptPath, sha: receipt.sha }]
    );
    Object.assign(lock, {
      author: { ...bot },
      committer: { login: "web-flow" }
    });
    lock.commit.verification = { verified: true };
    f.radiusCommits.set(sha, lock);
    f.files.set(sha, receipt);
    return sha;
  };
  f.install = (values = {}) =>
    (f.head = signed(JSON.stringify({ ...record, ...values }), other));
  f.github = {
    rest: {
      users: {
        getByUsername: async ({ username }) => {
          assert.equal(username, bot.login);
          return { data: bot };
        }
      },
      repos: {
        getCommit: async ({ ref, per_page, ...repository }) => {
          assert.match(ref, /^[a-f0-9]{40}$/); // never an AWS branch tip
          assert.ok(per_page >= 2);
          assert.deepEqual(
            repository,
            repository.repo === aws.repo ? aws : radius
          );
          const data = (
            repository.repo === aws.repo ?
              f.awsCommits
            : f.radiusCommits).get(ref);
          if (!data) throw notFound();
          return { data };
        },
        compareCommitsWithBasehead: async ({ basehead, ...repository }) => {
          assert.deepEqual(repository, { ...aws, per_page: 1 });
          const [from, to] = basehead.split("...");
          const seen = new Set();
          for (const queue = [to]; queue.length;) {
            const sha = queue.shift();
            if (sha === from) return { data: { status: "ahead" } };
            if (seen.has(sha)) continue;
            seen.add(sha);
            queue.push(
              ...(f.awsCommits.get(sha)?.parents ?? []).map((p) => p.sha)
            );
          }
          return { data: { status: "diverged" } };
        },
        getContent: async ({ path, ref, ...repository }) => {
          assert.deepEqual(repository, radius);
          assert.match(ref, /^[a-f0-9]{40}$/); // never reread by mutable state ref
          const data = f.files.get(ref);
          if (data?.path !== path) throw notFound();
          return { data };
        }
      },
      git: {
        getRef: async ({ ref: requested, ...repository }) => {
          assert.deepEqual(repository, radius);
          if (requested === `tags/${version}`)
            return { data: { object: { type: "tag", sha: other } } };
          assert.equal(requested, ref);
          f.stateReads++;
          if (!f.head) throw notFound();
          return {
            data: {
              ref: `refs/${ref}`,
              object: { type: "commit", sha: f.head }
            }
          };
        },
        getTag: async ({ tag_sha, ...repository }) => {
          assert.deepEqual(repository, radius);
          assert.equal(tag_sha, other);
          return { data: { object: { type: "commit", sha: source } } };
        },
        getTree: async ({ tree_sha, ...repository }) => {
          assert.deepEqual(repository, aws);
          assert.equal(tree_sha, expectedTree);
          return { data: f.tree };
        },
        createTree: async ({ base_tree, tree: [entry], ...repository }) => {
          f.writes.push("tree");
          assert.deepEqual(repository, radius);
          assert.equal(base_tree, productTree);
          assert.deepEqual(
            [entry.path, entry.mode, entry.type],
            [receiptPath, "100644", "blob"]
          );
          const sha = file(receiptPath, entry.content).sha;
          trees.set(sha, entry.content);
          return { data: { sha } };
        },
        createCommit: async ({ tree, parents, message, ...repository }) => {
          f.writes.push("commit");
          assert.deepEqual(repository, radius); // no signer/author override
          assert.deepEqual(parents, [product]);
          assert.match(message, /Signed-off-by: radius-release\[bot\]/);
          return { data: { sha: signed(trees.get(tree), tree) } };
        },
        createRef: async ({ ref: requested, sha, request, ...repository }) => {
          f.writes.push("createRef");
          assert.deepEqual(repository, radius);
          assert.equal(requested, `refs/${ref}`);
          assert.ok(f.files.has(sha)); // cannot publish a seed ref
          if (f.head)
            throw Object.assign(new Error("already exists"), { status: 422 });
          f.head = sha;
        }
      }
    }
  };
  f.read = () => readReceipt(f.github, plan, planBlob);
  f.write = (sha = landed) => writeReceipt(f.github, plan, planBlob, sha);
  return f;
}

test("absent is uninitialized; a seed, missing file or API failure is not", async () => {
  const f = fixture();
  assert.equal(await f.read(), null);
  f.head = product;
  await assert.rejects(f.read, /history/);
  f.files.delete(f.install());
  await assert.rejects(f.write, /not found/);
  f.github.rest.git.getRef = () => {
    throw Object.assign(new Error("forbidden"), { status: 403 });
  };
  await assert.rejects(f.read, /forbidden/);
  assert.deepEqual(f.writes, []);
});

test("atomic create adopts an equivalent concurrent commit, never updates a winner", async () => {
  for (const second of [landed, other]) {
    const f = fixture();
    const [a, b] = await Promise.allSettled([f.write(), f.write(second)]);
    assert.equal(a.status, "fulfilled");
    assert.deepEqual(a.value, f.record);
    if (second === landed) assert.deepEqual(b, a);
    else {
      assert.equal(b.status, "rejected");
      assert.match(b.reason.message, /locked at/);
    }
    assert.equal(f.writes.filter((op) => op === "createRef").length, 2);
    assert.deepEqual(await f.read(), f.record);
    const writes = f.writes.length;
    await assert.rejects(() => f.write(other), /locked at/);
    assert.equal(f.writes.length, writes);
  }
});

test("lost ref creation response is recovered only from a matching visible receipt", async () => {
  for (const persisted of [false, true]) {
    const f = fixture();
    const create = f.github.rest.git.createRef;
    f.github.rest.git.createRef = async (args) => {
      if (persisted) await create(args);
      throw new Error("response lost");
    };
    if (persisted) assert.deepEqual(await f.write(), f.record);
    else await assert.rejects(f.write, /response lost/);
    f.github.rest.git.createRef = create;
    const before = f.writes.length;
    assert.deepEqual(await f.write(), f.record);
    if (persisted) assert.equal(f.writes.length, before);
    assert.deepEqual(await f.read(), f.record);
  }
});

test("reject untrusted history before publishing, and when adopting a receipt", async () => {
  for (const corrupt of [
    (c) => (c.author.login = "another-release[bot]"),
    (c) => (c.author.type = "User"),
    (c) => (c.committer.login = "untrusted"),
    (c) => (c.commit.verification.verified = false),
    (c) => (c.parents[0].sha = source),
    (c) => c.parents.push({ sha: other }),
    (c) => c.files.push({ filename: "unrelated" })
  ]) {
    const f = fixture();
    const create = f.github.rest.git.createCommit;
    f.github.rest.git.createCommit = async (args) => {
      const result = await create(args);
      corrupt(f.radiusCommits.get(result.data.sha));
      return result;
    };
    await assert.rejects(f.write, /history|release App/);
    assert.equal(f.head, null);
    corrupt(f.radiusCommits.get(f.install()));
    await assert.rejects(f.read, /history|release App/);
  }
});

test("reject corrupt content and foreign receipt versions, plans, pin intents or landed objects", async () => {
  for (const field of [
    "schemaVersion",
    "version",
    "planBlob",
    "baseCommit",
    "productCommit",
    "expectedTree",
    "landedCommit",
    "extra"
  ]) {
    const f = fixture();
    f.install({ [field]: "invalid" });
    await assert.rejects(f.write, /does not match/);
    assert.deepEqual(f.writes, []);
  }
  const f = fixture();
  f.files.get(f.install()).content = Buffer.from("{}").toString("base64");
  await assert.rejects(f.read, /Invalid Git blob/);
  f.install({ landedCommit: "0".repeat(40) });
  await assert.rejects(f.read, /not found/);
  f.plan.siblingRepositories[2].radiusPin.productCommit = other;
  await assert.rejects(f.read, /intent/);
});

test("verify the immutable AWS tree, gitlink and base ancestry for writes and replays; base is valid only on a no-op", async () => {
  const noop = fixture();
  await assert.rejects(() => noop.write(base), /planned tree/);
  noop.awsCommits.get(base).commit.tree.sha = expectedTree;
  assert.deepEqual(await noop.write(base), {
    ...noop.record,
    landedCommit: base
  });
  // A pin landed above a since-reverted backport still carries the approved tree.
  const backport = fixture();
  backport.awsCommits.get(landed).parents[0].sha = other;
  assert.equal((await backport.write()).landedCommit, landed);
  for (const corrupt of [
    (f) => (f.awsCommits.get(landed).commit.tree.sha = other),
    (f) => (f.awsCommits.get(landed).parents[0].sha = source),
    (f) => (f.tree.truncated = true),
    (f) => (f.tree.tree = []),
    (f) => (f.tree.tree[0].type = "blob"),
    (f) => (f.tree.tree[0].mode = "100644"),
    (f) => (f.tree.tree[0].sha = other)
  ]) {
    const f = fixture();
    corrupt(f);
    await assert.rejects(f.write, /planned tree|gitlink/);
    assert.deepEqual(f.writes, []);
    f.install();
    await assert.rejects(f.read, /planned tree|gitlink/);
  }
});

async function tagReader(t, f) {
  const runnerTemp = await mkdtemp(join(process.cwd(), ".aws-pin-test-"));
  t.after(() => rm(runnerTemp, { recursive: true, force: true }));
  const context = { repo: radius, ref: `refs/tags/${version}`, sha: source };
  const outputs = {};
  const core = { setOutput: (name, value) => (outputs[name] = value) };
  return {
    outputs,
    runnerTemp,
    context,
    run: () =>
      materializeTagPlan({ github: f.github, core, context, runnerTemp })
  };
}

test("tag reader uses the raw blob at the peeled Radius tag and routes only AWS to the landed SHA", async (t) => {
  const f = fixture(),
    reader = await tagReader(t, f);
  await assert.rejects(reader.run, /Missing AWS pin receipt/);
  assert.deepEqual(await readdir(reader.runnerTemp), []);
  f.install();
  await reader.run();
  assert.equal(reader.outputs.sha, landed);
  const effective = JSON.parse(
    await readFile(reader.outputs["plan-file"], "utf8")
  );
  const expected = structuredClone(f.plan);
  Object.assign(expected.siblingRepositories[2], {
    sourceCommit: landed,
    sourceRef: "release/0.63"
  });
  assert.deepEqual(effective, expected);
  const manifest = {
    schemaVersion: 1,
    tag: version,
    sourceSha: source,
    releaseType: "rc",
    channel: f.plan.channel,
    planSha256: createHash("sha256")
      .update(JSON.stringify(effective))
      .digest("hex"),
    checks: Object.fromEntries(
      mandatoryChecks.map((name) => [name, "verified"])
    ),
    observed: { imageLocks: [{ name: "deployment-engine", sourceSha: other }] }
  };
  assert.equal(releaseTargets(effective, manifest, source)[2].sha, landed);
  assert.throws(
    () => releaseTargets(f.plan, manifest, source),
    /exact verified release plan/
  );
  assert.deepEqual(f.files.get(source), file(planPath, f.raw));
  f.files.set(source, file(planPath, f.raw.replaceAll("\r\n", "\n")));
  await assert.rejects(reader.run, /does not match/); // same parsed plan, different Git blob
  assert.deepEqual(f.writes, []);
});

test("tag reader rejects the wrong tag, source, plan or product parent", async (t) => {
  const f = fixture(),
    reader = await tagReader(t, f);
  f.install();
  reader.context.ref = "refs/heads/main";
  await assert.rejects(reader.run, /release tag/);
  reader.context.ref = `refs/tags/${version}`;
  reader.context.sha = product;
  await assert.rejects(reader.run, /no longer identifies/);
  reader.context.sha = source;
  f.radiusCommits.get(source).parents[0].sha = other;
  await assert.rejects(reader.run, /planned product commit/);
  f.files.set(
    source,
    file(planPath, JSON.stringify({ ...f.plan, version: "v0.62.0" }))
  );
  await assert.rejects(reader.run, /version does not match/);
  assert.deepEqual(reader.outputs, {});
});

test("legacy tag plans keep the original plan hash and AWS SHA without looking for a receipt", async (t) => {
  const f = fixture(),
    reader = await tagReader(t, f);
  delete f.plan.siblingRepositories[2].radiusPin;
  f.files.set(source, file(planPath, JSON.stringify(f.plan)));
  await reader.run();
  assert.equal(reader.outputs.sha, base);
  assert.equal(f.stateReads, 0);
  assert.equal(
    JSON.stringify(JSON.parse(await readFile(reader.outputs["plan-file"]))),
    JSON.stringify(f.plan)
  );
});
