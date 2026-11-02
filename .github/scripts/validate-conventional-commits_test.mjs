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

import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  invalidCommits,
  validateBackportBase,
  validateGeneratedBackport,
  validateMergedBackportSource
} from "./validate-conventional-commits.mjs";

const commit = (sha, message) => ({ sha, commit: { message } });

test("accepts the repository Conventional Commit forms", () => {
  const commits = [
    commit("a", "fix: repair release preparation"),
    commit("b", "feat(cli)!: remove a legacy flag\n\nBREAKING CHANGE: removed"),
    commit("c", "chore(backport): resolve #123 conflict"),
    commit("d", "ci(deps): bump actions/checkout")
  ];

  assert.deepEqual(invalidCommits(commits), []);
});

test("rejects invalid and missing subjects", () => {
  const invalid = invalidCommits([
    commit("bad-subject", "Fix release preparation"),
    { sha: "missing-message", commit: {} }
  ]);

  assert.deepEqual(
    invalid.map(({ sha }) => sha),
    ["bad-subject", "missing-message"]
  );
});

test("rejects a non-array payload", () => {
  assert.throws(() => invalidCommits({}), /JSON array/);
});

test("CLI rejects an invalid commit file", () => {
  const directory = mkdtempSync(join(tmpdir(), "conventional-commits-"));
  const input = join(directory, "commits.json");
  const script = fileURLToPath(
    new URL("./validate-conventional-commits.mjs", import.meta.url)
  );
  writeFileSync(input, JSON.stringify([commit("bad", "Not conventional")]));

  const result = spawnSync(process.execPath, [script, input], {
    encoding: "utf8"
  });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /bad Not conventional/);
});

test("CLI accepts a valid commit file", () => {
  const directory = mkdtempSync(join(tmpdir(), "conventional-commits-"));
  const input = join(directory, "commits.json");
  const script = fileURLToPath(
    new URL("./validate-conventional-commits.mjs", import.meta.url)
  );
  writeFileSync(input, JSON.stringify([commit("good", "fix: valid")]));

  const output = execFileSync(process.execPath, [script, input], {
    encoding: "utf8"
  });
  assert.match(output, /Validated 1 Conventional Commit message/);
});

test("rejects an unresolved conflict handoff commit", () => {
  const invalid = invalidCommits([
    commit("handoff", "chore(backport): hand off #123 conflict")
  ]);

  assert.deepEqual(
    invalid.map(({ sha }) => sha),
    ["handoff"]
  );
});

test("accepts a backport pinned to the current release base", () => {
  const base = "a".repeat(40);
  assert.doesNotThrow(() =>
    validateBackportBase(`<!-- radius-backport-base: ${base} -->`, base)
  );
});

test("rejects a backport after the release branch advances", () => {
  const expected = "a".repeat(40);
  const current = "b".repeat(40);
  assert.throws(
    () =>
      validateBackportBase(
        `<!-- radius-backport-base: ${expected} -->`,
        current
      ),
    /release branch advanced/
  );
});

test("rejects duplicate backport base markers", () => {
  const base = "a".repeat(40);
  assert.throws(
    () =>
      validateBackportBase(
        `<!-- radius-backport-base: ${base} -->\n` +
          `<!-- radius-backport-base: ${base} -->`,
        base
      ),
    /exactly one base marker/
  );
});

test("accepts complete generated backport metadata", () => {
  const base = "a".repeat(40);
  const source = "b".repeat(40);
  const body = [
    "<!-- radius-backport-source: #123 -->",
    `<!-- radius-backport-base: ${base} -->`,
    `<!-- radius-backport-commit: ${source} -->`
  ].join("\n");
  const commits = [
    commit("head", `fix: backport\n\n(cherry picked from commit ${source})`)
  ];

  assert.doesNotThrow(() =>
    validateGeneratedBackport(
      body,
      base,
      "automation/backport-123-to-0.60",
      commits
    )
  );
});

test("rejects generated backport with removed markers", () => {
  const base = "a".repeat(40);
  assert.throws(
    () =>
      validateGeneratedBackport(
        "",
        base,
        "automation/backport-123-to-0.60",
        []
      ),
    /source marker/
  );
});

test("rejects generated backport without a base marker", () => {
  const source = "b".repeat(40);
  const body = [
    "<!-- radius-backport-source: #123 -->",
    `<!-- radius-backport-commit: ${source} -->`
  ].join("\n");
  assert.throws(
    () =>
      validateGeneratedBackport(
        body,
        "a".repeat(40),
        "automation/backport-123-to-0.60",
        [
          commit(
            "head",
            `fix: backport\n\n(cherry picked from commit ${source})`
          )
        ]
      ),
    /base marker/
  );
});

test("rejects generated backport without exact source trailer", () => {
  const base = "a".repeat(40);
  const source = "b".repeat(40);
  const body = [
    "<!-- radius-backport-source: #123 -->",
    `<!-- radius-backport-base: ${base} -->`,
    `<!-- radius-backport-commit: ${source} -->`
  ].join("\n");
  assert.throws(
    () =>
      validateGeneratedBackport(body, base, "automation/backport-123-to-0.60", [
        commit(
          "head",
          `fix: malformed\n\ntext (cherry picked from commit ${source})`
        )
      ]),
    /missing exact trailer/
  );
});

function backportFixture() {
  const base = "a".repeat(40);
  const sha = "b".repeat(40);
  const pull = {
    number: 123,
    merged: true,
    merged_at: "2026-10-23T00:00:00Z",
    base: { ref: "main", repo: { full_name: "radius-project/radius" } },
    merge_commit_sha: sha
  };
  const comparison = { status: "ahead" };
  const options = {
    context: { repo: { owner: "radius-project", repo: "radius" } },
    commits: [
      commit("head", `fix: backport\n\n(cherry picked from commit ${sha})`)
    ],
    body: [
      "<!-- radius-backport-source: #123 -->",
      `<!-- radius-backport-base: ${base} -->`,
      `<!-- radius-backport-commit: ${sha} -->`
    ].join("\n"),
    baseSha: base,
    headRef: "manual-backport",
    github: {
      rest: {
        pulls: {
          get: async ({ pull_number }) => {
            assert.equal(pull_number, 123);
            return { data: pull };
          }
        },
        repos: {
          compareCommitsWithBasehead: async ({ basehead, per_page }) => {
            assert.equal(basehead, `${sha}...main`);
            assert.equal(per_page, 1);
            return { data: comparison };
          }
        }
      }
    }
  };
  return { options, pull, comparison };
}

test("accepts a manual backport with a verified merged-main source", async () => {
  const { options } = backportFixture();
  await validateMergedBackportSource(options);
});

test("retains earlier provenance already carried by the merged source", async () => {
  const { options } = backportFixture();
  options.commits[0].commit.message = options.commits[0].commit.message.replace(
    "\n\n",
    `\n\n(cherry picked from commit ${"c".repeat(40)})\n\n`
  );
  await validateMergedBackportSource(options);
});

test("a non-automation branch cannot bypass source validation", async () => {
  const { options } = backportFixture();
  options.body = "";
  await assert.rejects(validateMergedBackportSource(options), /source marker/);
});

test("rejects unmerged, wrong-base, wrong-repository and mismatched sources", async () => {
  for (const change of [
    (pull) => {
      pull.merged = false;
    },
    (pull) => {
      pull.merged_at = null;
    },
    (pull) => {
      pull.base.ref = "release/0.62";
    },
    (pull) => {
      pull.base.repo.full_name = "another/radius";
    },
    (pull) => {
      pull.merge_commit_sha = "c".repeat(40);
    },
    (pull) => {
      pull.number = 456;
    }
  ]) {
    const { options, pull } = backportFixture();
    change(pull);
    await assert.rejects(
      validateMergedBackportSource(options),
      /matching PR merged into main/
    );
  }
});

test("rejects sources that are no longer on main and preserves API failures", async () => {
  for (const status of ["behind", "diverged", "unknown"]) {
    const { options, comparison } = backportFixture();
    comparison.status = status;
    await assert.rejects(
      validateMergedBackportSource(options),
      /not reachable from main/
    );
  }
  const { options } = backportFixture();
  options.github.rest.pulls.get = async () => {
    throw new Error("GitHub unavailable");
  };
  await assert.rejects(
    validateMergedBackportSource(options),
    /GitHub unavailable/
  );
});

test("rejects extra release-only commits and multiple source trailers", async () => {
  const { options } = backportFixture();
  options.commits.push(commit("extra", "fix: release-only change"));
  await assert.rejects(
    validateMergedBackportSource(options),
    /exactly one non-merge commit/
  );
  options.commits.pop();
  options.commits[0].commit.message += `\n(cherry picked from commit ${"c".repeat(40)})`;
  await assert.rejects(
    validateMergedBackportSource(options),
    /missing exact trailer/
  );
});
