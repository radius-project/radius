// @ts-check

import { readFile } from "node:fs/promises";

/** @param {any} github @param {string} owner @param {string} repo @param {string} tag */
async function getRelease(github, owner, repo, tag) {
  const releases = await github.paginate(github.rest.repos.listReleases, {
    owner,
    repo,
    per_page: 100,
  });
  const matches = releases.filter((release) => release.tag_name === tag);
  if (matches.length !== 1) {
    throw new Error(`Expected one release for ${tag}; found ${matches.length}`);
  }
  return matches[0];
}

/** @param {{github: any, core: any}} options */
export default async function publishDraftRelease({ github, core }) {
  const owner = core.getInput("OWNER", { required: true });
  const repo = core.getInput("REPO", { required: true });
  const tag = core.getInput("TAG", { required: true });
  const notesFile = core.getInput("NOTES_FILE", { required: true });
  const prerelease = core.getInput("PRERELEASE", { required: true }) === "true";
  const makeLatest = core.getInput("MAKE_LATEST", { required: true }) === "true";
  if (prerelease && makeLatest) {
    throw new Error("A prerelease cannot be marked latest");
  }

  let release = await getRelease(github, owner, repo, tag);
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
    try {
      await github.rest.repos.updateRelease({
        owner,
        repo,
        release_id: release.id,
        draft: false,
        prerelease,
        make_latest: makeLatest ? "true" : "false",
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