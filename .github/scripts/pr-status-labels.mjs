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

const labelDefinitions = {
  "pr:needs-reviewer": {
    color: "BFD4F2",
    description: "Ready for review but no reviewer has been requested"
  },
  "pr:waiting-for-review": {
    color: "0E8A16",
    description: "A requested reviewer owns the next action"
  },
  "pr:waiting-for-author": {
    color: "FBCA04",
    description: "The author needs to address review feedback"
  },
  "pr:review-approved": {
    color: "7FD3B1",
    description: "Required reviews are approved; checks may still be pending"
  },
  "pr:needs-rebase": {
    color: "D93F0B",
    description: "The pull request has merge conflicts"
  },
  "pr:ready-for-queue": {
    color: "5319E7",
    description:
      "Review, checks and mergeability permit adding this PR to the queue"
  },
  "pr:breaking-change": {
    color: "B60205",
    description: "The pull request title declares a breaking change"
  },
  "pr:needs-author-response": {
    color: "F9D0C4",
    description: "A reviewer manually requested an author response"
  }
};

export const managedLabels = Object.freeze(
  Object.keys(labelDefinitions).filter(
    (name) => name !== "pr:needs-author-response"
  )
);

const query = `
  query($owner: String!, $repo: String!, $number: Int!) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        id
        state
        isDraft
        title
        mergeable
        mergeStateStatus
        reviewDecision
        isInMergeQueue
        reviewRequests(first: 100) {
          totalCount
          nodes {
            requestedReviewer {
              ... on User {
                login
              }
              ... on Team {
                slug
              }
            }
          }
        }
        labels(first: 100) {
          totalCount
          nodes {
            name
          }
        }
      }
    }
  }
`;

function currentLabels(pull) {
  if (pull.labels.totalCount !== pull.labels.nodes.length) {
    throw new Error("Cannot classify a pull request with more than 100 labels");
  }
  return new Set(pull.labels.nodes.map((label) => label.name));
}

export function desiredLabels(pull, rereviewRequested = false) {
  const existing = currentLabels(pull);
  const desired = new Set();
  const breakingTitle =
    /^(?:feat|fix|perf|refactor|style|revert|docs|test|build|ci|chore)(?:\([^)]+\))?!: .+/;

  if (breakingTitle.test(pull.title)) {
    desired.add("pr:breaking-change");
  }
  if (pull.state !== "OPEN") {
    return { desired, mergeabilityUnknown: false };
  }

  const mergeabilityUnknown =
    pull.mergeable == null ||
    pull.mergeable === "UNKNOWN" ||
    pull.mergeStateStatus == null ||
    pull.mergeStateStatus === "UNKNOWN";

  if (
    pull.mergeable === "CONFLICTING" ||
    pull.mergeStateStatus === "DIRTY" ||
    (mergeabilityUnknown && existing.has("pr:needs-rebase"))
  ) {
    desired.add("pr:needs-rebase");
  }

  if (
    pull.isDraft ||
    existing.has("blocked") ||
    existing.has("pr:do-not-merge")
  ) {
    return { desired, mergeabilityUnknown };
  }

  if (
    ![null, "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED"].includes(
      pull.reviewDecision
    )
  ) {
    throw new Error(`Unexpected review decision: ${pull.reviewDecision}`);
  }

  let handoff;
  if (existing.has("pr:needs-author-response")) {
    handoff = "pr:waiting-for-author";
  } else if (pull.reviewDecision === "APPROVED") {
    handoff = "pr:review-approved";
  } else if (pull.reviewDecision === "CHANGES_REQUESTED") {
    handoff =
      rereviewRequested ? "pr:waiting-for-review" : "pr:waiting-for-author";
  } else {
    handoff =
      pull.reviewRequests.totalCount > 0 ?
        "pr:waiting-for-review"
      : "pr:needs-reviewer";
  }
  desired.add(handoff);

  if (
    handoff === "pr:review-approved" &&
    pull.mergeable === "MERGEABLE" &&
    pull.mergeStateStatus === "CLEAN" &&
    !pull.isInMergeQueue
  ) {
    desired.add("pr:ready-for-queue");
  }

  return { desired, mergeabilityUnknown };
}

function reviewerKey(reviewer) {
  if (reviewer?.login) {
    return `user:${reviewer.login.toLowerCase()}`;
  }
  if (reviewer?.slug) {
    return `team:${reviewer.slug.toLowerCase()}`;
  }
  return null;
}

export function hasRereviewRequest(pull, reviews, timeline) {
  if (pull.reviewRequests.totalCount !== pull.reviewRequests.nodes.length) {
    throw new Error("Cannot classify more than 100 pending review requests");
  }

  const decisive = new Map();
  const decisions = reviews.filter((review) =>
    ["CHANGES_REQUESTED", "APPROVED"].includes(review.state)
  );
  for (const review of decisions) {
    if (
      !review.user?.login ||
      !Number.isFinite(Date.parse(review.submitted_at))
    ) {
      throw new Error(
        "A decisive review has no reviewer or valid submission time"
      );
    }
  }
  decisions.sort(
    (left, right) =>
      Date.parse(left.submitted_at) - Date.parse(right.submitted_at) ||
      left.id - right.id
  );
  for (const review of decisions) {
    decisive.set(review.user.login.toLowerCase(), review);
  }
  const changes = [...decisive.values()].filter(
    (review) => review.state === "CHANGES_REQUESTED"
  );
  if (changes.length === 0) {
    return false;
  }
  const lastChange = Math.max(
    ...changes.map((review) => Date.parse(review.submitted_at))
  );
  if (!Number.isFinite(lastChange)) {
    throw new Error("An outstanding change request has an invalid timestamp");
  }

  const pending = new Set(
    pull.reviewRequests.nodes.map((request) =>
      reviewerKey(request.requestedReviewer)
    )
  );
  // An earlier request does not hand feedback back to reviewers after changes are requested.
  return timeline.some((event) => {
    if (event.event !== "review_requested") {
      return false;
    }
    const key = reviewerKey(event.requested_reviewer ?? event.requested_team);
    if (key == null || !pending.has(key)) {
      return false;
    }
    const requestedAt = Date.parse(event.created_at);
    if (!Number.isFinite(requestedAt)) {
      throw new Error("A pending review request has an invalid timestamp");
    }
    return requestedAt > lastChange;
  });
}

export function reviewSignal(run, pull) {
  // A fork can change the read-only signal workflow's run-name; bind it to the actual PR commit.
  const match = /^([1-9]\d*)$/.exec(run.display_title ?? "");
  if (
    run.event !== "pull_request_review" ||
    run.conclusion !== "success" ||
    !match ||
    pull.number !== Number(match[1]) ||
    !Number.isSafeInteger(Number(match[1]))
  ) {
    return null;
  }

  const sha = run.head_sha?.toLowerCase();
  if (
    !/^[0-9a-f]{40}$/.test(sha ?? "") ||
    ![pull.head.sha, pull.merge_commit_sha]
      .filter(Boolean)
      .some((commit) => commit.toLowerCase() === sha)
  ) {
    return null;
  }
  return Number(match[1]);
}

async function ensureLabels(github, core, owner, repo) {
  const available = await github.paginate(
    github.rest.issues.listLabelsForRepo,
    {
      owner,
      repo,
      per_page: 100
    }
  );
  const names = new Set(available.map((label) => label.name));
  for (const [name, definition] of Object.entries(labelDefinitions)) {
    if (names.has(name)) {
      continue;
    }
    try {
      await github.rest.issues.createLabel({
        owner,
        repo,
        name,
        ...definition
      });
    } catch (error) {
      if (error.status !== 422) {
        throw error;
      }
      await github.rest.issues.getLabel({ owner, repo, name });
      core.info(`Label ${name} was created by another workflow run`);
    }
  }
}

export async function syncPull(github, core, owner, repo, number) {
  const result = await github.graphql(query, { owner, repo, number });
  const pull = result.repository?.pullRequest;
  if (!pull) {
    throw new Error(`Pull request #${number} was not found`);
  }

  const existing = currentLabels(pull);
  if (
    pull.state === "OPEN" &&
    pull.isInMergeQueue &&
    (existing.has("blocked") ||
      existing.has("pr:do-not-merge") ||
      existing.has("pr:needs-author-response"))
  ) {
    await github.graphql(
      `mutation($id: ID!) {
        dequeuePullRequest(input: { id: $id }) {
          mergeQueueEntry {
            id
          }
        }
      }`,
      { id: pull.id }
    );
    core.info(`Removed held pull request #${number} from the merge queue`);
  }

  let rereviewRequested = false;
  if (
    pull.state === "OPEN" &&
    pull.reviewDecision === "CHANGES_REQUESTED" &&
    pull.reviewRequests.totalCount > 0
  ) {
    const [reviews, timeline] = await Promise.all([
      github.paginate(github.rest.pulls.listReviews, {
        owner,
        repo,
        pull_number: number,
        per_page: 100
      }),
      github.paginate(github.rest.issues.listEventsForTimeline, {
        owner,
        repo,
        issue_number: number,
        per_page: 100
      })
    ]);
    rereviewRequested = hasRereviewRequest(pull, reviews, timeline);
    if (!reviews.some((review) => review.state === "CHANGES_REQUESTED")) {
      core.warning(
        `#${number} reports changes requested but has no active change-request review`
      );
    }
  }

  const { desired, mergeabilityUnknown } = desiredLabels(
    pull,
    rereviewRequested
  );
  if (mergeabilityUnknown) {
    core.warning(
      `Mergeability for #${number} is unknown; queue and rebase labels will be reconciled on the next run`
    );
  }
  for (const name of managedLabels) {
    if (!existing.has(name) || desired.has(name)) {
      continue;
    }
    try {
      await github.rest.issues.removeLabel({
        owner,
        repo,
        issue_number: number,
        name
      });
    } catch (error) {
      if (error.status !== 404) {
        throw error;
      }
      core.info(`Label ${name} was already removed from #${number}`);
    }
  }

  const missing = [...desired].filter((name) => !existing.has(name));
  if (missing.length > 0) {
    await github.rest.issues.addLabels({
      owner,
      repo,
      issue_number: number,
      labels: missing
    });
  }
  core.info(`#${number}: ${[...desired].join(", ") || "no automated labels"}`);
}

export default async function run({ github, context, core }) {
  const { owner, repo } = context.repo;
  await ensureLabels(github, core, owner, repo);

  if (context.eventName === "pull_request_target") {
    await syncPull(
      github,
      core,
      owner,
      repo,
      context.payload.pull_request.number
    );
    return;
  }

  if (context.eventName === "workflow_run") {
    const run = context.payload.workflow_run;
    const match = /^([1-9]\d*)$/.exec(run.display_title ?? "");
    if (!match || !Number.isSafeInteger(Number(match[1]))) {
      core.warning("Ignoring a review signal without a valid PR number");
      return;
    }
    const number = Number(match[1]);
    const { data: pull } = await github.rest.pulls.get({
      owner,
      repo,
      pull_number: number
    });
    if (!reviewSignal(run, pull)) {
      core.warning(
        `Ignoring an unverified or outdated review signal for #${number}; the scheduled run will reconcile it`
      );
      return;
    }
    await syncPull(github, core, owner, repo, number);
    return;
  }

  if (context.eventName === "workflow_dispatch") {
    const number = context.payload.inputs?.pr_number;
    if (number) {
      if (
        !/^[1-9]\d*$/.test(String(number)) ||
        !Number.isSafeInteger(Number(number))
      ) {
        throw new Error(`Invalid pull request number: ${number}`);
      }
      await syncPull(github, core, owner, repo, Number(number));
      return;
    }
  } else if (context.eventName !== "schedule") {
    throw new Error(`Unsupported event: ${context.eventName}`);
  }

  const open = await github.paginate(github.rest.pulls.list, {
    owner,
    repo,
    state: "open",
    per_page: 100
  });
  const failures = [];
  for (const pull of open) {
    try {
      await syncPull(github, core, owner, repo, pull.number);
    } catch (error) {
      core.error(`#${pull.number}: ${error}`);
      failures.push(pull.number);
    }
  }
  if (failures.length > 0) {
    throw new Error(
      `Failed to reconcile pull requests: ${failures.join(", ")}`
    );
  }
}
