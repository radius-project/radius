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
import test from "node:test";

import {
  entriesForMergedPull,
  selectNextBackport
} from "./select-release-backports.mjs";

const source = (number) => ({
  number,
  merge_commit_sha: String(number).padStart(40, "a"),
  title: `fix: source ${number}`,
  html_url: `https://example.test/pull/${number}`,
  head: { ref: `feature-${number}` },
  labels: [{ name: "backport release/0.60" }]
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

test("uses every current release label on a merged source PR", () => {
  const pull = source(101);
  pull.labels.push({ name: "backport release/0.59" });
  assert.deepEqual(
    entriesForMergedPull(pull).map((entry) => entry.channel),
    ["0.59", "0.60"]
  );
});
