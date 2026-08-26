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
  validateGeneratedBackport
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
