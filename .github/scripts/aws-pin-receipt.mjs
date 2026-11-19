// Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtemp, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { isDeepStrictEqual } from "node:util";

import { tagCommit } from "./coordinate-release.mjs";

const radius = { owner: "radius-project", repo: "radius" };
const aws = { owner: "radius-project", repo: "bicep-types-aws" };
export const releaseBot = "radius-release[bot]";
const receiptPath = ".github/release-state/aws-pin.json";
const stateRef = (version) =>
  `heads/automation/release-state-aws-${version.slice(1)}`;
const versionPattern =
  /^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-rc\.[1-9]\d*)?$/;
const isOid = (value) =>
  typeof value === "string" && /^[a-f0-9]{40}$/.test(value);

function requireThat(condition, message) {
  if (!condition) throw new Error(message);
}

export function signedByReleaseApp(commit) {
  return (
    commit.author?.login === releaseBot &&
    commit.author.type === "Bot" &&
    [releaseBot, "web-flow"].includes(commit.committer?.login) &&
    commit.commit.verification?.verified === true
  );
}

// GitHub signs for the installation when no author, committer or signature override is supplied.
export async function createReleaseAppCommit(
  github,
  repository,
  parent,
  tree,
  subject
) {
  const { data: user } = await github.rest.users.getByUsername({
    username: releaseBot
  });
  requireThat(
    user.login === releaseBot &&
      user.type === "Bot" &&
      Number.isSafeInteger(user.id),
    "Could not resolve the release App for sign-off"
  );
  const { data: candidate } = await github.rest.git.createCommit({
    ...repository,
    tree,
    parents: [parent],
    message: `${subject}\n\nSigned-off-by: ${releaseBot} <${user.id}+${releaseBot}@users.noreply.github.com>`
  });
  requireThat(isOid(candidate.sha), "Invalid release App commit");
  return candidate.sha;
}

function awsEntry(plan) {
  const entries = plan.siblingRepositories?.filter((s) => s.name === aws.repo);
  requireThat(
    plan.schemaVersion === 2 &&
      typeof plan.version === "string" &&
      versionPattern.test(plan.version) &&
      entries?.length === 1 &&
      entries[0].repository === `${aws.owner}/${aws.repo}` &&
      isOid(entries[0].sourceCommit),
    "Expected a schema-v2 release plan with one frozen AWS source"
  );
  return entries[0];
}

function binding(plan, planBlob) {
  const { sourceCommit, radiusPin } = awsEntry(plan);
  requireThat(
    isOid(planBlob) &&
      isOid(radiusPin?.productCommit) &&
      isOid(radiusPin.expectedTree) &&
      radiusPin.productCommit === plan.source?.productCommit &&
      Object.keys(radiusPin).length === 2,
    "Invalid AWS pin intent or committed plan blob"
  );
  return {
    schemaVersion: 1,
    version: plan.version,
    planBlob,
    baseCommit: sourceCommit,
    productCommit: radiusPin.productCommit,
    expectedTree: radiusPin.expectedTree
  };
}

async function commitAt(github, repository, sha) {
  const { data } = await github.rest.repos.getCommit({
    ...repository,
    ref: sha,
    per_page: 2
  });
  requireThat(
    data.sha === sha && isOid(data.commit?.tree?.sha),
    `Invalid commit at ${repository.repo}:${sha}`
  );
  return data;
}

async function fileAt(github, sha, path) {
  const { data } = await github.rest.repos.getContent({
    ...radius,
    ref: sha,
    path
  });
  requireThat(
    data.type === "file" &&
      data.path === path &&
      data.encoding === "base64" &&
      typeof data.content === "string",
    `Expected a committed file at ${sha}:${path}`
  );
  const bytes = Buffer.from(data.content, "base64");
  const blob = createHash("sha1")
    .update(`blob ${bytes.length}\0`)
    .update(bytes)
    .digest("hex");
  requireThat(blob === data.sha, `Invalid Git blob at ${sha}:${path}`);
  return { bytes, blob };
}

const onlyParent = (commit, sha) =>
  commit.parents?.length === 1 && commit.parents[0].sha === sha;
const onlyChange = (commit, path, status) =>
  commit.files?.length === 1 &&
  commit.files[0].filename === path &&
  commit.files[0].status === status;

async function verifyLanded(
  github,
  { landedCommit, baseCommit, productCommit, expectedTree }
) {
  const landed = await commitAt(github, aws, landedCommit);
  requireThat(
    landed.commit.tree.sha === expectedTree &&
      (landedCommit === baseCommit ||
        (onlyParent(landed, baseCommit) &&
          onlyChange(landed, "radius", "modified"))),
    `AWS commit ${landedCommit} is not the planned tree and pin on ${baseCommit}`
  );
  const { data } = await github.rest.git.getTree({
    ...aws,
    tree_sha: expectedTree
  });
  const links = data.tree.filter((entry) => entry.path === "radius");
  requireThat(
    data.sha === expectedTree &&
      data.truncated === false &&
      links.length === 1 &&
      links[0].mode === "160000" &&
      links[0].type === "commit" &&
      links[0].sha === productCommit,
    `AWS tree ${expectedTree} does not pin the Radius gitlink to ${productCommit}`
  );
}

async function recordAt(github, sha, expected) {
  const commit = await commitAt(github, radius, sha);
  requireThat(
    onlyParent(commit, expected.productCommit) &&
      onlyChange(commit, receiptPath, "added"),
    `Unexpected AWS pin receipt history at ${sha}`
  );
  requireThat(
    signedByReleaseApp(commit),
    `AWS pin receipt ${sha} is not a GitHub-verified release App commit`
  );
  const { bytes, blob } = await fileAt(github, sha, receiptPath);
  requireThat(
    commit.files[0].sha === blob,
    `Unexpected AWS pin receipt blob at ${sha}`
  );
  const record = JSON.parse(bytes.toString("utf8"));
  requireThat(
    isOid(record?.landedCommit) &&
      isDeepStrictEqual(record, {
        ...expected,
        landedCommit: record.landedCommit
      }),
    `AWS pin receipt ${sha} does not match the approved plan for ${expected.version}`
  );
  return record;
}

export async function readReceipt(github, plan, planBlob) {
  const expected = binding(plan, planBlob);
  const ref = stateRef(expected.version);
  let head;
  try {
    ({ data: head } = await github.rest.git.getRef({ ...radius, ref }));
  } catch (error) {
    if (error.status === 404) return null;
    throw error;
  }
  // There is no seed/update phase. Once the ref exists, a missing object or
  // receipt is corruption, not permission to initialize the branch.
  requireThat(
    head.ref === `refs/${ref}` &&
      head.object?.type === "commit" &&
      isOid(head.object.sha),
    `Unexpected AWS pin receipt ref for ${plan.version}`
  );
  const record = await recordAt(github, head.object.sha, expected);
  await verifyLanded(github, record);
  return record;
}

function adopt(record, landedCommit) {
  requireThat(
    record.landedCommit === landedCommit,
    `AWS pin receipt is locked at ${record.landedCommit}, not ${landedCommit}`
  );
  return record;
}

export async function writeReceipt(github, plan, planBlob, landedCommit) {
  const expected = binding(plan, planBlob);
  requireThat(isOid(landedCommit), "Expected a full landed AWS commit SHA");
  const existing = await readReceipt(github, plan, planBlob);
  if (existing) return adopt(existing, landedCommit);

  const record = { ...expected, landedCommit };
  await verifyLanded(github, record);
  const parent = await commitAt(github, radius, expected.productCommit);
  const { data: tree } = await github.rest.git.createTree({
    ...radius,
    base_tree: parent.commit.tree.sha,
    tree: [
      {
        path: receiptPath,
        mode: "100644",
        type: "blob",
        content: `${JSON.stringify(record, null, 2)}\n`
      }
    ]
  });
  requireThat(isOid(tree.sha), "Invalid AWS pin receipt tree");
  const candidate = await createReleaseAppCommit(
    github,
    radius,
    expected.productCommit,
    tree.sha,
    `chore(release): record AWS pin for ${plan.version}`
  );
  adopt(await recordAt(github, candidate, expected), landedCommit);

  // Creating (never updating) the protected ref is the compare-and-swap.
  // A concurrent winner may have signed identical contents at a different SHA.
  let createError;
  try {
    await github.rest.git.createRef({
      ...radius,
      ref: `refs/${stateRef(plan.version)}`,
      sha: candidate,
      request: { retries: 0 }
    });
  } catch (error) {
    createError = error;
  }
  const winner = await readReceipt(github, plan, planBlob);
  if (!winner)
    throw (
      createError ??
      new Error(`Missing AWS pin receipt after create for ${plan.version}`)
    );
  return adopt(winner, landedCommit);
}

export default async function materializeTagPlan({
  github,
  context,
  core,
  runnerTemp = process.env.RUNNER_TEMP
}) {
  const version = context.ref?.replace(/^refs\/tags\//, "");
  requireThat(
    context.repo.owner === radius.owner &&
      context.repo.repo === radius.repo &&
      versionPattern.test(version) &&
      context.ref === `refs/tags/${version}` &&
      isOid(context.sha),
    "AWS tag plan requires a Radius release tag"
  );
  const source = await tagCommit(github, context, version);
  requireThat(
    source === context.sha,
    `Radius tag ${version} no longer identifies ${context.sha}`
  );
  const { bytes, blob } = await fileAt(
    github,
    source,
    `.github/release-plans/${version}.yaml`
  );
  const plan = JSON.parse(
    execFileSync("yq", ["-p=yaml", "-o=json", ".", "-"], {
      input: bytes,
      encoding: "utf8"
    })
  );
  requireThat(
    plan.version === version,
    "Committed plan version does not match the Radius tag"
  );
  const entry = awsEntry(plan);
  if (Object.hasOwn(entry, "radiusPin")) {
    requireThat(
      onlyParent(
        await commitAt(github, radius, source),
        plan.source?.productCommit
      ),
      `Radius tag ${version} is not based on its planned product commit`
    );
    const record = await readReceipt(github, plan, blob);
    requireThat(record, `Missing AWS pin receipt for ${version}`);
    entry.sourceCommit = record.landedCommit;
  }

  const directory = await mkdtemp(join(runnerTemp, "aws-pin-"));
  const file = join(directory, "effective-plan.json");
  await writeFile(file, `${JSON.stringify(plan, null, 2)}\n`, {
    flag: "wx",
    mode: 0o600
  });
  core.setOutput("plan-file", file);
  core.setOutput("sha", entry.sourceCommit);
}
