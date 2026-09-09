import { readFile } from "node:fs/promises";
import { isDeepStrictEqual } from "node:util";

const apiVersion = "2026-03-10";
const versionPattern = /^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/;

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
  const environment = `release-${version}-${task.id}`;
  const deployments = await github.paginate(github.rest.repos.listDeployments, {
    ...context.repo,
    environment,
    task: "release_coordination",
    per_page: 100
  });
  if (
    deployments.length > 1 ||
    deployments.some((entry) => entry.sha !== sourceSha)
  ) {
    throw new Error(`Conflicting coordination receipt for ${environment}`);
  }
  return { environment, deployment: deployments[0] };
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
    const { deployment } = await deploymentFor(
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
  let { environment, deployment } = await deploymentFor(
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
      task: "release_coordination",
      environment,
      auto_merge: false,
      required_contexts: [],
      production_environment: false,
      payload: { version, sourceSha, task, ref },
      request: { retries: 0 }
    };
    try {
      ({ data: deployment } =
        await github.rest.repos.createDeployment(request));
    } catch (error) {
      ({ deployment } = await deploymentFor(
        github,
        context,
        version,
        sourceSha,
        task
      ));
      if (!deployment) throw error;
    }
  }
  if (
    deployment.sha !== sourceSha ||
    deployment.payload?.version !== version ||
    !isDeepStrictEqual(deployment.payload?.task, task)
  ) {
    throw new Error(`Conflicting coordination payload for ${environment}`);
  }
  const status = await latestStatus(github, context, deployment);
  const record = async (state, logUrl, description) =>
    github.rest.repos.createDeploymentStatus({
      ...context.repo,
      deployment_id: deployment.id,
      state,
      auto_inactive: false,
      log_url: logUrl,
      description
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
  if (task.workflow === "upmerge.yaml") {
    const { data } = await call(
      "GET /repos/{owner}/{repo}/compare/{basehead}",
      { basehead: `edge...${run.head_sha}` }
    );
    if (data.ahead_by !== 0) {
      await record(
        "in_progress",
        run.html_url,
        "Waiting for the generated upmerge pull request to merge"
      );
      throw new Error(
        `Merge the ${task.repo} upmerge PR, then resume; ${run.html_url}`
      );
    }
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
    "Workflow and required destination verified"
  );
  return {
    name: task.id,
    state: "success",
    runId: runID,
    url: run.html_url,
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
