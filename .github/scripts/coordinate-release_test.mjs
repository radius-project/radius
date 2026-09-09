import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import coordinateRelease, {
  coordinationTasks,
  coordinateTask,
  checkReleaseCandidate
} from "./coordinate-release.mjs";

function fixture() {
  const sourceSha = "a".repeat(40);
  const task = coordinationTasks("rc", "v0.61.0-rc.1")[0];
  const deployments = [];
  const statuses = [];
  const calls = [];
  const run = {
    id: 77,
    workflow_id: 3,
    head_branch: "v0.60",
    event: "workflow_dispatch",
    head_sha: sourceSha,
    status: "completed",
    conclusion: "success",
    run_attempt: 1,
    html_url: "https://github.com/radius-project/docs/actions/runs/77"
  };
  const github = {
    rest: {
      repos: {
        listDeployments: async (request) =>
          deployments.filter(
            (entry) => entry.environment === request.environment
          ),
        createDeployment: async (request) => {
          const deployment = { ...request, sha: request.ref, id: 12 };
          deployments.push(deployment);
          return { data: deployment };
        },
        listDeploymentStatuses: async (request) =>
          statuses.filter(
            (entry) => entry.deployment_id === request.deployment_id
          ),
        createDeploymentStatus: async (request) => {
          statuses.push({ ...request, id: statuses.length + 1 });
          return { data: statuses.at(-1) };
        },
        listReleases: async () => [{ tag_name: "v0.61.0-rc.1", draft: false }]
      },
      git: {
        getRef: async () => ({
          data: { object: { type: "commit", sha: sourceSha } }
        })
      }
    },
    paginate: async (method, request) => method(request)
  };
  const remote = async (route, request) => {
    calls.push({ route, request });
    if (route.endsWith("/dispatches"))
      return { data: { workflow_run_id: 77, html_url: run.html_url } };
    if (route.endsWith("/rerun-failed-jobs")) {
      run.run_attempt++;
      run.conclusion = "success";
      return {};
    }
    if (route.endsWith("/{run_id}")) return { data: { ...run } };
    if (route.includes("/compare/")) return { data: { ahead_by: 0 } };
    if (route.endsWith("/{workflow_id}")) return { data: { id: 3 } };
    return { data: { default_branch: "v0.60" } };
  };
  return {
    github,
    remote,
    context: { repo: { owner: "radius-project", repo: "radius" } },
    task,
    version: "v0.61.0-rc.1",
    sourceSha,
    deadline: Date.now() + 10000,
    sleep: async () => {},
    deployments,
    statuses,
    calls,
    run
  };
}

test("uses native dispatch run IDs and resumes without duplicate work", async () => {
  const state = fixture();
  assert.equal((await coordinateTask(state)).runId, 77);
  await coordinateTask(state);
  const dispatches = state.calls.filter((call) =>
    call.route.endsWith("/dispatches")
  );
  assert.equal(dispatches.length, 1);
  assert.equal(
    dispatches[0].request.headers["X-GitHub-Api-Version"],
    "2026-03-10"
  );
  assert.equal(dispatches[0].request.request.retries, 0);
  assert.deepEqual(state.deployments[0].required_contexts, []);
  assert.equal(state.deployments[0].auto_merge, false);
});

test("uncertain dispatch fails closed and cannot be repeated by resume", async () => {
  const state = fixture();
  const remote = state.remote;
  state.remote = async (route, request) => {
    if (route.endsWith("/dispatches"))
      throw new Error("lost response after acceptance");
    return remote(route, request);
  };
  await assert.rejects(() => coordinateTask(state), /lost response/);
  state.remote = remote;
  await assert.rejects(() => coordinateTask(state), /outcome unknown/);
  assert.equal(
    state.calls.some((call) => call.route.endsWith("/dispatches")),
    false
  );
});

test("an open upmerge PR is not a successful final-release gate", async () => {
  const state = fixture();
  const remote = state.remote;
  state.remote = (route, request) =>
    route.includes("/compare/") ?
      Promise.resolve({ data: { ahead_by: 2 } })
    : remote(route, request);
  await assert.rejects(
    () => coordinateTask(state),
    /Merge the docs upmerge PR/
  );
  assert.equal(state.statuses.at(-1).state, "in_progress");
  state.remote = remote;
  await coordinateTask(state);
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/dispatches")).length,
    1
  );
});

test("failed downstream work retries the same run ID", async () => {
  const state = fixture();
  state.run.conclusion = "failure";
  await assert.rejects(() => coordinateTask(state), /docs-upmerge failed/);
  await coordinateTask(state);
  assert.equal(state.run.run_attempt, 2);
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/dispatches")).length,
    1
  );
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/rerun-failed-jobs"))
      .length,
    1
  );
});

test("rejects a conflicting source and a receipt pointing to another workflow", async () => {
  const state = fixture();
  await coordinateTask(state);
  state.sourceSha = "b".repeat(40);
  await assert.rejects(() => coordinateTask(state), /Conflicting/);
  state.sourceSha = "a".repeat(40);
  state.run.workflow_id = 88;
  await assert.rejects(() => coordinateTask(state), /does not match/);
});

test("final preparation rejects an RC without complete coordination receipts", async () => {
  const state = fixture();
  await assert.rejects(
    () =>
      checkReleaseCandidate({
        ...state,
        plan: {
          releaseType: "final",
          version: "v0.61.0",
          previousVersion: state.version
        }
      }),
    /not verified/
  );
  await checkReleaseCandidate({ ...state, plan: { releaseType: "patch" } });
  assert.deepEqual(
    coordinationTasks("patch", "v0.61.1").map((task) => task.id),
    ["sample-tests"]
  );
  assert.deepEqual(
    coordinationTasks("final", "v0.61.0").map((task) => task.id),
    ["docs-release", "samples-release", "sample-tests"]
  );
});

test("final preparation accepts all source-bound RC gates and rejects a later failure", async () => {
  const state = fixture();
  const plan = {
    releaseType: "final",
    version: "v0.61.0",
    previousVersion: state.version
  };
  for (const task of coordinationTasks("rc", state.version)) {
    const id = state.deployments.length + 1;
    state.deployments.push({
      id,
      environment: `release-${state.version}-${task.id}`,
      sha: state.sourceSha,
      payload: { task, version: state.version, sourceSha: state.sourceSha }
    });
    state.statuses.push({ id, deployment_id: id, state: "success" });
  }
  await checkReleaseCandidate({ ...state, plan });
  state.statuses.push({ id: 99, deployment_id: 3, state: "failure" });
  await assert.rejects(
    () => checkReleaseCandidate({ ...state, plan }),
    /sample-tests is not verified/
  );
});

test("monitor timeout retains exact run correlation for the next resume", async () => {
  const state = fixture();
  let clock = 0;
  state.deadline = 10;
  state.now = () => clock;
  state.sleep = async () => {
    clock += 10;
  };
  state.run.status = "in_progress";
  await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
  assert.equal(state.statuses.at(-1).log_url, state.run.html_url);
  state.run.status = "completed";
  await coordinateTask(state);
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/dispatches")).length,
    1
  );
});

test("sample tests are not dispatched while either RC upmerge is incomplete", async () => {
  const state = fixture();
  const directory = await mkdtemp(
    path.join(os.tmpdir(), "coordinate-release-")
  );
  try {
    const planFile = path.join(directory, "plan.json");
    await writeFile(
      planFile,
      JSON.stringify({ version: state.version, releaseType: "rc" })
    );
    const outputs = {};
    state.core = {
      getInput: (name) =>
        ({ PLAN_FILE: planFile, SOURCE_SHA: state.sourceSha })[name] ?? "",
      setOutput: (name, value) => {
        outputs[name] = value;
      }
    };
    const remote = state.remote;
    state.remote = (route, request) =>
      route.includes("/compare/") ?
        Promise.resolve({ data: { ahead_by: 1 } })
      : remote(route, request);
    await assert.rejects(() => coordinateRelease(state), /Sample tests wait/);
    assert.equal(
      state.calls.some(
        (call) =>
          call.route.endsWith("/dispatches") &&
          call.request.workflow_id === "test.yaml"
      ),
      false
    );
    assert.equal(
      JSON.parse(outputs.results).find(
        (result) => result.name === "sample-tests"
      ).state,
      "incomplete"
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
