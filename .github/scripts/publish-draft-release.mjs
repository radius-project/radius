// @ts-check

import { readFile } from "node:fs/promises";

export const mandatoryChecks = [
  "plan",
  "assets",
  "metadata",
  "images",
  "helm",
  "external",
  "installation"
];

export function validatePublicationManifest(manifest, tag, sourceSha) {
  if (
    manifest.schemaVersion !== 1 ||
    manifest.tag !== tag ||
    !/^[a-f0-9]{40}$/.test(sourceSha) ||
    manifest.sourceSha !== sourceSha ||
    mandatoryChecks.some((name) => manifest.checks?.[name] !== "verified")
  ) {
    throw new Error(
      "Release manifest is incomplete, failed, or bound to another source"
    );
  }
}

async function requirePublicationManifest(core, tag) {
  const file = core.getInput("MANIFEST_FILE", { required: true });
  if (!file) {
    throw new Error(
      "A verified release manifest is required before publication"
    );
  }
  const manifest = JSON.parse(await readFile(file, "utf8"));
  validatePublicationManifest(
    manifest,
    tag,
    core.getInput("SOURCE_SHA", { required: true })
  );
}

/** @param {any} github @param {string} owner @param {string} repo */
async function listReleases(github, owner, repo) {
  return github.paginate(github.rest.repos.listReleases, {
    owner,
    repo,
    per_page: 100
  });
}

function findRelease(releases, tag) {
  const matches = releases.filter((release) => release.tag_name === tag);
  if (matches.length !== 1) {
    throw new Error(`Expected one release for ${tag}; found ${matches.length}`);
  }
  return matches[0];
}

/** @param {any} github @param {string} owner @param {string} repo @param {string} tag */
async function getRelease(github, owner, repo, tag) {
  return findRelease(await listReleases(github, owner, repo), tag);
}

function selectAliases(releases, tag) {
  if (!/^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/.test(tag)) {
    throw new Error("TAG must be a Radius release tag");
  }
  if (tag.includes("-")) {
    return { channel: false, latest: false };
  }
  const channel = tag.slice(0, tag.lastIndexOf(".") + 1);
  const newer = releases.filter(
    (release) =>
      !release.draft &&
      !release.prerelease &&
      /^v\d+\.\d+\.\d+$/.test(release.tag_name) &&
      release.tag_name.localeCompare(tag, "en", { numeric: true }) > 0
  );
  return {
    channel: !newer.some((release) => release.tag_name.startsWith(channel)),
    latest: newer.length === 0
  };
}

/** @param {{github: any, core: any}} options */
export default async function publishDraftRelease({ github, core }) {
  const owner = core.getInput("OWNER", { required: true });
  const repo = core.getInput("REPO", { required: true });
  const tag = core.getInput("TAG", { required: true });
  const mode = core.getInput("MODE") || "publish";
  if (mode !== "publish" && mode !== "select-aliases") {
    throw new Error("MODE must be publish or select-aliases");
  }
  const releases = await listReleases(github, owner, repo);
  const aliases = selectAliases(releases, tag);
  if (mode === "select-aliases") {
    await requirePublicationManifest(core, tag);
    core.setOutput("promote_channel", String(aliases.channel));
    core.setOutput("promote_latest", String(aliases.latest));
    return;
  }
  const notesFile = core.getInput("NOTES_FILE", { required: true });
  const prerelease = core.getInput("PRERELEASE", { required: true }) === "true";
  const requestedLatest =
    core.getInput("MAKE_LATEST", { required: true }) === "true";
  if (prerelease && requestedLatest) {
    throw new Error("A prerelease cannot be marked latest");
  }
  const makeLatest = requestedLatest && aliases.latest;

  let release = findRelease(releases, tag);
  const expectedBody = (await readFile(notesFile, "utf8")).trimEnd();
  if (release.name !== `Radius ${tag}`) {
    throw new Error(`Release ${tag} has the wrong title`);
  }
  if ((release.body ?? "").trimEnd() !== expectedBody) {
    throw new Error(`Release ${tag} does not use the prepared notes`);
  }
  if (!release.draft) {
    if (release.prerelease !== prerelease) {
      throw new Error(`Published release ${tag} has the wrong classification`);
    }
  } else {
    await requirePublicationManifest(core, tag);
    try {
      await github.rest.repos.updateRelease({
        owner,
        repo,
        release_id: release.id,
        draft: false,
        prerelease,
        make_latest: makeLatest ? "true" : "false"
      });
    } catch (error) {
      release = await getRelease(github, owner, repo, tag);
      if (release.draft || release.prerelease !== prerelease) {
        throw error;
      }
    }
  }

  release = await getRelease(github, owner, repo, tag);
  if (release.draft || release.prerelease !== prerelease) {
    throw new Error(`Release ${tag} failed publication verification`);
  }
  if (makeLatest) {
    const latest = await github.rest.repos.getLatestRelease({ owner, repo });
    if (latest.data.tag_name !== tag) {
      throw new Error(`Latest release is ${latest.data.tag_name}, not ${tag}`);
    }
  }
  core.setOutput("release_url", release.html_url);
}
