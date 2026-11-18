// @ts-check

import { readFile } from "node:fs/promises";

import {
  findRelease,
  listReleases,
  waitForRelease
} from "./release-reconciliation.mjs";

function verifyRelease(release, tag, notes, prerelease) {
  if (!release) {
    throw new Error(`Release ${tag} was not returned by GitHub`);
  }
  if (release.tag_name !== tag) {
    throw new Error(`Release ${release.id} does not match tag ${tag}`);
  }
  if (!release.draft) {
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

/** @param {{github: any} & import("./release-reconciliation.mjs").ReleaseReadOptions} options */
export default async function ensureDraftRelease({ github, core, sleep }) {
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
  let release = findRelease(await listReleases(github, owner, repo), tag);

  if (!release) {
    try {
      const response = await github.rest.repos.createRelease({
        owner,
        repo,
        tag_name: tag,
        target_commitish: sourceSha,
        name: `Radius ${tag}`,
        body: notes,
        draft: true,
        prerelease,
        make_latest: "false",
        request: { retries: 0 }
      });
      release = response.data;
    } catch (error) {
      release = await waitForRelease(github, owner, repo, tag, {
        core,
        sleep,
        cause: error
      });
    }
  }
  verifyRelease(release, tag, notes, prerelease);
  core.setOutput("release_id", String(release.id));
}
