// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";

import { tagCommit } from "./coordinate-release.mjs";
import {
  selectAliases,
  validatePublicationManifest
} from "./publish-draft-release.mjs";
import {
  findRelease,
  listReleases,
  waitForRelease,
  waitForReleaseState
} from "./release-reconciliation.mjs";

const siblingNames = ["recipes", "dashboard", "bicep-types-aws"];
const fullSha = /^[a-f0-9]{40}$/;

export function releaseTargets(plan, manifest, sourceSha) {
  validatePublicationManifest(manifest, plan.version, sourceSha);
  if (
    plan.schemaVersion !== 2 ||
    !/^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/.test(plan.version) ||
    !["rc", "final", "patch"].includes(plan.releaseType) ||
    (plan.releaseType === "rc") !== plan.version.includes("-") ||
    plan.channel !== plan.version.slice(1).split(".").slice(0, 2).join(".") ||
    manifest.releaseType !== plan.releaseType ||
    manifest.channel !== plan.channel ||
    !/^v\d+\.\d+\.\d+(?:-rc\.?[1-9]\d*)?$/.test(plan.previousVersion) ||
    createHash("sha256").update(JSON.stringify(plan)).digest("hex") !==
      manifest.planSha256 ||
    !Array.isArray(plan.siblingRepositories) ||
    plan.siblingRepositories?.length !== siblingNames.length
  ) {
    throw new Error("Sibling releases require the exact verified release plan");
  }
  const targets = siblingNames.map((name) => {
    const matches = plan.siblingRepositories.filter(
      (entry) => entry.name === name
    );
    if (
      matches.length !== 1 ||
      matches[0].repository !== `radius-project/${name}` ||
      !fullSha.test(matches[0].sourceCommit)
    ) {
      throw new Error(`No approved release source for ${name}`);
    }
    return {
      owner: "radius-project",
      repo: name,
      sha: matches[0].sourceCommit
    };
  });
  const engines = manifest.observed?.imageLocks?.filter(
    (entry) => entry.name === "deployment-engine"
  );
  if (engines?.length !== 1 || !fullSha.test(engines[0].sourceSha)) {
    throw new Error("No verified Deployment Engine source lock");
  }
  return [
    ...targets,
    {
      owner: "azure-octo",
      repo: "deployment-engine",
      sha: engines[0].sourceSha
    }
  ];
}

export function notesBase(plan, tags) {
  if (
    plan.releaseType === "patch" ||
    (plan.releaseType === "rc" && plan.previousVersion.includes("-rc"))
  ) {
    if (!tags.some((tag) => tag.name === plan.previousVersion)) {
      throw new Error(
        `Previous release tag ${plan.previousVersion} is missing`
      );
    }
    return plan.previousVersion;
  }
  return tags
    .map((tag) => tag.name)
    .filter(
      (tag) =>
        /^v\d+\.\d+\.\d+$/.test(tag) &&
        tag.localeCompare(plan.version.split("-")[0], "en", { numeric: true }) <
          0
    )
    .sort((left, right) =>
      right.localeCompare(left, "en", { numeric: true })
    )[0];
}

async function latestRelease(github, repository) {
  try {
    return (await github.rest.repos.getLatestRelease(repository)).data;
  } catch (error) {
    if (error.status !== 404) throw error;
    return undefined;
  }
}

function verifyEntry(release, plan, target) {
  if (
    !release ||
    !Number.isSafeInteger(release.id) ||
    release.id <= 0 ||
    typeof release.draft !== "boolean" ||
    release.tag_name !== plan.version ||
    release.prerelease !== (plan.releaseType === "rc") ||
    typeof (release.body ?? "") !== "string"
  ) {
    throw new Error(
      `${target.owner}/${target.repo} release ${plan.version} conflicts with its approved tag or classification`
    );
  }
}

async function attachDashboardBundle(github, plan, target, release, execute) {
  const repository = { owner: target.owner, repo: target.repo };
  const artifactName = "radius-plugin-release-assets";
  const { data: workflow } = await github.rest.repos.getContent({
    ...repository,
    path: ".github/workflows/build.yaml",
    ref: target.sha
  });
  if (workflow.encoding !== "base64" || typeof workflow.content !== "string") {
    throw new Error("Cannot read the approved Dashboard build workflow");
  }
  // Older release sources predate the bundle producer.
  if (
    !Buffer.from(workflow.content, "base64")
      .toString("utf8")
      .includes(artifactName)
  ) {
    return;
  }
  const archive = `radius-backstage-plugin-${plan.version}.tar.gz`;
  const names = [
    archive,
    `${archive}.sha256`,
    `radius-backstage-plugin-${plan.version}.qualification.json`
  ];
  if (!release.draft) {
    const assets = await github.paginate(github.rest.repos.listReleaseAssets, {
      ...repository,
      release_id: release.id,
      per_page: 100
    });
    if (
      !names.every(
        (name) =>
          assets.filter(
            (asset) =>
              asset.name === name &&
              asset.state === "uploaded" &&
              asset.size > 0
          ).length === 1
      )
    ) {
      throw new Error(
        "Published Dashboard release is missing plugin bundle assets"
      );
    }
    return;
  }
  const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
    ...repository,
    workflow_id: "build.yaml",
    head_sha: target.sha,
    event: "push",
    status: "success",
    per_page: 100
  });
  const run = runs
    .filter(
      (entry) =>
        entry.head_sha === target.sha &&
        entry.head_branch === plan.version &&
        entry.event === "push" &&
        entry.status === "completed" &&
        entry.conclusion === "success" &&
        entry.repository.full_name === `${target.owner}/${target.repo}`
    )
    .sort((left, right) => right.id - left.id)[0];
  if (!run) {
    throw new Error(
      "No successful Dashboard tag build for the approved source; resume after it succeeds"
    );
  }
  const artifacts = await github.paginate(
    github.rest.actions.listWorkflowRunArtifacts,
    {
      ...repository,
      run_id: run.id,
      per_page: 100
    }
  );
  const matches = artifacts.filter(
    (artifact) => artifact.name === artifactName
  );
  if (matches.length !== 1 || matches[0].expired) {
    throw new Error(
      "Dashboard tag build requires one retained radius-plugin-release-assets artifact"
    );
  }
  const directory = await mkdtemp(path.join(os.tmpdir(), "dashboard-release-"));
  try {
    const options = {
      env: { ...process.env, GH_TOKEN: (await github.auth()).token }
    };
    await execute(
      "gh",
      [
        "run",
        "download",
        String(run.id),
        "--repo",
        `${target.owner}/${target.repo}`,
        "--name",
        artifactName,
        "--dir",
        directory
      ],
      options
    );
    if (
      JSON.stringify((await readdir(directory)).sort()) !==
      JSON.stringify([...names].sort())
    ) {
      throw new Error(
        "Dashboard plugin artifact must contain exactly the three release assets"
      );
    }
    const data = await readFile(path.join(directory, archive));
    const checksum = await readFile(
      path.join(directory, `${archive}.sha256`),
      "utf8"
    );
    const receipt = JSON.parse(
      await readFile(path.join(directory, names[2]), "utf8")
    );
    if (
      checksum.trim() !==
        `${createHash("sha256").update(data).digest("hex")}  ${archive}` ||
      receipt.stage !== "installed-external-frontend" ||
      receipt.bundle?.source?.revision !== target.sha ||
      receipt.bundle?.source?.dirty !== false
    ) {
      throw new Error(
        "Dashboard plugin bundle checksum or qualification source is invalid"
      );
    }
    await execute(
      "gh",
      [
        "release",
        "upload",
        plan.version,
        "--repo",
        `${target.owner}/${target.repo}`,
        "--clobber",
        ...names.map((name) => path.join(directory, name))
      ],
      options
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

export async function publishRepositoryRelease({
  github,
  core,
  plan,
  target,
  sleep,
  execute = promisify(execFile)
}) {
  const repository = { owner: target.owner, repo: target.repo };
  const verifyTag = async () => {
    if (
      (await tagCommit(github, { repo: repository }, plan.version)) !==
      target.sha
    ) {
      throw new Error(
        `${target.owner}/${target.repo} tag ${plan.version} does not match its approved source`
      );
    }
  };
  await verifyTag();
  let release = findRelease(
    await listReleases(github, target.owner, target.repo),
    plan.version
  );
  if (!release) {
    const tags = await github.paginate(github.rest.repos.listTags, {
      ...repository,
      per_page: 100
    });
    const previous = notesBase(plan, tags);
    const { data: notes } = await github.rest.repos.generateReleaseNotes({
      ...repository,
      tag_name: plan.version,
      target_commitish: target.sha,
      ...(previous ? { previous_tag_name: previous } : {})
    });
    if (typeof notes.body !== "string" || typeof notes.name !== "string") {
      throw new Error(
        `GitHub did not generate release notes for ${target.repo}`
      );
    }
    try {
      const response = await github.rest.repos.createRelease({
        ...repository,
        tag_name: plan.version,
        target_commitish: target.sha,
        name: notes.name,
        body: notes.body,
        draft: true,
        prerelease: plan.releaseType === "rc",
        make_latest: "false",
        request: { retries: 0 }
      });
      release = response.data;
    } catch (error) {
      release = await waitForRelease(
        github,
        target.owner,
        target.repo,
        plan.version,
        {
          core,
          sleep,
          cause: error
        }
      );
    }
  }
  verifyEntry(release, plan, target);
  if (target.owner === "radius-project" && target.repo === "dashboard") {
    await attachDashboardBundle(github, plan, target, release, execute);
  }
  const releaseID = release.id;
  const latest =
    plan.releaseType === "rc" ?
      undefined
    : await latestRelease(github, repository);
  const releases = await listReleases(github, target.owner, target.repo);
  const makeLatest = selectAliases(
    latest ? [...releases, latest] : releases,
    plan.version
  ).latest;
  let publicationError;
  if (release.draft || (makeLatest && latest?.tag_name !== plan.version)) {
    await verifyTag();
    try {
      await github.rest.repos.updateRelease({
        ...repository,
        release_id: releaseID,
        draft: false,
        prerelease: plan.releaseType === "rc",
        make_latest: makeLatest ? "true" : "false"
      });
    } catch (error) {
      publicationError = error;
    }
  }
  release = await waitForReleaseState(
    async () => {
      let observed;
      try {
        observed = (
          await github.rest.repos.getRelease({
            ...repository,
            release_id: releaseID
          })
        ).data;
      } catch (error) {
        if (error.status !== 404) throw error;
        return undefined;
      }
      verifyEntry(observed, plan, target);
      return observed.draft ? undefined : observed;
    },
    {
      core,
      sleep,
      cause: publicationError,
      description: `${target.owner}/${target.repo} published release ${plan.version}`
    }
  );
  await verifyTag();
  if (makeLatest) {
    await waitForReleaseState(
      async () => {
        const observed = await latestRelease(github, repository);
        return observed?.tag_name === plan.version ? observed : undefined;
      },
      {
        core,
        sleep,
        description: `${target.repo} latest release ${plan.version}`
      }
    );
  }
  return {
    name: `${target.owner}/${target.repo}`,
    state: "success",
    url: release.html_url
  };
}

export default async function publishSiblingReleases({
  github,
  siblings,
  engine,
  context,
  core,
  plan,
  manifest,
  sleep
}) {
  plan ??= JSON.parse(
    await readFile(core.getInput("PLAN_FILE", { required: true }), "utf8")
  );
  manifest ??= JSON.parse(
    await readFile(core.getInput("MANIFEST_FILE", { required: true }), "utf8")
  );
  const targets = releaseTargets(plan, manifest, context.sha);
  if (
    context.repo.owner !== "radius-project" ||
    context.repo.repo !== "radius" ||
    context.ref !== `refs/tags/${plan.version}` ||
    (await tagCommit(github, context, plan.version)) !== context.sha
  ) {
    throw new Error(
      "Sibling releases require the published Radius tag and source"
    );
  }
  const { data: radius } = await github.rest.repos.getReleaseByTag({
    ...context.repo,
    tag: plan.version
  });
  if (
    radius.draft !== false ||
    radius.tag_name !== plan.version ||
    radius.prerelease !== (plan.releaseType === "rc")
  ) {
    throw new Error("Radius must be published before sibling releases");
  }
  const results = [];
  try {
    for (const target of targets) {
      results.push(
        await publishRepositoryRelease({
          github: target.owner === "azure-octo" ? engine : siblings,
          core,
          plan,
          target,
          sleep
        })
      );
    }
  } finally {
    core.setOutput("releases", JSON.stringify(results));
  }
}
