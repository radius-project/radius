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

const STATUS_LABELS = Object.freeze({
  needsReviewer: "pr:needs-reviewer",
  waitingForReview: "pr:waiting-for-review",
  waitingForAuthor: "pr:waiting-for-author",
  reviewApproved: "pr:review-approved",
  needsRebase: "pr:needs-rebase",
  readyForQueue: "pr:ready-for-queue"
});

const MANUAL_LABELS = Object.freeze({
  needsAuthorResponse: "pr:needs-author-response",
  doNotMerge: "pr:do-not-merge"
});

const MANAGED_LABELS = Object.freeze(Object.values(STATUS_LABELS));

// GitHub's merge-queue GraphQL fields require this feature header.
const MERGE_QUEUE_HEADERS = Object.freeze({
  "GraphQL-Features": "merge_queue"
});

const query = `
  query($owner: String!, $repo: String!, $number: Int!) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        id
        state
        isDraft
        mergeable
        mergeStateStatus
        reviewDecision
        isInMergeQueue
        baseRefName
        baseRefOid
        headRefOid
        author {
          __typename
          login
        }
        baseRef {
          branchProtectionRule {
            requiredStatusChecks {
              context
              app {
                databaseId
              }
            }
            requiredStatusCheckContexts
          }
        }
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

const OPINIONATED_REVIEWS_QUERY = `
  query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        baseRefOid
        headRefOid
        latestOpinionatedReviews(first: 100, after: $cursor) {
          nodes {
            state
            author {
              __typename
              login
            }
            authorCanPushToRepository
            commit {
              oid
            }
            onBehalfOf(first: 100) {
              totalCount
              nodes {
                slug
                organization {
                  login
                }
              }
            }
          }
          pageInfo {
            hasNextPage
            endCursor
          }
        }
      }
    }
  }
`;

const REQUIRED_CHECKS_QUERY = `
  query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                contexts(first: 100, after: $cursor) {
                  nodes {
                    __typename
                    ... on CheckRun {
                      name
                      status
                      conclusion
                      isRequired(pullRequestNumber: $number)
                      checkSuite {
                        app {
                          databaseId
                        }
                      }
                    }
                    ... on StatusContext {
                      context
                      state
                      isRequired(pullRequestNumber: $number)
                    }
                  }
                  pageInfo {
                    hasNextPage
                    endCursor
                  }
                }
              }
            }
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

function sharedCodeOwnerTeams(contents, owner) {
  const prefix = `@${owner.toLowerCase()}/`;
  let shared = null;
  for (const rawLine of contents.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    const [, ...owners] = line.split(/\s+/);
    const teams = new Set(
      owners
        .map((name) => name.toLowerCase())
        .filter(
          (name) => name.startsWith(prefix) && name.length > prefix.length
        )
        .map((name) => name.slice(prefix.length))
    );
    // A team on every rule owns every changed path, regardless of rule precedence.
    shared =
      shared === null
        ? teams
        : new Set([...shared].filter((team) => teams.has(team)));
    if (shared.size === 0) {
      break;
    }
  }
  return shared ?? new Set();
}

async function inferredReviewDecision(github, core, owner, repo, number, pull) {
  if (
    !pull.baseRefName ||
    !pull.baseRefOid ||
    !pull.headRefOid ||
    !pull.baseRef
  ) {
    throw new Error(`Cannot determine the base or head commit for #${number}`);
  }
  if (pull.baseRef.branchProtectionRule) {
    return null;
  }

  const rules = await github.paginate(
    "GET /repos/{owner}/{repo}/rules/branches/{branch}",
    { owner, repo, branch: pull.baseRefName, per_page: 100 }
  );
  const reviewRules = rules.filter((rule) => rule.type === "pull_request");
  if (reviewRules.length === 0) {
    return null;
  }

  let requiredCount = 0;
  let codeOwnerRequired = false;
  for (const rule of reviewRules) {
    const parameters = rule.parameters;
    if (
      !Number.isSafeInteger(parameters?.required_approving_review_count) ||
      parameters.required_approving_review_count < 0 ||
      typeof parameters.require_code_owner_review !== "boolean" ||
      !Array.isArray(parameters.required_reviewers)
    ) {
      throw new Error(`Invalid review ruleset for #${number}`);
    }
    if (parameters.required_reviewers.length > 0) {
      core.info(`#${number}: cannot infer specifically required team reviews`);
      return null;
    }
    if (
      parameters.require_extra_approval_for_unattributed_changes &&
      pull.author?.__typename !== "User"
    ) {
      core.info(`#${number}: cannot infer approvals for an unattributed PR`);
      return null;
    }
    requiredCount = Math.max(
      requiredCount,
      parameters.required_approving_review_count
    );
    codeOwnerRequired ||= parameters.require_code_owner_review;
  }

  const reviews = [];
  let cursor = null;
  do {
    const result = await github.graphql(OPINIONATED_REVIEWS_QUERY, {
      owner,
      repo,
      number,
      cursor
    });
    const reviewedPull = result.repository?.pullRequest;
    if (
      !reviewedPull ||
      reviewedPull.baseRefOid !== pull.baseRefOid ||
      reviewedPull.headRefOid !== pull.headRefOid
    ) {
      throw new Error(
        `The base or head commit changed while reviewing #${number}`
      );
    }
    const page = reviewedPull.latestOpinionatedReviews;
    if (!page || !Array.isArray(page.nodes) || !page.pageInfo) {
      throw new Error(`Invalid review response for #${number}`);
    }
    reviews.push(...page.nodes);
    if (page.pageInfo.hasNextPage && !page.pageInfo.endCursor) {
      throw new Error(`Missing review cursor for #${number}`);
    }
    cursor = page.pageInfo.hasNextPage ? page.pageInfo.endCursor : null;
  } while (cursor);

  if (
    reviews.some(
      (review) =>
        review.state === "CHANGES_REQUESTED" && review.authorCanPushToRepository
    )
  ) {
    return "CHANGES_REQUESTED";
  }

  const approvals = reviews.filter(
    (review) =>
      review.state === "APPROVED" &&
      review.author?.__typename === "User" &&
      review.authorCanPushToRepository &&
      review.author.login.toLowerCase() !== pull.author?.login?.toLowerCase() &&
      review.commit?.oid === pull.headRefOid
  );
  if (
    new Set(approvals.map((review) => review.author.login.toLowerCase())).size <
    Math.max(requiredCount, Number(codeOwnerRequired), 1)
  ) {
    return null;
  }

  if (codeOwnerRequired) {
    let file;
    try {
      ({ data: file } = await github.rest.repos.getContent({
        owner,
        repo,
        path: ".github/CODEOWNERS",
        ref: pull.baseRefOid
      }));
    } catch (error) {
      if (error.status !== 404) {
        throw error;
      }
      core.info(`#${number}: cannot verify CODEOWNERS on the base branch`);
      return null;
    }
    if (
      Array.isArray(file) ||
      file?.type !== "file" ||
      file.encoding !== "base64" ||
      typeof file.content !== "string"
    ) {
      throw new Error(`Invalid CODEOWNERS response for #${number}`);
    }
    const teams = sharedCodeOwnerTeams(
      Buffer.from(file.content, "base64").toString("utf8"),
      owner
    );
    if (
      teams.size === 0 ||
      !approvals.some((review) => {
        const represented = review.onBehalfOf;
        if (
          !represented ||
          represented.totalCount !== represented.nodes?.length
        ) {
          throw new Error(`Incomplete review team attribution for #${number}`);
        }
        return represented.nodes.some(
          (team) =>
            team.organization?.login?.toLowerCase() === owner.toLowerCase() &&
            teams.has(team.slug?.toLowerCase())
        );
      })
    ) {
      core.info(`#${number}: cannot verify a current CODEOWNER approval`);
      return null;
    }
  }

  core.info(`#${number}: inferred approval from current ruleset and reviews`);
  return "APPROVED";
}

function desiredLabels(
  pull,
  rereviewRequested = false,
  requiredChecksPassed = false,
  reviewDecision = pull.reviewDecision
) {
  const existing = currentLabels(pull);
  const desired = new Set();

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
    (mergeabilityUnknown && existing.has(STATUS_LABELS.needsRebase))
  ) {
    desired.add(STATUS_LABELS.needsRebase);
  }

  if (pull.isDraft || existing.has(MANUAL_LABELS.doNotMerge)) {
    return { desired, mergeabilityUnknown };
  }

  if (
    ![null, "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED"].includes(
      reviewDecision
    )
  ) {
    throw new Error(`Unexpected review decision: ${reviewDecision}`);
  }

  let handoff;
  if (existing.has(MANUAL_LABELS.needsAuthorResponse)) {
    handoff = STATUS_LABELS.waitingForAuthor;
  } else if (reviewDecision === "APPROVED") {
    handoff = STATUS_LABELS.reviewApproved;
  } else if (reviewDecision === "CHANGES_REQUESTED") {
    handoff = rereviewRequested
      ? STATUS_LABELS.waitingForReview
      : STATUS_LABELS.waitingForAuthor;
  } else {
    handoff =
      pull.reviewRequests.totalCount > 0
        ? STATUS_LABELS.waitingForReview
        : STATUS_LABELS.needsReviewer;
  }
  desired.add(handoff);

  if (
    handoff === STATUS_LABELS.reviewApproved &&
    pull.mergeable === "MERGEABLE" &&
    (pull.mergeStateStatus === "CLEAN" ||
      (pull.mergeStateStatus === "BEHIND" && requiredChecksPassed)) &&
    !pull.isInMergeQueue
  ) {
    desired.add(STATUS_LABELS.readyForQueue);
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

function hasRereviewRequest(pull, reviews, timeline) {
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

function reviewSignal(run, pull) {
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

function requiredCheck(context, appId) {
  if (typeof context !== "string" || context.length === 0) {
    throw new Error("A required status check has no context name");
  }
  if (
    appId != null &&
    appId !== -1 &&
    (!Number.isSafeInteger(appId) || appId <= 0)
  ) {
    throw new Error(`Invalid integration ID for required check ${context}`);
  }
  return { context, appId: appId > 0 ? appId : null };
}

function observedRequiredCheck(node) {
  if (node.__typename === "CheckRun") {
    return {
      ...requiredCheck(node.name, node.checkSuite?.app?.databaseId),
      required: node.isRequired,
      passed:
        node.status === "COMPLETED" &&
        ["SUCCESS", "NEUTRAL", "SKIPPED"].includes(node.conclusion)
    };
  }
  if (node.__typename === "StatusContext") {
    return {
      ...requiredCheck(node.context, null),
      required: node.isRequired,
      passed: node.state === "SUCCESS"
    };
  }
  throw new Error(`Unsupported status-check context: ${node.__typename}`);
}

async function hasPassingRequiredChecks(github, owner, repo, number, pull) {
  if (!pull.baseRefName || !pull.baseRef) {
    throw new Error(`Cannot determine the base branch for #${number}`);
  }

  const rules = await github.paginate(
    "GET /repos/{owner}/{repo}/rules/branches/{branch}",
    { owner, repo, branch: pull.baseRefName, per_page: 100 }
  );
  const required = [];
  for (const rule of rules) {
    if (rule.type !== "required_status_checks") {
      continue;
    }
    const checks = rule.parameters?.required_status_checks;
    if (!Array.isArray(checks)) {
      throw new Error("A required-status-check rule has no check list");
    }
    required.push(
      ...checks.map((check) =>
        requiredCheck(check.context, check.integration_id)
      )
    );
  }

  const protection = pull.baseRef.branchProtectionRule;
  const classicChecks = protection?.requiredStatusChecks ?? [];
  if (classicChecks.length > 0) {
    required.push(
      ...classicChecks.map((check) =>
        requiredCheck(check.context, check.app?.databaseId)
      )
    );
  } else {
    required.push(
      ...(protection?.requiredStatusCheckContexts ?? []).map((context) =>
        requiredCheck(context, null)
      )
    );
  }

  const observed = [];
  let cursor = null;
  do {
    const result = await github.graphql(REQUIRED_CHECKS_QUERY, {
      owner,
      repo,
      number,
      cursor,
      headers: MERGE_QUEUE_HEADERS
    });
    const commit = result.repository?.pullRequest?.commits?.nodes?.[0]?.commit;
    if (!commit) {
      throw new Error(`Cannot find the head commit for #${number}`);
    }
    if (!commit.statusCheckRollup) {
      return required.length === 0;
    }
    const page = commit.statusCheckRollup.contexts;
    if (!page || !Array.isArray(page.nodes) || !page.pageInfo) {
      throw new Error(`Invalid status-check response for #${number}`);
    }
    observed.push(
      ...page.nodes.map(observedRequiredCheck).filter((check) => check.required)
    );
    if (page.pageInfo.hasNextPage && !page.pageInfo.endCursor) {
      throw new Error(`Missing status-check cursor for #${number}`);
    }
    cursor = page.pageInfo.hasNextPage ? page.pageInfo.endCursor : null;
  } while (cursor);

  return (
    observed.every((check) => check.passed) &&
    observed.every((check) =>
      required.some(
        (expected) =>
          expected.context === check.context &&
          (expected.appId == null || expected.appId === check.appId)
      )
    ) &&
    required.every((expected) =>
      observed.some(
        (check) =>
          check.passed &&
          check.context === expected.context &&
          (expected.appId == null || expected.appId === check.appId)
      )
    )
  );
}

async function syncPull(github, core, owner, repo, number) {
  const result = await github.graphql(query, {
    owner,
    repo,
    number,
    headers: MERGE_QUEUE_HEADERS
  });
  const pull = result.repository?.pullRequest;
  if (!pull) {
    throw new Error(`Pull request #${number} was not found`);
  }

  const existing = currentLabels(pull);
  if (
    pull.state === "OPEN" &&
    pull.isInMergeQueue &&
    (existing.has(MANUAL_LABELS.doNotMerge) ||
      existing.has(MANUAL_LABELS.needsAuthorResponse))
  ) {
    // DequeuePullRequestInput.id is the pull request node ID.
    await github.graphql(
      `mutation($pullRequestId: ID!) {
        dequeuePullRequest(input: { id: $pullRequestId }) {
          mergeQueueEntry {
            id
          }
        }
      }`,
      { pullRequestId: pull.id, headers: MERGE_QUEUE_HEADERS }
    );
    core.info(`Removed held pull request #${number} from the merge queue`);
  }

  const reviewDecision =
    pull.state === "OPEN" &&
    !pull.isDraft &&
    pull.reviewDecision === null &&
    !existing.has(MANUAL_LABELS.doNotMerge) &&
    !existing.has(MANUAL_LABELS.needsAuthorResponse)
      ? await inferredReviewDecision(github, core, owner, repo, number, pull)
      : pull.reviewDecision;

  let rereviewRequested = false;
  if (
    pull.state === "OPEN" &&
    reviewDecision === "CHANGES_REQUESTED" &&
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

  let requiredChecksPassed = false;
  if (
    pull.state === "OPEN" &&
    !pull.isDraft &&
    reviewDecision === "APPROVED" &&
    pull.mergeable === "MERGEABLE" &&
    pull.mergeStateStatus === "BEHIND" &&
    !pull.isInMergeQueue &&
    !existing.has(MANUAL_LABELS.doNotMerge) &&
    !existing.has(MANUAL_LABELS.needsAuthorResponse)
  ) {
    requiredChecksPassed = await hasPassingRequiredChecks(
      github,
      owner,
      repo,
      number,
      pull
    );
    if (!requiredChecksPassed) {
      core.info(`#${number}: waiting for required checks before queueing`);
    }
  }

  const { desired, mergeabilityUnknown } = desiredLabels(
    pull,
    rereviewRequested,
    requiredChecksPassed,
    reviewDecision
  );
  if (mergeabilityUnknown) {
    core.warning(
      `Mergeability for #${number} is unknown; queue and rebase labels will be reconciled on the next run`
    );
  }
  for (const name of MANAGED_LABELS) {
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

/** @param {import('@actions/github-script').AsyncFunctionArguments} AsyncFunctionArguments */
export default async ({ github, context, core }) => {
  const { owner, repo } = context.repo;

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
};
