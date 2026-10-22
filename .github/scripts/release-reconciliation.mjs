// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// @ts-check

import { setTimeout } from "node:timers/promises";

/** @typedef {{core: any, sleep?: (milliseconds: number) => Promise<unknown>, cause?: unknown}} ReleaseReadOptions */

/**
 * @template T
 * @param {() => Promise<T | undefined>} read
 * @param {ReleaseReadOptions & {description: string}} options
 * @returns {Promise<T>}
 */
export async function waitForReleaseState(
  read,
  { core, description, sleep = setTimeout, cause }
) {
  const attempts = 5;
  for (let attempt = 1; attempt <= attempts; attempt++) {
    const value = await read();
    if (value !== undefined) return value;
    if (attempt < attempts) {
      const delay = 1000 * 2 ** (attempt - 1);
      core.info(
        `${description} is not visible yet; retrying in ${delay}ms (${attempt}/${attempts})`
      );
      await sleep(delay);
    }
  }
  throw new Error(
    `${description} was not visible after ${attempts} attempts; preserve existing release state and resume after verification`,
    { cause }
  );
}

/** @param {any} github @param {string} owner @param {string} repo */
export function listReleases(github, owner, repo) {
  return github.paginate(github.rest.repos.listReleases, {
    owner,
    repo,
    per_page: 100
  });
}

/** @param {any[]} releases @param {string} tag */
export function findRelease(releases, tag) {
  const matches = releases.filter((release) => release.tag_name === tag);
  if (matches.length > 1) {
    throw new Error(`Multiple releases found for ${tag}`);
  }
  return matches[0];
}

/** @param {any} github @param {string} owner @param {string} repo @param {string} tag @param {ReleaseReadOptions} options */
export function waitForRelease(github, owner, repo, tag, options) {
  return waitForReleaseState(
    async () => findRelease(await listReleases(github, owner, repo), tag),
    { ...options, description: `Release ${tag}` }
  );
}
