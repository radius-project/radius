/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import assert from "node:assert/strict";
import test from "node:test";
import run, {
  desiredLabels,
  hasRereviewRequest,
  reviewSignal,
  syncPull
} from "./pr-status-labels.mjs";

function pull(options = {}) {
  const { labelNames = [], requested = [], ...fields } = options;
  return {
    state: "OPEN",
    isDraft: false,
    title: "fix(cli): make review updates reliable",
    mergeable: "MERGEABLE",
    mergeStateStatus: "BLOCKED",
    reviewDecision: "REVIEW_REQUIRED",
    isInMergeQueue: false,
    ...fields,
    labels: fields.labels ?? {
      totalCount: labelNames.length,
      nodes: labelNames.map((name) => ({ name }))
    },
    reviewRequests: fields.reviewRequests ?? {
      totalCount: requested.length,
      nodes: requested.map((requestedReviewer) => ({ requestedReviewer }))
    }
  };
}

const handoffCases = [
  {
    name: "a ready PR without reviewers needs a reviewer",
    options: {},
    expected: ["pr:needs-reviewer"]
  },
  {
    name: "a requested team owns the review",
    options: { requested: [{ slug: "approvers-radius" }] },
    expected: ["pr:waiting-for-review"]
  },
  {
    name: "a requested change belongs to the author until rereview",
    options: {
      reviewDecision: "CHANGES_REQUESTED",
      requested: [{ slug: "approvers-radius" }]
    },
    expected: ["pr:waiting-for-author"]
  },
  {
    name: "an explicit rereview request returns the handoff to reviewers",
    options: {
      reviewDecision: "CHANGES_REQUESTED",
      requested: [{ login: "reviewer" }]
    },
    rereviewRequested: true,
    expected: ["pr:waiting-for-review"]
  },
  {
    name: "a manual author-response request overrides an approval",
    options: {
      labelNames: ["pr:needs-author-response"],
      reviewDecision: "APPROVED",
      mergeStateStatus: "CLEAN"
    },
    expected: ["pr:waiting-for-author"]
  },
  {
    name: "approval and all merge requirements allow queueing",
    options: { reviewDecision: "APPROVED", mergeStateStatus: "CLEAN" },
    expected: ["pr:ready-for-queue", "pr:review-approved"]
  },
  {
    name: "approval while checks are blocked is not queue-ready",
    options: { reviewDecision: "APPROVED" },
    expected: ["pr:review-approved"]
  },
  {
    name: "a queued PR is not waiting to be queued again",
    options: {
      reviewDecision: "APPROVED",
      mergeStateStatus: "CLEAN",
      isInMergeQueue: true
    },
    expected: ["pr:review-approved"]
  },
  {
    name: "the blocked label suppresses handoffs and queueing",
    options: {
      reviewDecision: "APPROVED",
      mergeStateStatus: "CLEAN",
      labelNames: ["blocked"]
    },
    expected: []
  },
  {
    name: "the do-not-merge label suppresses handoffs and queueing",
    options: {
      reviewDecision: "APPROVED",
      mergeStateStatus: "CLEAN",
      labelNames: ["pr:do-not-merge"]
    },
    expected: []
  },
  {
    name: "draft PRs have no handoff",
    options: { isDraft: true, requested: [{ login: "reviewer" }] },
    expected: []
  },
  {
    name: "closed PRs lose transient labels",
    options: {
      state: "CLOSED",
      labelNames: ["pr:waiting-for-author", "pr:needs-rebase"]
    },
    expected: []
  },
  {
    name: "conflicts are labeled independently of the author handoff",
    options: { reviewDecision: "CHANGES_REQUESTED", mergeable: "CONFLICTING" },
    expected: ["pr:needs-rebase", "pr:waiting-for-author"]
  },
  {
    name: "a branch merely behind its base does not need a conflict label",
    options: { mergeStateStatus: "BEHIND" },
    expected: ["pr:needs-reviewer"]
  },
  {
    name: "a dirty merge requires updating the branch",
    options: { mergeStateStatus: "DIRTY" },
    expected: ["pr:needs-rebase", "pr:needs-reviewer"]
  },
  {
    name: "unknown mergeability retains an existing rebase label",
    options: {
      mergeable: "UNKNOWN",
      mergeStateStatus: "UNKNOWN",
      reviewDecision: "APPROVED",
      labelNames: ["pr:needs-rebase", "pr:ready-for-queue"]
    },
    expected: ["pr:needs-rebase", "pr:review-approved"],
    unknown: true
  },
  {
    name: "a conventional breaking title adds the existing label",
    options: { title: "feat(api)!: change resource contract" },
    expected: ["pr:breaking-change", "pr:needs-reviewer"]
  },
  {
    name: "an invalid breaking title does not imply a breaking change",
    options: {
      title: "something!: not a conventional title",
      labelNames: ["pr:breaking-change"]
    },
    expected: ["pr:needs-reviewer"]
  }
];

for (const {
  name,
  options,
  expected,
  rereviewRequested,
  unknown
} of handoffCases) {
  test(name, () => {
    const result = desiredLabels(pull(options), rereviewRequested);
    assert.deepEqual([...result.desired].sort(), expected.sort());
    assert.equal(result.mergeabilityUnknown, unknown ?? false);
  });
}

test("unrecognized review decisions and incomplete label lists fail", () => {
  assert.throws(
    () => desiredLabels(pull({ reviewDecision: "PENDING" })),
    /Unexpected review decision/
  );
  assert.throws(
    () =>
      desiredLabels(
        pull({ labels: { totalCount: 101, nodes: [{ name: "blocked" }] } })
      ),
    /more than 100 labels/
  );
});

const change = (user, submittedAt, state = "CHANGES_REQUESTED") => ({
  user: { login: user },
  submitted_at: submittedAt,
  state
});
const request = (createdAt, reviewer) => ({
  event: "review_requested",
  created_at: createdAt,
  ...(reviewer.slug ?
    { requested_team: reviewer }
  : { requested_reviewer: reviewer })
});

test("only a pending re-request after the latest outstanding change transfers review", () => {
  const pending = pull({
    reviewDecision: "CHANGES_REQUESTED",
    requested: [{ login: "Alice" }, { slug: "approvers-radius" }]
  });
  const reviews = [change("alice", "2026-09-01T12:00:00Z")];
  const timeline = [
    request("2026-09-01T11:00:00Z", { login: "Alice" }),
    request("2026-09-01T12:01:00Z", { slug: "approvers-radius" })
  ];
  assert.equal(hasRereviewRequest(pending, reviews, timeline), true);
  assert.equal(
    hasRereviewRequest(pending, reviews, timeline.slice(0, 1)),
    false
  );
  assert.equal(
    hasRereviewRequest(
      pull({ requested: [{ login: "Alice" }] }),
      reviews,
      timeline.slice(1)
    ),
    false
  );
});

test("a newer outstanding change supersedes an earlier rereview request", () => {
  const pending = pull({ requested: [{ login: "Alice" }] });
  const reviews = [
    change("alice", "2026-09-01T12:00:00Z"),
    change("bob", "2026-09-01T12:02:00Z")
  ];
  const timeline = [request("2026-09-01T12:01:00Z", { login: "Alice" })];
  assert.equal(hasRereviewRequest(pending, reviews, timeline), false);
  assert.equal(
    hasRereviewRequest(
      pending,
      [...reviews, change("bob", "2026-09-01T12:03:00Z", "APPROVED")],
      timeline
    ),
    true
  );
});

test("review order in the API cannot revive an older change request", () => {
  assert.equal(
    hasRereviewRequest(
      pull({ requested: [{ login: "Alice" }] }),
      [
        change("alice", "2026-09-01T12:03:00Z", "APPROVED"),
        change("alice", "2026-09-01T12:00:00Z")
      ],
      [request("2026-09-01T12:01:00Z", { login: "Alice" })]
    ),
    false
  );
});

test("comments do not cancel a changes-requested review", () => {
  const pending = pull({ requested: [{ login: "Alice" }] });
  const reviews = [
    change("alice", "2026-09-01T12:00:00Z"),
    change("alice", "2026-09-01T12:02:00Z", "COMMENTED")
  ];
  assert.equal(
    hasRereviewRequest(pending, reviews, [
      request("2026-09-01T12:01:00Z", { login: "Alice" })
    ]),
    true
  );
});

test("an incomplete request list or review with no timestamp fails", () => {
  const pending = pull({
    reviewRequests: {
      totalCount: 101,
      nodes: [{ requestedReviewer: { login: "a" } }]
    }
  });
  assert.throws(
    () => hasRereviewRequest(pending, [], []),
    /more than 100 pending review requests/
  );
  assert.throws(
    () =>
      hasRereviewRequest(
        pull({ requested: [{ login: "a" }] }),
        [{ state: "CHANGES_REQUESTED", user: { login: "a" } }],
        []
      ),
    /no reviewer or valid submission time/
  );
});

test("a fork review signal must identify this PR and its actual commit", () => {
  const run = {
    display_title: "42",
    event: "pull_request_review",
    conclusion: "success",
    head_sha: "a".repeat(40)
  };
  const source = {
    number: 42,
    head: { sha: "b".repeat(40) },
    merge_commit_sha: "a".repeat(40)
  };
  assert.equal(reviewSignal(run, source), 42);
  assert.equal(reviewSignal({ ...run, head_sha: source.head.sha }, source), 42);
  assert.equal(
    reviewSignal({ ...run, head_sha: "c".repeat(40) }, source),
    null
  );
  assert.equal(reviewSignal({ ...run, display_title: "43" }, source), null);
  assert.equal(reviewSignal({ ...run, display_title: "PR #42" }, source), null);
  assert.equal(reviewSignal({ ...run, conclusion: "failure" }, source), null);
  assert.equal(reviewSignal({ ...run, event: "pull_request" }, source), null);
  assert.equal(
    reviewSignal({ ...run, display_title: "9007199254740993" }, source),
    null
  );
});

test("synchronization changes only its own labels and is idempotent", async () => {
  let data = pull({
    labelNames: ["pr:waiting-for-review", "pr:standard", "cli"],
    reviewDecision: "APPROVED",
    mergeStateStatus: "CLEAN"
  });
  const added = [];
  const removed = [];
  const github = {
    graphql: async () => ({ repository: { pullRequest: data } }),
    rest: {
      issues: {
        addLabels: async ({ labels }) => added.push(...labels),
        removeLabel: async ({ name }) => removed.push(name)
      }
    }
  };
  const core = { info() {}, warning() {} };

  await syncPull(github, core, "radius-project", "radius", 42);
  assert.deepEqual(removed, ["pr:waiting-for-review"]);
  assert.deepEqual(added.sort(), ["pr:ready-for-queue", "pr:review-approved"]);

  data = pull({
    labelNames: ["pr:standard", "cli", ...added],
    reviewDecision: "APPROVED",
    mergeStateStatus: "CLEAN"
  });
  await syncPull(github, core, "radius-project", "radius", 42);
  assert.equal(removed.length, 1);
  assert.equal(added.length, 2);
});

for (const hold of ["blocked", "pr:do-not-merge", "pr:needs-author-response"]) {
  test(`${hold} removes an already queued PR before updating its labels`, async () => {
    const queued = pull({
      id: "PR_queued",
      labelNames: [hold, "pr:ready-for-queue", "pr:review-approved"],
      reviewDecision: "APPROVED",
      mergeStateStatus: "CLEAN",
      isInMergeQueue: true
    });
    const calls = [];
    const github = {
      graphql: async (source, variables) => {
        if (source.includes("dequeuePullRequest")) {
          calls.push(["dequeue", variables.id]);
          return { dequeuePullRequest: { mergeQueueEntry: { id: "MQ_1" } } };
        }
        return { repository: { pullRequest: queued } };
      },
      rest: {
        issues: {
          removeLabel: async ({ name }) => calls.push(["remove", name]),
          addLabels: async ({ labels }) => calls.push(["add", labels])
        }
      }
    };
    await syncPull(
      github,
      { info() {}, warning() {} },
      "radius-project",
      "radius",
      42
    );
    assert.deepEqual(calls, [
      ["dequeue", "PR_queued"],
      ["remove", "pr:review-approved"],
      ["remove", "pr:ready-for-queue"],
      ...(hold === "pr:needs-author-response" ?
        [["add", ["pr:waiting-for-author"]]]
      : [])
    ]);
  });
}

test("a failed dequeue does not report a held PR as synchronized", async () => {
  const github = {
    graphql: async (source) => {
      if (source.includes("dequeuePullRequest")) {
        throw new Error("Dequeue denied");
      }
      return {
        repository: {
          pullRequest: pull({
            id: "PR_queued",
            labelNames: ["pr:do-not-merge"],
            isInMergeQueue: true
          })
        }
      };
    }
  };
  await assert.rejects(
    syncPull(
      github,
      { info() {}, warning() {} },
      "radius-project",
      "radius",
      42
    ),
    /Dequeue denied/
  );
});

test("a scheduled run continues past a failed PR, then reports the failure", async () => {
  const created = [];
  const errors = [];
  const added = [];
  const github = {
    paginate: async (endpoint) =>
      endpoint === github.rest.pulls.list ?
        [{ number: 1 }, { number: 2 }]
      : [{ name: "pr:breaking-change" }],
    graphql: async (_, { number }) => {
      if (number === 1) {
        throw new Error("API unavailable");
      }
      return { repository: { pullRequest: pull() } };
    },
    rest: {
      issues: {
        listLabelsForRepo() {},
        createLabel: async ({ name }) => created.push(name),
        addLabels: async ({ labels }) => added.push(...labels)
      },
      pulls: { list() {} }
    }
  };
  const core = {
    info() {},
    warning() {},
    error: (message) => errors.push(message)
  };
  await assert.rejects(
    run({
      github,
      context: {
        repo: { owner: "radius-project", repo: "radius" },
        eventName: "schedule"
      },
      core
    }),
    /Failed to reconcile pull requests: 1/
  );
  assert.ok(created.includes("pr:needs-author-response"));
  assert.deepEqual(added, ["pr:needs-reviewer"]);
  assert.match(errors[0], /#1: Error: API unavailable/);
});
