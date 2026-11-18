// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const owner = "radius-project";
const repo = "bicep-types-aws";
const repository = `${owner}/${repo}`;
const bot = "radius-release[bot]";
const actionsApp = 15368;
const buildCheck = "Build and test";
const sha = /^[a-f0-9]{40}$/;
const number = "(?:0|[1-9][0-9]*)";
const verifier = fileURLToPath(
  new URL("./verify-bicep-types-aws-radius-pin.sh", import.meta.url)
);

function assert(ok, message) {
  if (!ok) throw new Error(message);
}

function git(directory, ...args) {
  return execFileSync("git", ["-C", directory, ...args], {
    encoding: "utf8"
  }).trim();
}

function fetchSource(directory, remote, ref, commit) {
  git(directory, "fetch", "--quiet", "--no-tags", remote, `refs/heads/${ref}`);
  git(directory, "merge-base", "--is-ancestor", commit, "FETCH_HEAD");
}

// Derive the entire intended tree independently of the write API. No sibling
// code, hooks, filters, or submodules are checked out or executed.
export function expectedTree(directory, base, product) {
  git(directory, "read-tree", base);
  git(directory, "update-index", "--cacheinfo", `160000,${product},radius`);
  return git(directory, "write-tree");
}

async function getRef(github, branch) {
  try {
    const { data } = await github.rest.git.getRef({
      owner,
      repo,
      ref: `heads/${branch}`
    });
    assert(
      data.ref === `refs/heads/${branch}` && sha.test(data.object.sha),
      `Unexpected ref returned for ${branch}`
    );
    return data.object.sha;
  } catch (error) {
    if (error.status !== 404) throw error;
    return null;
  }
}

// Never update or force an existing ref. Recover a lost create response only
// when the two writers agree about what the branch was created for.
async function createRef(github, branch, commit) {
  try {
    await github.rest.git.createRef({
      owner,
      repo,
      ref: `refs/heads/${branch}`,
      sha: commit
    });
  } catch (error) {
    if ((await getRef(github, branch)) !== commit) throw error;
  }
}

async function commitAt(github, ref) {
  return (await github.rest.repos.getCommit({ owner, repo, ref })).data;
}

function verifyContent(commit, base, tree) {
  assert(
    commit.parents.length === 1 &&
      commit.parents[0].sha === base &&
      commit.commit.tree.sha === tree,
    `AWS commit ${commit.sha} does not contain exactly the planned pin on ${base}`
  );
}

function verifyProposal(commit, base, tree) {
  verifyContent(commit, base, tree);
  assert(
    commit.author?.login === bot &&
      commit.author.type === "Bot" &&
      [bot, "web-flow"].includes(commit.committer?.login) &&
      commit.commit.verification?.verified === true,
    `AWS pin proposal ${commit.sha} is not signed by the release App`
  );
}

async function signedProposal(
  github,
  branch,
  base,
  baseTree,
  product,
  tree,
  version
) {
  let head = await getRef(github, branch);
  if (!head) {
    const { data: replacement } = await github.rest.git.createTree({
      owner,
      repo,
      base_tree: baseTree,
      tree: [{ path: "radius", mode: "160000", type: "commit", sha: product }]
    });
    assert(replacement.sha === tree, "GitHub returned an unexpected AWS tree");
    const { data: user } = await github.rest.users.getByUsername({
      username: bot
    });
    assert(
      user.type === "Bot" &&
        user.login === bot &&
        Number.isSafeInteger(user.id),
      "Could not resolve the release App identity"
    );
    const signoff = `${bot} <${user.id}+${bot}@users.noreply.github.com>`;
    // Let GitHub sign the installation's commit. Supplying an author,
    // committer or signature here changes those verification semantics.
    const { data } = await github.rest.git.createCommit({
      owner,
      repo,
      tree,
      parents: [base],
      message: `chore(release): pin Radius for ${version}\n\nSigned-off-by: ${signoff}`
    });
    head = data.sha;
    assert(sha.test(head), "Invalid AWS pin commit");
  }
  verifyProposal(await commitAt(github, head), base, tree);
  if (!(await getRef(github, branch))) await createRef(github, branch, head);
  assert(
    (await getRef(github, branch)) === head,
    `Pin proposal ${branch} changed`
  );
  return head;
}

async function findPull(github, branch, target) {
  const { data } = await github.rest.pulls.list({
    owner,
    repo,
    head: `${owner}:${branch}`,
    base: target,
    state: "all",
    per_page: 2
  });
  assert(data.length < 2, `Ambiguous AWS pin PR for ${branch}`);
  return data[0];
}

async function ensurePull(github, branch, target, head, product, version) {
  let pull = await findPull(github, branch, target);
  if (!pull) {
    try {
      ({ data: pull } = await github.rest.pulls.create({
        owner,
        repo,
        head: branch,
        base: target,
        title: `chore(release): pin Radius for ${version}`,
        body: `Prepare Release selected Radius \`${product}\` for \`${version}\`. Only the \`radius\` submodule changes.`
      }));
    } catch (error) {
      pull = await findPull(github, branch, target);
      if (!pull) throw error;
    }
  }
  assert(
    pull.user.login === bot &&
      pull.head.repo?.full_name === repository &&
      pull.head.ref === branch &&
      pull.head.sha === head &&
      pull.base.repo.full_name === repository &&
      pull.base.ref === target,
    `Unexpected owner, head or base for AWS pin PR #${pull.number}`
  );
  return pull.number;
}

export async function requireCheck(github, targetRepo, branch, name) {
  const rules = await github.paginate(
    "GET /repos/{owner}/{repo}/rules/branches/{branch}",
    {
      owner,
      repo: targetRepo,
      branch,
      per_page: 100
    }
  );
  assert(
    rules.some(
      (rule) =>
        rule.type === "required_status_checks" &&
        rule.parameters.required_status_checks.some(
          (check) =>
            check.context === name && check.integration_id === actionsApp
        )
    ),
    `Require the GitHub Actions check "${name}" on ${owner}/${targetRepo}:${branch} before release preparation`
  );
}

async function buildPassed(github, head) {
  const { data } = await github.rest.checks.listForRef({
    owner,
    repo,
    ref: head,
    check_name: buildCheck,
    filter: "latest",
    per_page: 100
  });
  assert(data.total_count <= 100, `Ambiguous AWS build results for ${head}`);
  const runs = data.check_runs.filter(
    (run) =>
      run.app?.id === actionsApp &&
      run.name === buildCheck &&
      run.head_sha === head
  );
  for (const run of runs) {
    if (run.status === "completed") {
      assert(
        run.conclusion === "success",
        `AWS ${buildCheck} ${run.conclusion}: ${run.html_url}`
      );
    }
  }
  return runs.length > 0 && runs.every((run) => run.conclusion === "success");
}

async function land(github, pr, head, target, base, tree, wait) {
  const args = { owner, repo, pull_number: pr };
  for (let attempt = 0; attempt < 75; attempt++) {
    const { data: pull } = await github.rest.pulls.get(args);
    assert(
      pull.head.sha === head && pull.base.ref === target,
      `AWS pin PR #${pr} changed`
    );
    if (pull.merged) {
      assert(
        await buildPassed(github, head),
        `Missing successful AWS build for merged pin PR #${pr}`
      );
      assert(
        sha.test(pull.merge_commit_sha),
        `Missing landed SHA on AWS pin PR #${pr}`
      );
      // GitHub rebase-merges can rewrite the signature and SHA. Verify the
      // actual landed parent and entire tree against the verified App proposal.
      verifyContent(await commitAt(github, pull.merge_commit_sha), base, tree);
      return pull.merge_commit_sha;
    }
    assert(
      pull.state === "open",
      `AWS pin PR #${pr} was closed without merging`
    );
    assert(
      (await getRef(github, target)) === base,
      `AWS ${target} advanced during preparation`
    );
    assert(pull.mergeable_state !== "dirty", `AWS pin PR #${pr} conflicts`);
    if ((await buildPassed(github, head)) && pull.mergeable_state === "clean") {
      try {
        const { data } = await github.rest.pulls.merge({
          ...args,
          sha: head,
          merge_method: "rebase"
        });
        assert(data.merged, `AWS pin PR #${pr} did not merge`);
      } catch (error) {
        if (!(await github.rest.pulls.get(args)).data.merged) throw error;
      }
      continue;
    }
    await wait(20_000);
  }
  throw new Error(
    `Timed out waiting for required checks or reviews: https://github.com/${repository}/pull/${pr}`
  );
}

export async function reconcile({
  github,
  base,
  baseTree,
  product,
  tree,
  channel,
  version,
  sourceRef,
  log,
  wait = sleep
}) {
  const target = `release/${channel}`;
  const branch = `automation/radius-pin-${version}-${product}`;
  await requireCheck(github, repo, target, buildCheck);
  let tip = await getRef(github, target);
  if (!tip) {
    assert(
      sourceRef === "main",
      `Captured AWS branch ${sourceRef} disappeared`
    );
    await createRef(github, target, base);
    tip = await getRef(github, target);
  }
  if (tip !== base) {
    // Only an already merged proposal can explain a newer target on replay.
    const prior = await findPull(github, branch, target);
    assert(
      prior?.merged_at &&
        prior.head.repo?.full_name === repository &&
        prior.user.login === bot,
      `AWS ${target} changed since its source was captured`
    );
    verifyProposal(await commitAt(github, prior.head.sha), base, tree);
    return land(github, prior.number, prior.head.sha, target, base, tree, wait);
  }
  const head = await signedProposal(
    github,
    branch,
    base,
    baseTree,
    product,
    tree,
    version
  );
  const pr = await ensurePull(github, branch, target, head, product, version);
  log(`AWS pin proposal: https://github.com/${repository}/pull/${pr}`);
  return land(github, pr, head, target, base, tree, wait);
}

export async function resolvePin({
  siblings,
  product,
  channel,
  version,
  remote = `https://github.com/${repository}.git`,
  writePin
}) {
  assert(
    new RegExp(`^${number}\\.${number}$`).test(channel) &&
      new RegExp(
        `^v${number}\\.${number}\\.${number}(?:-rc\\.[1-9][0-9]*)?$`
      ).test(version) &&
      version.startsWith(`v${channel}.`) &&
      sha.test(product),
    "Invalid Radius release identity"
  );
  const matches = siblings.filter((item) => item.name === repo);
  const source = matches[0];
  assert(
    matches.length === 1 &&
      source.repository === repository &&
      ["main", `release/${channel}`].includes(source.sourceRef) &&
      sha.test(source.sourceCommit),
    "Invalid captured AWS source"
  );
  const directory = mkdtempSync(join(tmpdir(), "radius-aws-pin-"));
  try {
    git(directory, "init", "--quiet");
    let { sourceCommit: base, sourceRef } = source;
    fetchSource(directory, remote, sourceRef, base);
    const entry = git(directory, "ls-tree", base, "--", "radius");
    const link = /^160000 commit ([a-f0-9]{40})\tradius$/.exec(entry);
    if (link && link[1] !== product) {
      const tree = expectedTree(directory, base, product);
      base = await writePin({
        base,
        baseTree: git(directory, "rev-parse", `${base}^{tree}`),
        product,
        tree,
        channel,
        version,
        sourceRef
      });
      assert(sha.test(base), "Invalid landed AWS commit");
      sourceRef = `release/${channel}`;
      fetchSource(directory, remote, sourceRef, base);
    }
    // Same independent check as PR validation and the release controller,
    // including the historical never-existed vs removed-submodule distinction.
    execFileSync("bash", [verifier, directory, base, product], {
      stdio: "inherit"
    });
    return siblings.map((item) =>
      item === source ? { ...source, sourceRef, sourceCommit: base } : item
    );
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

export default async function prepare({ github, core, context }) {
  assert(
    context.repo.owner === owner &&
      context.repo.repo === "radius" &&
      context.ref === "refs/heads/main" &&
      context.eventName === "workflow_dispatch" &&
      core.getInput("APP_SLUG") === "radius-release",
    "Unexpected pin workflow identity"
  );
  const file = core.getInput("SIBLINGS_FILE", { required: true });
  const result = await resolvePin({
    siblings: JSON.parse(readFileSync(file, "utf8")),
    channel: core.getInput("CHANNEL"),
    version: core.getInput("VERSION"),
    product: core.getInput("PRODUCT_COMMIT"),
    writePin: (inputs) => reconcile({ ...inputs, github, log: core.info })
  });
  writeFileSync(file, `${JSON.stringify(result, null, 2)}\n`);
}
