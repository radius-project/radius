// @ts-check

import { readFile } from "node:fs/promises";

import {
  findRelease,
  listReleases,
  waitForReleaseState
} from "./release-reconciliation.mjs";

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

/** @param {{github: any} & import("./release-reconciliation.mjs").ReleaseReadOptions} options */
export default async function publishDraftRelease({ github, core, sleep }) {
  const owner = core.getInput("OWNER", { required: true });
  const repo = core.getInput("REPO", { required: true });
  const tag = core.getInput("TAG", { required: true });
  const mode = core.getInput("MODE") || "publish";
  if (mode !== "publish" && mode !== "select-aliases") {
    throw new Error("MODE must be publish or select-aliases");
  }
  let releases = await listReleases(github, owner, repo);
  let aliases = selectAliases(releases, tag);
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
  let release = findRelease(releases, tag);
  if (!release) {
    releases = await waitForReleaseState(
      async () => {
        const observed = await listReleases(github, owner, repo);
        return findRelease(observed, tag) ? observed : undefined;
      },
      { core, sleep, description: `Release ${tag}` }
    );
    release = findRelease(releases, tag);
    aliases = selectAliases(releases, tag);
  }
  const makeLatest = requestedLatest && aliases.latest;
  const releaseID = release.id;
  const expectedBody = (await readFile(notesFile, "utf8")).trimEnd();
  if (release.name !== `Radius ${tag}`) {
    throw new Error(`Release ${tag} has the wrong title`);
  }
  if ((release.body ?? "").trimEnd() !== expectedBody) {
    throw new Error(`Release ${tag} does not use the prepared notes`);
  }
  let publicationError;
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
      publicationError = error;
    }
  }

  release = await waitForReleaseState(
    async () => {
      let observed;
      try {
        const response = await github.rest.repos.getRelease({
          owner,
          repo,
          release_id: releaseID
        });
        observed = response.data;
      } catch (error) {
        if (error.status !== 404) throw error;
        return undefined;
      }
      if (observed.tag_name !== tag) {
        throw new Error(`Release ${releaseID} does not match tag ${tag}`);
      }
      if (observed.draft) return undefined;
      if (observed.prerelease !== prerelease) {
        throw new Error(
          `Published release ${tag} has the wrong classification`
        );
      }
      return observed;
    },
    {
      core,
      sleep,
      cause: publicationError,
      description: `Published release ${tag} (ID ${releaseID})`
    }
  );
  if (makeLatest) {
    await waitForReleaseState(
      async () => {
        try {
          const latest = await github.rest.repos.getLatestRelease({
            owner,
            repo
          });
          return latest.data.tag_name === tag ? latest.data : undefined;
        } catch (error) {
          if (error.status !== 404) throw error;
          return undefined;
        }
      },
      { core, sleep, description: `Latest release ${tag}` }
    );
  }
  core.setOutput("release_url", release.html_url);
}
