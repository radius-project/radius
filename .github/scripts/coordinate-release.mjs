import { readFile } from "node:fs/promises";
import { isDeepStrictEqual } from "node:util";

const apiVersion = "2026-03-10";
const versionPattern = /^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/;
// Every receipt lives in one environment. The version and task name identify
// a receipt, and the receipt is bound to the Radius source SHA, so releases do
// not create environments of their own.
const receiptEnvironment = "release-coordination";
const receiptTask = (version, task) =>
  `release-coordination:${version}:${task.id}`;
// The docs and samples upmerge workflows open a pull request into edge in a
// step with this name; its conclusion tells whether a pull request exists.
const pullRequestStep = "Create pull request";
const upmergeBase = "edge";
const upmergeBranchPrefix = "upmerge/";
const pullRequestSlack = 5000;

export function coordinationTasks(releaseType, version) {
  const inputs = { version: version.replace(/^v/, "") };
  const tasks =
    releaseType === "rc" ?
      [
        {
          id: "docs-upmerge",
          repo: "docs",
          workflow: "upmerge.yaml",
          inputs: {}
        },
        {
          id: "samples-upmerge",
          repo: "samples",
          workflow: "upmerge.yaml",
          inputs: {}
        }
      ]
    : releaseType === "final" ?
      [
        {
          id: "docs-release",
          repo: "docs",
          workflow: "release.yaml",
          ref: "edge",
          inputs
        },
        {
          id: "samples-release",
          repo: "samples",
          workflow: "release.yaml",
          ref: "edge",
          inputs
        }
      ]
    : [];
  return [
    ...tasks,
    {
      id: "sample-tests",
      repo: "samples",
      workflow: "test.yaml",
      ref: "edge",
      inputs
    }
  ];
}

async function tagCommit(github, context, version) {
  let {
    data: { object }
  } = await github.rest.git.getRef({ ...context.repo, ref: `tags/${version}` });
  for (let depth = 0; object.type === "tag" && depth < 5; depth++) {
    ({
      data: { object }
    } = await github.rest.git.getTag({ ...context.repo, tag_sha: object.sha }));
  }
  if (object.type !== "commit" || !/^[a-f0-9]{40}$/.test(object.sha))
    throw new Error(`Cannot resolve ${version} to a commit`);
  return object.sha;
}

async function deploymentFor(github, context, version, sourceSha, task) {
  const deployments = await github.paginate(github.rest.repos.listDeployments, {
    ...context.repo,
    environment: receiptEnvironment,
    task: receiptTask(version, task),
    per_page: 100
  });
  if (
    deployments.length > 1 ||
    deployments.some((entry) => entry.sha !== sourceSha)
  ) {
    throw new Error(
      `Conflicting coordination receipt for ${version} ${task.id}`
    );
  }
  return deployments[0];
}

function pullRequestUrl(context, task, number) {
  return `${context.serverUrl ?? "https://github.com"}/${context.repo.owner}/${task.repo}/pull/${number}`;
}

// Binds the pull request that a successful upmerge run opened. The workflow
// exposes no identifier for it, so the pull request is the single upmerge
// pull request into edge created while the run's pull-request step ran; the
// binding is stored on the receipt and never searched for again. Returns null
// when the run had nothing to merge and skipped the step.
async function createdPullRequest(call, context, task, run) {
  const { data: jobs } = await call(
    "GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs",
    { run_id: run.id, filter: "latest", per_page: 100 }
  );
  const step = jobs.jobs
    .flatMap((job) => job.steps ?? [])
    .find((entry) => entry.name === pullRequestStep);
  if (!step) {
    throw new Error(
      `${run.html_url} has no '${pullRequestStep}' step; bind its pull request as the receipt's environment URL before resuming`
    );
  }
  if (step.conclusion === "skipped") return null;
  if (step.conclusion !== "success") {
    throw new Error(
      `${run.html_url} did not create its pull request; reconcile the receipt before resuming`
    );
  }
  const earliest = Date.parse(step.started_at) - pullRequestSlack;
  const latest = Date.parse(step.completed_at) + pullRequestSlack;
  const { data: pulls } = await call("GET /repos/{owner}/{repo}/pulls", {
    base: upmergeBase,
    state: "all",
    sort: "created",
    direction: "desc",
    per_page: 100
  });
  const candidates = pulls.filter(
    (pull) =>
      pull.head.ref.startsWith(upmergeBranchPrefix) &&
      Date.parse(pull.created_at) >= earliest &&
      Date.parse(pull.created_at) <= latest
  );
  if (candidates.length !== 1) {
    throw new Error(
      `${candidates.length} upmerge pull requests match ${run.html_url}; bind the right one as the receipt's environment URL before resuming`
    );
  }
  return candidates[0].html_url;
}

// An upmerge is complete when the pull request its run opened has merged.
// docs and samples squash-merge, so the source commits never become
// reachable from edge and cannot serve as the completion signal.
async function verifyUpmerge({ call, context, task, run, status, record }) {
  const prefix = pullRequestUrl(context, task, "");
  let url = status?.environment_url;
  if (!url?.startsWith(prefix) || !/^\d+$/.test(url.slice(prefix.length))) {
    url = await createdPullRequest(call, context, task, run);
    if (!url) return null;
  }
  const number = Number(url.slice(prefix.length));
  const { data: pull } = await call(
    "GET /repos/{owner}/{repo}/pulls/{pull_number}",
    { pull_number: number }
  );
  if (
    pull.base.ref !== upmergeBase ||
    !pull.head.ref.startsWith(upmergeBranchPrefix)
  ) {
    throw new Error(
      `${url} is not an upmerge pull request into ${upmergeBase}`
    );
  }
  if (pull.merged) return url;
  if (pull.state === "closed") {
    await record(
      "failure",
      run.html_url,
      `Pull request #${number} closed without merging`,
      url
    );
    throw new Error(
      `${task.repo} upmerge pull request ${url} closed without merging; reconcile the receipt before resuming`
    );
  }
  await record(
    "in_progress",
    run.html_url,
    `Waiting for pull request #${number} to merge`,
    url
  );
  throw new Error(`Merge the ${task.repo} upmerge PR ${url}, then resume`);
}

async function latestStatus(github, context, deployment) {
  const statuses = await github.paginate(
    github.rest.repos.listDeploymentStatuses,
    {
      ...context.repo,
      deployment_id: deployment.id,
      per_page: 100
    }
  );
  return statuses.sort((left, right) => right.id - left.id)[0];
}

async function publishedRelease(github, context, version) {
  const releases = await github.paginate(github.rest.repos.listReleases, {
    ...context.repo,
    per_page: 100
  });
  const matches = releases.filter(
    (release) => release.tag_name === version && !release.draft
  );
  if (matches.length !== 1)
    throw new Error(`${version} is not a published release`);
}

export async function checkReleaseCandidate({ github, context, plan }) {
  if (plan.releaseType !== "final") return;
  const version = plan.previousVersion;
  if (
    !/^v\d+\.\d+\.0-rc\.[1-9]\d*$/.test(version) ||
    version.split("-rc.")[0] !== plan.version
  ) {
    throw new Error("A final release requires the matching last validated RC");
  }
  await publishedRelease(github, context, version);
  const sourceSha = await tagCommit(github, context, version);
  for (const task of coordinationTasks("rc", version)) {
    const deployment = await deploymentFor(
      github,
      context,
      version,
      sourceSha,
      task
    );
    const status =
      deployment && (await latestStatus(github, context, deployment));
    if (
      status?.state !== "success" ||
      deployment.payload?.version !== version ||
      deployment.payload?.sourceSha !== sourceSha ||
      deployment.payload?.task?.id !== task.id
    ) {
      throw new Error(
        `${version}: ${task.id} is not verified; resume that RC before preparing the final release`
      );
    }
  }
}

export async function coordinateTask({
  github,
  remote,
  context,
  task,
  version,
  sourceSha,
  deadline,
  now = Date.now,
  sleep = (milliseconds) =>
    new Promise((resolve) => setTimeout(resolve, milliseconds))
}) {
  const call = (route, parameters = {}) =>
    remote(route, {
      owner: context.repo.owner,
      repo: task.repo,
      ...parameters,
      headers: { "X-GitHub-Api-Version": apiVersion },
      request: {
        retries: route.startsWith("POST") ? 0 : 5,
        timeout: Math.max(1, Math.min(30000, deadline - now()))
      }
    });
  let deployment = await deploymentFor(
    github,
    context,
    version,
    sourceSha,
    task
  );
  if (!deployment) {
    const ref =
      task.ref || (await call("GET /repos/{owner}/{repo}")).data.default_branch;
    const request = {
      ...context.repo,
      ref: sourceSha,
      task: receiptTask(version, task),
      environment: receiptEnvironment,
      auto_merge: false,
      required_contexts: [],
      production_environment: false,
      transient_environment: false,
      payload: { version, sourceSha, task, ref },
      request: { retries: 0 }
    };
    try {
      ({ data: deployment } =
        await github.rest.repos.createDeployment(request));
    } catch (error) {
      deployment = await deploymentFor(
        github,
        context,
        version,
        sourceSha,
        task
      );
      if (!deployment) throw error;
    }
  }
  if (
    deployment.sha !== sourceSha ||
    deployment.payload?.version !== version ||
    !isDeepStrictEqual(deployment.payload?.task, task)
  ) {
    throw new Error(
      `Conflicting coordination payload for ${version} ${task.id}`
    );
  }
  const status = await latestStatus(github, context, deployment);
  const record = async (state, logUrl, description, environmentUrl) =>
    github.rest.repos.createDeploymentStatus({
      ...context.repo,
      deployment_id: deployment.id,
      state,
      auto_inactive: false,
      log_url: logUrl,
      description,
      ...(environmentUrl ? { environment_url: environmentUrl } : {})
    });
  const ref = deployment.payload.ref;
  const { data: workflow } = await call(
    "GET /repos/{owner}/{repo}/actions/workflows/{workflow_id}",
    { workflow_id: task.workflow }
  );
  let runID;
  if (status) {
    const prefix = `${context.serverUrl ?? "https://github.com"}/${context.repo.owner}/${task.repo}/actions/runs/`;
    if (
      !status.log_url?.startsWith(prefix) ||
      !/^\d+$/.test(status.log_url.slice(prefix.length))
    ) {
      throw new Error(
        `Dispatch outcome unknown for deployment ${deployment.id}; bind its exact remote run URL before resuming. No duplicate was dispatched.`
      );
    }
    runID = Number(status.log_url.slice(prefix.length));
  } else {
    await record(
      "queued",
      "",
      "Dispatch requested; missing run URL requires reconciliation"
    );
    const { data } = await call(
      "POST /repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches",
      {
        workflow_id: task.workflow,
        ref,
        inputs: task.inputs
      }
    );
    runID = data.workflow_run_id;
    if (!Number.isSafeInteger(runID) || runID < 1)
      throw new Error(
        "Workflow dispatch returned no run ID; do not redispatch"
      );
    await record(
      "in_progress",
      `${context.serverUrl ?? "https://github.com"}/${context.repo.owner}/${task.repo}/actions/runs/${runID}`,
      "Monitoring exact dispatched run"
    );
  }
  const getRun = async () => {
    const { data } = await call(
      "GET /repos/{owner}/{repo}/actions/runs/{run_id}",
      { run_id: runID }
    );
    if (
      data.workflow_id !== workflow.id ||
      data.head_branch !== ref ||
      data.event !== "workflow_dispatch"
    ) {
      throw new Error(
        `Run ${runID} does not match the recorded workflow and ref`
      );
    }
    return data;
  };
  let run = await getRun();
  let requiredAttempt =
    status?.description?.startsWith("Retry after attempt ") ?
      Number(status.description.slice(20)) + 1
    : 0;
  if (
    status &&
    status.state !== "success" &&
    run.status === "completed" &&
    run.conclusion !== "success" &&
    !requiredAttempt
  ) {
    requiredAttempt = run.run_attempt + 1;
    await record(
      "queued",
      run.html_url,
      `Retry after attempt ${run.run_attempt}`
    );
    await call(
      "POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs",
      { run_id: runID }
    );
  }
  while (run.status !== "completed" || run.run_attempt < requiredAttempt) {
    if (now() >= deadline)
      throw new Error(
        `Timed out monitoring ${run.html_url}; resume the same release`
      );
    await sleep(Math.min(10000, deadline - now()));
    run = await getRun();
  }
  if (run.conclusion !== "success") {
    await record(
      "failure",
      run.html_url,
      `Remote workflow concluded ${run.conclusion}`
    );
    throw new Error(`${task.id} failed: ${run.html_url}`);
  }
  let pullRequest = null;
  if (task.workflow === "upmerge.yaml") {
    pullRequest = await verifyUpmerge({
      call,
      context,
      task,
      run,
      status,
      record
    });
  }
  if (task.workflow === "release.yaml") {
    const channel = `v${version.slice(1).split(".").slice(0, 2).join(".")}`;
    const { data } = await call("GET /repos/{owner}/{repo}");
    if (data.default_branch !== channel)
      throw new Error(`${task.repo} did not publish channel ${channel}`);
    await call("GET /repos/{owner}/{repo}/git/ref/{ref}", {
      ref: `heads/${channel}`
    });
  }
  await record(
    "success",
    run.html_url,
    "Workflow and required destination verified",
    pullRequest
  );
  return {
    name: task.id,
    state: "success",
    runId: runID,
    url: run.html_url,
    ...(pullRequest ? { pullRequest } : {}),
    deploymentId: deployment.id
  };
}

export default async function coordinateRelease({
  github,
  remote = github.request,
  context,
  core,
  now = Date.now,
  sleep
}) {
  const plan = JSON.parse(
    await readFile(core.getInput("PLAN_FILE", { required: true }), "utf8")
  );
  if (
    !versionPattern.test(plan.version) ||
    !["rc", "final", "patch"].includes(plan.releaseType)
  )
    throw new Error("Invalid release coordination plan");
  if (core.getInput("MODE") === "check-rc") {
    await checkReleaseCandidate({ github, context, plan });
    return;
  }
  const sourceSha = core.getInput("SOURCE_SHA", { required: true });
  if (!/^[a-f0-9]{40}$/.test(sourceSha))
    throw new Error("SOURCE_SHA must be a full commit");
  await publishedRelease(github, context, plan.version);
  if ((await tagCommit(github, context, plan.version)) !== sourceSha)
    throw new Error("Release tag/source conflict");
  const deadline = now() + 2400000;
  const results = [];
  for (const task of coordinationTasks(plan.releaseType, plan.version)) {
    try {
      if (
        plan.releaseType === "rc" &&
        task.id === "sample-tests" &&
        results.some((result) => result.state !== "success")
      ) {
        throw new Error(
          "Sample tests wait for both upmerges to merge; resume the same RC afterward"
        );
      }
      if (now() >= deadline)
        throw new Error(
          "Coordination budget exhausted; resume the same release"
        );
      results.push(
        await coordinateTask({
          github,
          remote,
          context,
          task,
          version: plan.version,
          sourceSha,
          deadline,
          now,
          sleep
        })
      );
    } catch (error) {
      results.push({
        name: task.id,
        state: "incomplete",
        recovery: error.message
      });
    }
  }
  core.setOutput("results", JSON.stringify(results));
  if (results.some((result) => result.state !== "success")) {
    throw new Error(
      results
        .filter((result) => result.recovery)
        .map((result) => result.recovery)
        .join("\n")
    );
  }
}
