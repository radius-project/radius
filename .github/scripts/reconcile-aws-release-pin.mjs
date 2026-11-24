// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { setTimeout as sleep } from "node:timers/promises";
import { awsSource, git, requireCheck, verifyIntent } from "./aws-pin-plan.mjs";
import {
  createReleaseAppCommit,
  readReceipt,
  releaseBot,
  signedByReleaseApp,
  writeReceipt
} from "./aws-pin-receipt.mjs";
import { verifyFinalInputs } from "./verify-aws-final-inputs.mjs";

const owner = "radius-project";
const repo = "bicep-types-aws";
const repository = `${owner}/${repo}`;
const actionsApp = 15368;
const buildCheck = "Build and test";
const sha = /^[a-f0-9]{40}$/;

function assert(ok, message) {
  if (!ok) throw new Error(message);
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

function verifyContent(commit, parent, tree) {
  assert(
    commit.parents.length === 1 &&
      commit.parents[0].sha === parent &&
      commit.commit.tree.sha === tree,
    `AWS commit ${commit.sha} does not contain exactly the planned pin on ${parent}`
  );
}

function verifyProposal(commit, parent, tree) {
  verifyContent(commit, parent, tree);
  assert(
    signedByReleaseApp(commit),
    `AWS pin proposal ${commit.sha} is not signed by the release App`
  );
}

async function signedProposal(
  github,
  branch,
  parent,
  baseTree,
  product,
  tree,
  version
) {
  const existing = await getRef(github, branch);
  if (existing) {
    verifyProposal(await commitAt(github, existing), parent, tree);
    return existing;
  }
  const { data: replacement } = await github.rest.git.createTree({
    owner,
    repo,
    base_tree: baseTree,
    tree: [{ path: "radius", mode: "160000", type: "commit", sha: product }]
  });
  assert(replacement.sha === tree, "GitHub returned an unexpected AWS tree");
  const head = await createReleaseAppCommit(
    github,
    { owner, repo },
    parent,
    tree,
    `chore(release): pin Radius for ${version}`
  );
  verifyProposal(await commitAt(github, head), parent, tree);
  await createRef(github, branch, head);
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
    pull.user.login === releaseBot &&
      pull.head.repo?.full_name === repository &&
      pull.head.ref === branch &&
      pull.head.sha === head &&
      pull.base.repo.full_name === repository &&
      pull.base.ref === target,
    `Unexpected owner, head or base for AWS pin PR #${pull.number}`
  );
  return pull.number;
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

async function land(github, pr, head, target, parent, tree, wait) {
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
      verifyContent(await commitAt(github, pull.merge_commit_sha), parent, tree);
      return pull.merge_commit_sha;
    }
    assert(
      pull.state === "open",
      `AWS pin PR #${pr} was closed without merging`
    );
    assert(
      (await getRef(github, target)) === parent,
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

// Read-only. The release branch may only hold the approved content: the
// captured tree, or that tree with the planned pin (which is then reused).
async function pinState(github, target, base, baseTree, tree, sourceRef) {
  const tip = await getRef(github, target);
  assert(
    tip || sourceRef === "main",
    `Captured AWS branch ${sourceRef} disappeared`
  );
  if (baseTree === tree) return { tip, landed: base };
  if (!tip) return { tip, parent: base };
  const { data } = await github.rest.repos.compareCommitsWithBasehead({
    owner,
    repo,
    basehead: `${base}...${tip}`
  });
  assert(
    ["ahead", "identical"].includes(data.status) &&
      data.commits.length === data.total_commits,
    `AWS ${target} no longer descends from ${base}`
  );
  const landed = data.commits.find((c) => c.commit.tree.sha === tree);
  if (landed) return { tip, landed: landed.sha };
  const tipTree =
    tip === base ? baseTree : (
      data.commits.find((c) => c.sha === tip)?.commit.tree.sha
    );
  assert(
    tipTree === baseTree,
    `AWS ${target} has changes outside the approved plan; revert them through a reviewed pull request, then resume the release`
  );
  return { tip, parent: tip };
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
  await requireCheck(github, repo, target, buildCheck);
  const { tip, landed, parent } = await pinState(
    github,
    target,
    base,
    baseTree,
    tree,
    sourceRef
  );
  if (!tip) await createRef(github, target, base);
  if (landed) return landed;
  // A new parent (after a reviewed revert) gets a new proposal branch.
  const branch = `automation/radius-pin-${version}-${product.slice(0, 12)}-${parent.slice(0, 12)}`;
  const head = await signedProposal(
    github,
    branch,
    parent,
    baseTree,
    product,
    tree,
    version
  );
  const pr = await ensurePull(github, branch, target, head, product, version);
  log(`AWS pin proposal: https://github.com/${repository}/pull/${pr}`);
  return land(github, pr, head, target, parent, tree, wait);
}

function parsePlan(raw) {
  return JSON.parse(
    execFileSync("yq", ["-o=json", ".", "-"], { input: raw, encoding: "utf8" })
  );
}

function blob(raw) {
  return execFileSync("git", ["hash-object", "--stdin"], {
    input: raw,
    encoding: "utf8"
  }).trim();
}

async function finalParity(github, plan, checkout) {
  const rc = plan.previousVersion;
  assert(
    /^v\d+\.\d+\.0-rc\.[1-9]\d*$/.test(rc) && rc.split("-")[0] === plan.version,
    "Final release does not reference its last validated RC"
  );
  const path = `.github/release-plans/${rc}.yaml`;
  const raw = execFileSync("git", ["show", `refs/tags/${rc}:${path}`]);
  const previous = parsePlan(raw);
  const prior = awsSource(previous);
  const lock =
    prior.radiusPin && (await readReceipt(github, previous, blob(raw)));
  assert(
    !prior.radiusPin || lock,
    "Last RC is missing its approved AWS pin receipt"
  );
  const landed = lock ? lock.landedCommit : prior.sourceCommit;
  assert(
    git(checkout, "rev-parse", `refs/tags/${rc}^{commit}`) === landed,
    "The AWS RC tag no longer identifies its validated inputs"
  );
  verifyFinalInputs({
    radius: ".",
    aws: checkout,
    rcPlan: previous,
    version: rc,
    product: plan.source.productCommit,
    landed,
    base: awsSource(plan).sourceCommit
  });
}

export default async function controller({ github, core, context }) {
  const mode = core.getInput("MODE", { required: true });
  assert(
    context.repo.owner === owner && context.repo.repo === "radius",
    "Unexpected release repository"
  );
  const raw = readFileSync(core.getInput("PLAN_FILE", { required: true }));
  const plan = parsePlan(raw),
    planBlob = blob(raw);
  const aws = awsSource(plan),
    base = aws.sourceCommit,
    product = plan.source.productCommit;
  const checkout = core.getInput("AWS_CHECKOUT");
  if (!Object.hasOwn(aws, "radiusPin")) {
    assert(mode === "inspect", "Legacy plans cannot create an AWS pin record");
    core.setOutput("commit", base);
    return;
  }
  assert(
    /^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-rc\.[1-9]\d*)?$/.test(
      plan.version
    ) && plan.version.startsWith(`v${plan.channel}.`),
    "Invalid release version"
  );
  const branch = `automation/release-state-aws-${plan.version.slice(1)}`;
  if (mode === "inspect") {
    verifyIntent(checkout, aws, product);
    const rules = await github.paginate(
      "GET /repos/{owner}/{repo}/rules/branches/{branch}",
      { owner, repo: "radius", branch, per_page: 100 }
    );
    assert(
      ["update", "deletion"].every((type) =>
        rules.some((rule) => rule.type === type)
      ),
      `Protect ${branch} against updates and deletion before releasing`
    );
    await requireCheck(github, repo, `release/${plan.channel}`, buildCheck);
    const record = await readReceipt(github, plan, planBlob);
    if (!record) {
      try {
        await github.rest.git.getRef({
          owner,
          repo,
          ref: `tags/${plan.version}`
        });
        throw new Error(
          "AWS tag exists without its approved pin receipt; refusing to adopt it"
        );
      } catch (error) {
        if (error.status !== 404) throw error;
      }
      // Catch unplanned AWS release-branch changes before approval.
      await pinState(
        github,
        `release/${plan.channel}`,
        base,
        git(checkout, "rev-parse", `${base}^{tree}`),
        aws.radiusPin.expectedTree,
        aws.sourceRef
      );
    }
    if (plan.releaseType === "final") await finalParity(github, plan, checkout);
    // No releasable SHA until the approved pin is locked. The base is only a preflight input.
    core.setOutput("commit", record?.landedCommit ?? "");
    await core.summary
      .addRaw(
        `AWS ${plan.version}: base \`${base}\`, Radius product \`${product}\`, expected tree \`${aws.radiusPin.expectedTree}\`, receipt \`${branch}\`.`
      )
      .write();
    return;
  }
  assert(
    core.getInput("APP_SLUG") === "radius-release",
    "Unexpected release App identity"
  );
  if (mode === "pin") {
    verifyIntent(checkout, aws, product);
    const commit = await reconcile({
      github,
      base,
      baseTree: git(checkout, "rev-parse", `${base}^{tree}`),
      product,
      tree: aws.radiusPin.expectedTree,
      channel: plan.channel,
      version: plan.version,
      sourceRef: aws.sourceRef,
      log: core.info
    });
    core.setOutput("commit", commit);
  } else if (mode === "lock") {
    const record = await writeReceipt(
      github,
      plan,
      planBlob,
      core.getInput("LANDED_COMMIT", { required: true })
    );
    core.setOutput("commit", record.landedCommit);
  } else throw new Error(`Unknown AWS pin mode: ${mode}`);
}
