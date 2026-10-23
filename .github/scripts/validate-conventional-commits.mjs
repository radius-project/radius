#!/usr/bin/env node

// ------------------------------------------------------------
// Copyright 2026 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

const allowedTypes = [
  "build",
  "chore",
  "ci",
  "docs",
  "feat",
  "fix",
  "perf",
  "refactor",
  "revert",
  "style",
  "test"
];
const conventionalSubject = new RegExp(
  `^(?:${allowedTypes.join("|")})(?:\\([^)]+\\))?!?: .+`
);
const conflictHandoffSubject = /^chore\(backport\): hand off #\d+ conflict$/;

export function invalidCommits(commits) {
  if (!Array.isArray(commits)) {
    throw new TypeError("commit input must be a JSON array");
  }

  return commits.filter((entry) => {
    const message = entry?.commit?.message;
    const subject =
      typeof message === "string" ? message.split("\n", 1)[0] : "";
    return (
      !conventionalSubject.test(subject) || conflictHandoffSubject.test(subject)
    );
  });
}

export function validateBackportBase(body, baseSha) {
  const markers = [
    ...(body ?? "").matchAll(/<!-- radius-backport-base: ([0-9a-f]{40}) -->/g)
  ];
  if (markers.length === 0) {
    return;
  }
  if (markers.length !== 1) {
    throw new Error("backport PR must contain exactly one base marker");
  }
  if (markers[0][1] !== baseSha) {
    throw new Error(
      `release branch advanced from ${markers[0][1]} to ${baseSha}`
    );
  }
}

export function validateGeneratedBackport(body, baseSha, headRef, commits) {
  const branch = headRef.match(/^automation\/backport-(\d+)-to-\d+\.\d+$/);
  const sources = [
    ...(body ?? "").matchAll(/<!-- radius-backport-source: #(\d+) -->/g)
  ];
  const sourceCommits = [
    ...(body ?? "").matchAll(/<!-- radius-backport-commit: ([0-9a-f]{40}) -->/g)
  ];
  if (sources.length !== 1 || (branch && sources[0][1] !== branch[1])) {
    throw new Error(
      "generated backport must contain one matching source marker"
    );
  }
  if (sourceCommits.length !== 1) {
    throw new Error("generated backport must contain one source commit marker");
  }
  const bases = [
    ...(body ?? "").matchAll(/<!-- radius-backport-base: ([0-9a-f]{40}) -->/g)
  ];
  if (bases.length !== 1) {
    throw new Error("generated backport must contain one base marker");
  }

  validateBackportBase(body, baseSha);
  if (
    commits.length !== 1 ||
    (commits[0].parents && commits[0].parents.length !== 1)
  ) {
    throw new Error("a backport PR must contain exactly one non-merge commit");
  }
  const trailer = `(cherry picked from commit ${sourceCommits[0][1]})`;
  const trailers = (commits[0]?.commit?.message ?? "")
    .split(/\r?\n/)
    .filter((line) => /^\(cherry picked from commit /.test(line));
  if (trailers.at(-1) !== trailer) {
    throw new Error(`generated backport is missing exact trailer: ${trailer}`);
  }
  return { number: Number(sources[0][1]), sha: sourceCommits[0][1] };
}

export async function validateMergedBackportSource({
  github,
  context,
  commits,
  body,
  baseSha,
  headRef
}) {
  const source = validateGeneratedBackport(body, baseSha, headRef, commits);
  const { data: pull } = await github.rest.pulls.get({
    ...context.repo,
    pull_number: source.number
  });
  if (
    pull.number !== source.number ||
    !pull.merged ||
    !pull.merged_at ||
    pull.base.ref !== "main" ||
    pull.base.repo.full_name !== `${context.repo.owner}/${context.repo.repo}` ||
    pull.merge_commit_sha !== source.sha
  ) {
    throw new Error(
      `Backport source #${source.number} must be the matching PR merged into main`
    );
  }
  const { data: comparison } =
    await github.rest.repos.compareCommitsWithBasehead({
      ...context.repo,
      basehead: `${source.sha}...main`,
      per_page: 1
    });
  if (!["ahead", "identical"].includes(comparison.status)) {
    throw new Error(
      `Backport source #${source.number} is not reachable from main`
    );
  }
}

async function main() {
  const inputPath = process.argv[2];
  if (!inputPath) {
    throw new Error("usage: validate-conventional-commits.mjs <commits.json>");
  }

  const commits = JSON.parse(await readFile(inputPath, "utf8"));
  const bodyPath = process.argv[3];
  const baseSha = process.argv[4];
  const headRef = process.argv[5];
  if (bodyPath || baseSha || headRef) {
    if (!bodyPath || !baseSha || !headRef) {
      throw new Error(
        "body path, base SHA, and head ref must be supplied together"
      );
    }
    const body = await readFile(bodyPath, "utf8");
    validateBackportBase(body, baseSha);
    validateGeneratedBackport(body, baseSha, headRef, commits);
  }
  const invalid = invalidCommits(commits);
  if (invalid.length === 0) {
    console.log(`Validated ${commits.length} Conventional Commit message(s).`);
    return;
  }

  console.error("The following release-branch commits are not conventional:");
  for (const entry of invalid) {
    const subject =
      entry?.commit?.message?.split("\n", 1)[0] ?? "<missing message>";
    console.error(`- ${(entry?.sha ?? "<unknown>").slice(0, 12)} ${subject}`);
  }
  process.exitCode = 1;
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  await main();
}
