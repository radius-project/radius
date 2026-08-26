// @ts-check

import { readFile } from "node:fs/promises";

/** @param {any} github @param {string} owner @param {string} repo @param {string} tag */
async function findRelease(github, owner, repo, tag) {
  const releases = await github.paginate(github.rest.repos.listReleases, {
    owner,
    repo,
    per_page: 100,
  });
  const matches = releases.filter((release) => release.tag_name === tag);
  if (matches.length > 1) {
    throw new Error(`Multiple releases found for ${tag}`);
  }
  return matches[0];
}

function verifyRelease(release, tag, notes, prerelease) {
  if (!release?.draft) {
    throw new Error(`Release ${tag} is not a draft`);
  }
  if (release.name !== `Radius ${tag}`) {
    throw new Error(`Release ${tag} has the wrong title`);
  }
  if ((release.body ?? "").trimEnd() !== notes) {
    throw new Error(`Release ${tag} does not use the prepared notes`);
  }
  if (release.prerelease !== prerelease) {
    throw new Error(`Release ${tag} has the wrong classification`);
  }
}

/** @param {{github: any, core: any}} options */
export default async function ensureDraftRelease({ github, core }) {
  const owner = core.getInput("OWNER", { required: true });
  const repo = core.getInput("REPO", { required: true });
  const tag = core.getInput("TAG", { required: true });
  const sourceSha = core.getInput("SOURCE_SHA", { required: true });
  const notesFile = core.getInput("NOTES_FILE", { required: true });
  const prerelease = core.getInput("PRERELEASE", { required: true }) === "true";
  const notes = (await readFile(notesFile, "utf8")).trimEnd();
  if (!/^[0-9a-f]{40}$/.test(sourceSha)) {
    throw new Error("SOURCE_SHA must be a full commit SHA");
  }
  let release = await findRelease(github, owner, repo, tag);

  if (!release) {
    try {
      await github.rest.repos.createRelease({
        owner,
        repo,
        tag_name: tag,
        target_commitish: sourceSha,
        name: `Radius ${tag}`,
        body: notes,
        draft: true,
        prerelease,
        make_latest: "false",
      });
    } catch (error) {
      release = await findRelease(github, owner, repo, tag);
      if (!release) {
        throw error;
      }
    }
    release = await findRelease(github, owner, repo, tag);
  }
  verifyRelease(release, tag, notes, prerelease);
  core.setOutput("release_id", String(release.id));
}