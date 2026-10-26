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
import { execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  entriesForMergedPull,
  findIncludedSourceCommits,
  orderByMainHistory,
  releaseChannelForEvent,
  selectReleaseBackports,
  selectNextBackport as select
} from "./select-release-backports.mjs";

const source = (number) => ({
  number,
  merge_commit_sha: String(number).padStart(40, "a"),
  title: `fix: source ${number}`,
  html_url: `https://example.test/pull/${number}`,
  head: { ref: `feature-${number}` },
  labels: [{ name: "backport release/0.60" }]
});

const selectNextBackport = (options) =>
  select({
    mainHistory: [source(101).merge_commit_sha, source(102).merge_commit_sha],
    ...options
  });

test("orders backports by main ancestry, not PR number, date, or input order", () => {
  const earlier = { ...source(102), merged_at: "2026-08-24T00:00:00Z" };
  const later = { ...source(101), merged_at: earlier.merged_at };
  const history = [earlier.merge_commit_sha, later.merge_commit_sha];
  for (const sources of [
    [earlier, later],
    [later, earlier]
  ]) {
    assert.deepEqual(orderByMainHistory(sources, history), [earlier, later]);
    assert.equal(
      selectNextBackport({
        channel: "0.60",
        sources,
        mainHistory: history,
        openBackports: [],
        historicalBackports: []
      })[0].source_pr,
      102
    );
  }
});

test("rejects sources absent from the pinned main history", () => {
  assert.throws(
    () => orderByMainHistory([source(101)], [source(102).merge_commit_sha]),
    /not in the checked-out main merge history/
  );
});

test("serializes two backports through successive release pushes", () => {
  const first = source(101);
  const second = source(102);
  const initial = selectNextBackport({
    channel: "0.60",
    sources: [second, first],
    openBackports: [],
    historicalBackports: []
  });
  assert.equal(initial[0].source_pr, 101);

  const deferred = selectNextBackport({
    channel: "0.60",
    sources: [first, second],
    openBackports: [{ head: { ref: "automation/backport-101-to-0.60" } }],
    historicalBackports: []
  });
  assert.deepEqual(deferred, []);

  const completedFirst = {
    merged_at: "2026-08-24T00:00:00Z",
    body: "<!-- radius-backport-source: #101 -->",
    commits: [
      {
        commit: {
          message: `fix: source 101\n\n(cherry picked from commit ${first.merge_commit_sha})`
        }
      }
    ]
  };
  const next = selectNextBackport({
    channel: "0.60",
    sources: [first, second],
    openBackports: [],
    historicalBackports: [completedFirst]
  });
  assert.equal(next[0].source_pr, 102);
});

test("does not accept a merged marker without the exact trailer", () => {
  const first = source(101);
  const selected = selectNextBackport({
    channel: "0.60",
    sources: [first],
    openBackports: [],
    historicalBackports: [
      {
        merged_at: "2026-08-24T00:00:00Z",
        body: "<!-- radius-backport-source: #101 -->",
        commits: [{ commit: { message: "conflict handoff only" } }]
      }
    ]
  });
  assert.equal(selected[0].source_pr, 101);
});

test("queue skips sources already included in the exact release branch", async () => {
  for (const relation of ["ahead", "identical", "behind", "diverged"]) {
    const first = source(101);
    const second = source(102);
    const releaseSHA = "b".repeat(40);
    const comparisons = [];
    const github = {
      rest: {
        git: {
          async listMatchingRefs(parameters) {
            assert.equal(parameters.ref, "heads/release/0.60");
            return {
              data: [
                {
                  ref: "refs/heads/release/0.600",
                  object: { sha: "c".repeat(40) }
                },
                { ref: "refs/heads/release/0.60", object: { sha: releaseSHA } }
              ]
            };
          }
        },
        repos: {
          async compareCommitsWithBasehead(parameters) {
            comparisons.push(parameters.basehead);
            assert.equal(parameters.per_page, 1);
            return {
              data: {
                status:
                  parameters.basehead.startsWith(first.merge_commit_sha) ?
                    relation
                  : "diverged"
              }
            };
          }
        }
      }
    };
    const includedSourceCommits = await findIncludedSourceCommits({
      github,
      owner: "radius-project",
      repo: "radius",
      channel: "0.60",
      sources: [first, second]
    });
    assert.deepEqual(comparisons, [
      `${first.merge_commit_sha}...${releaseSHA}`,
      `${second.merge_commit_sha}...${releaseSHA}`
    ]);
    const selected = selectNextBackport({
      channel: "0.60",
      sources: [first, second],
      openBackports: [],
      historicalBackports: [],
      includedSourceCommits
    });
    assert.equal(
      selected[0].source_pr,
      ["ahead", "identical"].includes(relation) ? 102 : 101
    );
  }
});

test("a release-branch prefix match does not prove source inclusion", async () => {
  const github = {
    rest: {
      git: {
        async listMatchingRefs() {
          return {
            data: [
              {
                ref: "refs/heads/release/0.600",
                object: { sha: "b".repeat(40) }
              }
            ]
          };
        }
      }
    }
  };
  assert.deepEqual(
    await findIncludedSourceCommits({
      github,
      owner: "radius-project",
      repo: "radius",
      channel: "0.60",
      sources: [source(101)]
    }),
    []
  );
});

test("failed ancestry queries stop queue selection", async () => {
  const github = {
    rest: {
      git: {
        async listMatchingRefs() {
          return {
            data: [
              {
                ref: "refs/heads/release/0.60",
                object: { sha: "b".repeat(40) }
              }
            ]
          };
        }
      },
      repos: {
        async compareCommitsWithBasehead() {
          throw new Error("GitHub unavailable");
        }
      }
    }
  };
  await assert.rejects(
    findIncludedSourceCommits({
      github,
      owner: "radius-project",
      repo: "radius",
      channel: "0.60",
      sources: [source(101)]
    }),
    /GitHub unavailable/
  );
});

test("queue is empty when every source is already included", () => {
  const first = source(101);
  assert.deepEqual(
    selectNextBackport({
      channel: "0.60",
      sources: [first],
      openBackports: [],
      historicalBackports: [],
      includedSourceCommits: [first.merge_commit_sha]
    }),
    []
  );
});

test("uses every current release label on a merged source PR", () => {
  const pull = source(101);
  pull.labels.push({ name: "backport release/0.59" });
  assert.deepEqual(
    entriesForMergedPull(pull).map((entry) => entry.channel),
    ["0.59", "0.60"]
  );
});

test("routes release pushes and merged backports to the same channel", () => {
  for (const context of [
    { eventName: "push", ref: "refs/heads/release/0.60" },
    {
      eventName: "pull_request_target",
      payload: { pull_request: { merged: true, base: { ref: "release/0.60" } } }
    }
  ]) {
    assert.equal(releaseChannelForEvent(context), "0.60");
  }
});

test("does not select a release queue for main or unmerged pull requests", () => {
  for (const context of [
    { eventName: "push", ref: "refs/heads/main" },
    {
      eventName: "pull_request_target",
      payload: { pull_request: { merged: true, base: { ref: "main" } } }
    },
    {
      eventName: "pull_request_target",
      payload: {
        pull_request: { merged: false, base: { ref: "release/0.60" } }
      }
    }
  ]) {
    assert.equal(releaseChannelForEvent(context), "");
  }
});

test("main merge events and release pushes use the same history-ordered queue", async (t) => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "backport-order-"));
  t.after(() => rm(directory, { recursive: true, force: true, maxRetries: 5 }));
  const git = (...args) =>
    execFileSync("git", ["-C", directory, ...args], {
      encoding: "utf8"
    }).trim();
  git("init", "--quiet", "-b", "main");
  git("config", "user.name", "Backport test");
  git("config", "user.email", "test@example.com");
  git("config", "commit.gpgsign", "false");
  git("commit", "--quiet", "--allow-empty", "-m", "fix: merged first");
  const earlier = {
    ...source(102),
    merge_commit_sha: git("rev-parse", "HEAD"),
    merged_at: "2026-10-23T00:00:00Z",
    base: { ref: "main" },
    merged: true
  };
  git("commit", "--quiet", "--allow-empty", "-m", "fix: merged later");
  const later = {
    ...source(101),
    merge_commit_sha: git("rev-parse", "HEAD"),
    merged_at: earlier.merged_at,
    base: { ref: "main" },
    merged: true
  };
  git("update-ref", "refs/remotes/origin/main", "HEAD");
  const original = process.cwd();
  try {
    process.chdir(directory);
    for (const sources of [
      [earlier, later],
      [later, earlier]
    ]) {
      const github = {
        paginate: async (method, args) => method(args),
        rest: {
          pulls: {
            list: async () => [],
            get: async ({ pull_number }) => ({
              data: sources.find((pull) => pull.number === pull_number)
            })
          },
          issues: {
            listForRepo: async () =>
              sources.map(({ number }) => ({ number, pull_request: {} }))
          },
          git: { listMatchingRefs: async () => ({ data: [] }) }
        }
      };
      for (const event of [
        { eventName: "pull_request_target", payload: { pull_request: later } },
        { eventName: "push", ref: "refs/heads/release/0.60" },
        { eventName: "workflow_dispatch", channel: "0.60" }
      ]) {
        const result = await selectReleaseBackports({
          github,
          context: {
            repo: { owner: "radius-project", repo: "recipes" },
            ...event
          },
          channel: event.channel
        });
        assert.equal(result[0].source_pr, 102);
      }
    }
  } finally {
    process.chdir(original);
  }
});

test("manual backport selection rejects invalid channels before API or Git access", async () => {
  for (const channel of ["0", "release/0.62", "0.062", "../main", "0.62\n"]) {
    await assert.rejects(
      selectReleaseBackports({ channel }),
      /channel must use X.Y format/
    );
  }
});
