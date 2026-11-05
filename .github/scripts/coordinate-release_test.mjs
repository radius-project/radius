import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import coordinateRelease, {
  coordinationTasks,
  coordinateTask,
  checkReleaseCandidate
} from "./coordinate-release.mjs";

for (const [releaseType, version, expected] of [
  [
    "rc",
    "v0.62.0-rc.2",
    [
      ["docs", "upmerge.yaml", undefined],
      ["samples", "upmerge.yaml", undefined],
      ["samples", "test.yaml", "edge"]
    ]
  ],
  [
    "final",
    "v0.62.0",
    [
      ["docs", "release.yaml", "edge"],
      ["samples", "release.yaml", "edge"],
      ["samples", "test.yaml", "edge"]
    ]
  ],
  ["patch", "v0.62.1", [["samples", "test.yaml", "edge"]]]
]) {
  test(`${releaseType} coordinates only its intended downstream workflows`, () => {
    const tasks = coordinationTasks(releaseType, version);
    assert.deepEqual(
      tasks.map(({ repo, workflow, ref }) => [repo, workflow, ref]),
      expected
    );
    for (const task of tasks) {
      assert.deepEqual(
        task.inputs,
        task.workflow === "upmerge.yaml" ? {} : { version: version.slice(1) }
      );
    }
  });
}

test("workflow uses the App client remotely and the job client for receipts", async (t) => {
  const workflow = (
    await readFile(
      new URL("../workflows/build-release.yaml", import.meta.url),
      "utf8"
    )
  ).replace(/\r\n/g, "\n");
  const step = workflow.match(
    /        id: coordinate\n[\s\S]*?          script: \|\n([\s\S]*?)(?=\n      - name:)/
  );
  assert.ok(step, "the downstream coordination step must exist");
  assert.match(step[0], /          retries: 5\n/);
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const runStep = new AsyncFunction(
    "github",
    "getOctokit",
    "context",
    "core",
    "process",
    step[1].replace(/^            /gm, "")
  );

  const state = fixture();
  const version = "v0.62.1";
  state.run.head_branch = "edge";
  state.run.html_url =
    "https://github.com/radius-project/samples/actions/runs/77";
  state.github.rest.repos.listReleases = async () => [
    { tag_name: version, draft: false }
  ];
  state.github.request = Object.assign(
    async () => assert.fail("the job client must not call remote repositories"),
    {
      defaults: () =>
        assert.fail(
          "request defaults cannot replace the job client's authentication"
        )
    }
  );
  const directory = await mkdtemp(path.join(os.tmpdir(), "coordination-auth-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const planFile = path.join(directory, "plan.json");
  await writeFile(planFile, JSON.stringify({ version, releaseType: "patch" }));
  const outputs = {};
  const core = {
    getInput: (name) =>
      ({ PLAN_FILE: planFile, SOURCE_SHA: state.sourceSha })[name] ?? "",
    setOutput: (name, value) => {
      outputs[name] = value;
    }
  };
  const tokens = [];
  const getOctokit = (token) => {
    tokens.push(token);
    return { request: state.remote };
  };
  await runStep(state.github, getOctokit, state.context, core, {
    env: {
      COORDINATION_TOKEN: "test-app-token",
      GITHUB_WORKSPACE: new URL("../../", import.meta.url).href.replace(
        /\/$/,
        ""
      )
    }
  });
  assert.deepEqual(tokens, ["test-app-token"]);
  assert.equal(state.deployments.length, 1);
  assert.equal(state.deployments[0].repo, "radius");
  assert.ok(state.statuses.length > 0);
  assert.ok(state.statuses.every(({ repo }) => repo === "radius"));
  assert.ok(state.calls.length > 0);
  assert.ok(state.calls.every(({ request }) => request.repo === "samples"));
  const dispatch = state.calls.find(({ route }) =>
    route.endsWith("/dispatches")
  );
  assert.equal(dispatch.request.request.retries, 0);
  assert.ok(
    state.calls
      .filter(({ route }) => route.startsWith("GET "))
      .every(({ request }) => request.request.retries === 5)
  );
  assert.equal(JSON.parse(outputs.results)[0].state, "success");
});

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
  // The upmerge run opened one pull request while its pull-request step ran.
  const step = {
    name: "Create pull request",
    conclusion: "success",
    started_at: "2026-09-10T10:00:00Z",
    completed_at: "2026-09-10T10:00:03Z"
  };
  const pulls = [
    {
      number: 5,
      html_url: "https://github.com/radius-project/docs/pull/5",
      created_at: "2026-09-10T09:30:00Z",
      state: "closed",
      merged: true,
      base: { ref: "edge" },
      head: { ref: "upmerge/2026-09-10-1a2b" }
    },
    {
      number: 6,
      html_url: "https://github.com/radius-project/docs/pull/6",
      created_at: "2026-09-10T10:00:02Z",
      state: "closed",
      merged: true,
      base: { ref: "edge" },
      head: { ref: "upmerge/2026-09-10-3c4d" }
    }
  ];
  const github = {
    rest: {
      repos: {
        listDeployments: async (request) =>
          deployments.filter(
            (entry) =>
              entry.environment === request.environment &&
              entry.task === request.task
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
    if (route.endsWith("/rerun")) {
      run.run_attempt++;
      step.started_at = new Date(
        Date.parse(step.started_at) + 60000
      ).toISOString();
      step.completed_at = new Date(
        Date.parse(step.completed_at) + 60000
      ).toISOString();
      const number = pulls.at(-1).number + 1;
      pulls.push({
        number,
        html_url: `https://github.com/radius-project/docs/pull/${number}`,
        created_at: step.started_at,
        state: "open",
        merged: false,
        base: { ref: "edge" },
        head: { ref: `upmerge/${run.id}-${run.run_attempt}` }
      });
      return {};
    }
    if (route.endsWith("/{run_id}")) return { data: { ...run } };
    if (route.endsWith("/{run_id}/jobs"))
      return { data: { jobs: [{ steps: [{ ...step }] }] } };
    if (route.endsWith("/pulls")) return { data: pulls.map((p) => ({ ...p })) };
    if (route.endsWith("/{pull_number}"))
      return {
        data: { ...pulls.find((p) => p.number === request.pull_number) }
      };
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
    run,
    step,
    pulls
  };
}

function sampleFixture(releaseType = "rc", version = "v0.61.0-rc.1") {
  const state = fixture();
  state.task = coordinationTasks(releaseType, version).at(-1);
  state.version = version;
  state.samplesHead = "b".repeat(40);
  state.nextRunStatus = "completed";
  state.nextRunConclusion = "failure";
  state.runs = [];
  const remote = state.remote;
  state.remote = async (route, request) => {
    if (route.endsWith("/dispatches")) {
      state.calls.push({ route, request });
      assert.equal(state.statuses.at(-1).state, "queued");
      assert.equal(state.statuses.at(-1).log_url, "");
      assert.equal(request.workflow_id, "test.yaml");
      assert.equal(request.ref, "edge");
      assert.deepEqual(request.inputs, { version: version.slice(1) });
      assert.equal(request.request.retries, 0);
      const id = 77 + state.runs.length;
      state.runs.push({
        ...state.run,
        id,
        head_branch: "edge",
        head_sha: state.samplesHead,
        status: state.nextRunStatus,
        conclusion: state.nextRunConclusion,
        html_url: `https://github.com/radius-project/samples/actions/runs/${id}`
      });
      return { data: { workflow_run_id: id } };
    }
    if (route.endsWith("/{run_id}")) {
      state.calls.push({ route, request });
      const run = state.runs.find((entry) => entry.id === request.run_id);
      assert.ok(run, `unknown sample-test run ${request.run_id}`);
      return { data: { ...run } };
    }
    assert.ok(
      !route.endsWith("/rerun") && !route.endsWith("/rerun-failed-jobs"),
      "sample-test retries must dispatch a new complete workflow"
    );
    return remote(route, request);
  };
  return state;
}

for (const [releaseType, version] of [
  ["rc", "v0.61.0-rc.1"],
  ["final", "v0.61.0"],
  ["patch", "v0.61.1"]
]) {
  test(`${releaseType} sample-test retries dispatch current edge on the same receipt`, async () => {
    for (const conclusion of ["failure", "cancelled", "timed_out"]) {
      const state = sampleFixture(releaseType, version);
      state.nextRunConclusion = conclusion;
      await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
      assert.equal(state.runs.length, 1, "initial failure must not loop");
      const receipt = structuredClone(state.deployments[0]);
      const originalRun = { ...state.runs[0] };
      state.samplesHead = "c".repeat(40);
      state.nextRunConclusion = "success";

      const result = await coordinateTask(state);

      assert.equal(result.runId, 78);
      assert.equal(result.samplesCommit, state.samplesHead);
      assert.deepEqual(state.runs[0], originalRun);
      assert.equal(state.runs[1].run_attempt, 1);
      assert.deepEqual(state.deployments, [receipt]);
      assert.equal(receipt.sha, state.sourceSha);
      assert.equal(receipt.payload.version, version);
      assert.equal(state.statuses.at(-1).state, "success");
      assert.equal(state.statuses.at(-1).log_url, state.runs[1].html_url);
      assert.equal(result.deploymentId, receipt.id);
      assert.equal(
        state.calls.filter(({ route }) => route.endsWith("/dispatches")).length,
        2
      );
      assert.ok(
        state.statuses.some(
          (status) =>
            status.state === "failure" &&
            status.log_url === originalRun.html_url
        )
      );
    }
  });
}

test("a fresh sample-test failure waits for the next resume before redispatching", async () => {
  const state = sampleFixture();
  for (const runCount of [1, 2, 3]) {
    await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
    assert.equal(state.runs.length, runCount);
    assert.equal(state.statuses.at(-1).log_url, state.runs.at(-1).html_url);
  }
});

test("pending and successful sample-test runs are reused even when edge advances", async () => {
  for (const status of ["queued", "in_progress"]) {
    const state = sampleFixture();
    state.nextRunStatus = status;
    state.nextRunConclusion = null;
    let clock = 0;
    state.now = () => clock;
    state.deadline = 10;
    state.sleep = async (milliseconds) => {
      clock += milliseconds;
    };
    await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
    state.samplesHead = "c".repeat(40);
    clock = 0;
    await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
    assert.equal(state.runs.length, 1);

    state.runs[0].status = "completed";
    state.runs[0].conclusion = "success";
    const result = await coordinateTask(state);
    assert.equal(result.runId, 77);
    assert.equal(result.samplesCommit, "b".repeat(40));
    await coordinateTask(state);
    assert.equal(state.runs.length, 1, "a completed gate must not be repeated");
  }
});

test("a timed-out replacement remains bound for the next resume", async () => {
  const state = sampleFixture();
  await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
  state.samplesHead = "c".repeat(40);
  state.nextRunStatus = "in_progress";
  state.nextRunConclusion = null;
  let clock = 0;
  state.now = () => clock;
  state.deadline = 10;
  state.sleep = async (milliseconds) => {
    clock += milliseconds;
  };
  await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
  assert.equal(state.statuses.at(-1).log_url, state.runs[1].html_url);
  state.runs[1].status = "completed";
  state.runs[1].conclusion = "success";
  const result = await coordinateTask(state);
  assert.equal(result.runId, 78);
  assert.equal(result.samplesCommit, "c".repeat(40));
  assert.equal(state.runs.length, 2);
});

test("an explicitly rejected sample-test replacement can be dispatched on resume", async () => {
  for (const status of [403, 429]) {
    const state = sampleFixture();
    await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
    const remote = state.remote;
    state.remote = async (route, request) => {
      if (route.endsWith("/dispatches")) {
        state.calls.push({ route, request });
        throw Object.assign(new Error("replacement rejected"), { status });
      }
      return remote(route, request);
    };
    await assert.rejects(() => coordinateTask(state), /replacement rejected/);
    assert.equal(state.statuses.at(-1).state, "failure");
    assert.equal(state.statuses.at(-1).log_url, "");
    assert.match(
      state.statuses.at(-1).description,
      /Dispatch rejected with HTTP/
    );
    assert.equal(state.runs.length, 1);

    state.remote = remote;
    state.samplesHead = "c".repeat(40);
    state.nextRunConclusion = "success";
    const result = await coordinateTask(state);
    assert.equal(result.runId, 78);
    assert.equal(result.samplesCommit, "c".repeat(40));
    assert.equal(state.deployments.length, 1);
    assert.equal(
      state.calls.filter(({ route }) => route.endsWith("/dispatches")).length,
      3
    );
  }
});

test("uncertain sample-test replacement outcomes stop without another dispatch", async () => {
  for (const mode of [
    "lost-response",
    "server-error",
    "missing-run-id",
    "receipt-write"
  ]) {
    const state = sampleFixture();
    await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
    const remote = state.remote;
    const record = state.github.rest.repos.createDeploymentStatus;
    state.nextRunConclusion = "success";
    state.remote = async (route, request) => {
      const result = await remote(route, request);
      if (route.endsWith("/dispatches")) {
        if (mode === "lost-response") throw new Error("lost dispatch response");
        if (mode === "server-error")
          throw Object.assign(new Error("dispatch server error"), {
            status: 500
          });
        if (mode === "missing-run-id") return { data: {} };
      }
      return result;
    };
    if (mode === "receipt-write") {
      state.github.rest.repos.createDeploymentStatus = async (request) => {
        if (request.log_url?.endsWith("/78"))
          throw new Error("receipt unavailable");
        return record(request);
      };
    }
    await assert.rejects(
      () => coordinateTask(state),
      /lost dispatch response|dispatch server error|no run ID|receipt unavailable/
    );
    assert.equal(state.runs.length, 2, mode);
    assert.equal(state.statuses.at(-1).state, "queued");
    assert.equal(state.statuses.at(-1).log_url, "");
    state.remote = remote;
    state.github.rest.repos.createDeploymentStatus = record;
    await assert.rejects(
      () => coordinateTask(state),
      /Dispatch outcome unknown/
    );
    assert.equal(state.runs.length, 2, mode);
  }
});

test("an unresolved legacy sample-test rerun marker is not replaced blindly", async () => {
  const state = sampleFixture();
  await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
  state.statuses.push({
    id: state.statuses.length + 1,
    deployment_id: state.deployments[0].id,
    state: "queued",
    log_url: state.runs[0].html_url,
    description: "Retry after attempt 1"
  });
  let clock = 0;
  state.now = () => clock;
  state.deadline = 10;
  state.sleep = async (milliseconds) => {
    clock += milliseconds;
  };
  await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
  assert.equal(state.runs.length, 1);

  state.runs[0].run_attempt = 2;
  state.samplesHead = "c".repeat(40);
  state.nextRunConclusion = "success";
  const result = await coordinateTask(state);
  assert.equal(result.runId, 78);
  assert.equal(result.samplesCommit, "c".repeat(40));
});

test("replacement sample tests require the recorded workflow, event, ref and a commit", async () => {
  for (const mismatch of [
    { workflow_id: 99 },
    { event: "push" },
    { head_branch: "v0.61" },
    { head_sha: "edge" },
    { head_sha: null }
  ]) {
    const state = sampleFixture();
    await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
    state.nextRunConclusion = "success";
    const remote = state.remote;
    state.remote = async (route, request) => {
      const result = await remote(route, request);
      if (route.endsWith("/{run_id}") && request.run_id === 78)
        Object.assign(result.data, mismatch);
      return result;
    };
    await assert.rejects(
      () => coordinateTask(state),
      Object.hasOwn(mismatch, "head_sha") ?
        /no valid source commit/
      : /does not match/
    );
    assert.notEqual(state.statuses.at(-1).state, "success");
    assert.equal(state.runs.length, 2);
  }
});

test("an accepted replacement receipt is reused after its response is lost", async () => {
  const state = sampleFixture();
  await assert.rejects(() => coordinateTask(state), /sample-tests failed/);
  state.samplesHead = "c".repeat(40);
  state.nextRunConclusion = "success";
  const record = state.github.rest.repos.createDeploymentStatus;
  state.github.rest.repos.createDeploymentStatus = async (request) => {
    const result = await record(request);
    if (request.log_url?.endsWith("/78"))
      throw new Error("lost receipt response");
    return result;
  };
  await assert.rejects(() => coordinateTask(state), /lost receipt response/);
  state.github.rest.repos.createDeploymentStatus = record;
  const result = await coordinateTask(state);
  assert.equal(result.runId, 78);
  assert.equal(result.samplesCommit, "c".repeat(40));
  assert.equal(state.runs.length, 2);
  assert.equal(state.deployments.length, 1);
});

test("uses native dispatch run IDs and resumes without duplicate work", async () => {
  const state = fixture();
  const result = await coordinateTask(state);
  assert.equal(result.runId, 77);
  assert.equal(result.pullRequest, state.pulls[1].html_url);
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
  assert.equal(state.deployments[0].environment, "release-coordination");
  assert.equal(
    state.deployments[0].task,
    "release-coordination:v0.61.0-rc.1:docs-upmerge"
  );
  assert.equal(state.statuses.at(-1).environment_url, state.pulls[1].html_url);
  // The bound pull request is read directly; the list is searched only once.
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/pulls")).length,
    1
  );
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

test("an explicitly rejected dispatch can be dispatched again on resume", async () => {
  for (const status of [403, 429]) {
    const state = fixture();
    const remote = state.remote;
    state.remote = async (route, request) => {
      if (route.endsWith("/dispatches")) {
        state.calls.push({ route, request });
        throw Object.assign(new Error("dispatch rejected"), { status });
      }
      return remote(route, request);
    };
    await assert.rejects(() => coordinateTask(state), /dispatch rejected/);
    assert.equal(state.statuses.at(-1).state, "failure", `${status}`);
    state.remote = remote;
    const result = await coordinateTask(state);
    assert.equal(result.runId, 77, `${status}`);
    assert.equal(
      state.calls.filter((call) => call.route.endsWith("/dispatches")).length,
      2,
      `${status}`
    );
  }
});

test("an open upmerge PR is not a successful final-release gate", async () => {
  const state = fixture();
  state.pulls[1].state = "open";
  state.pulls[1].merged = false;
  await assert.rejects(
    () => coordinateTask(state),
    /Merge the docs upmerge PR https:\/\/github.com\/radius-project\/docs\/pull\/6/
  );
  assert.equal(state.statuses.at(-1).state, "in_progress");
  assert.equal(state.statuses.at(-1).environment_url, state.pulls[1].html_url);
  // A squash merge leaves the source commits unreachable from edge; only the
  // bound pull request's merge state decides.
  state.pulls[1].state = "closed";
  state.pulls[1].merged = true;
  await coordinateTask(state);
  assert.equal(state.statuses.at(-1).state, "success");
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/dispatches")).length,
    1
  );
  assert.equal(
    state.calls.filter((call) => call.route.endsWith("/pulls")).length,
    1
  );
});

test("an upmerge run that had nothing to merge is complete without a pull request", async () => {
  const state = fixture();
  state.step.conclusion = "skipped";
  const result = await coordinateTask(state);
  assert.equal(result.state, "success");
  assert.equal(result.pullRequest, undefined);
  assert.equal(
    state.calls.some((call) => call.route.endsWith("/pulls")),
    false
  );
});

test("an upmerge pull request that cannot be bound exactly stops for reconciliation", async () => {
  const state = fixture();
  state.pulls[0].created_at = state.pulls[1].created_at;
  await assert.rejects(
    () => coordinateTask(state),
    /2 upmerge pull requests match/
  );
  assert.notEqual(state.statuses.at(-1).state, "success");
  // An operator binding on the receipt is honored on resume.
  state.statuses.push({
    id: 50,
    deployment_id: state.deployments[0].id,
    state: "in_progress",
    log_url: state.run.html_url,
    environment_url: state.pulls[1].html_url
  });
  const result = await coordinateTask(state);
  assert.equal(result.pullRequest, state.pulls[1].html_url);
});

test("a closed unmerged upmerge reruns all jobs and binds the replacement PR", async () => {
  const state = fixture();
  state.pulls[1].state = "open";
  state.pulls[1].merged = false;
  await assert.rejects(
    () => coordinateTask(state),
    /Merge the docs upmerge PR/
  );
  const original = state.pulls[1];
  original.state = "closed";

  await assert.rejects(() => coordinateTask(state), /upmerge PR .*\/pull\/7/);
  const replacement = state.pulls.at(-1);
  assert.equal(original.state, "closed");
  assert.equal(original.merged, false);
  assert.equal(state.run.run_attempt, 2);
  assert.equal(state.deployments.length, 1);
  assert.equal(state.statuses.at(-1).state, "in_progress");
  assert.equal(state.statuses.at(-1).environment_url, replacement.html_url);
  assert.equal(
    state.statuses.find(
      (status) => status.description === "Retry after attempt 1"
    ).environment_url,
    ""
  );
  replacement.state = "closed";
  replacement.merged = true;
  const result = await coordinateTask(state);
  assert.equal(result.pullRequest, replacement.html_url);
  assert.equal(state.statuses.at(-1).state, "success");
  assert.equal(
    state.calls.filter(({ route }) => route.endsWith("/dispatches")).length,
    1
  );
  assert.equal(
    state.calls.filter(({ route }) => route.endsWith("/rerun")).length,
    1
  );
  assert.equal(
    state.calls.filter(({ route }) => route.endsWith("/rerun-failed-jobs"))
      .length,
    0
  );
});

test("a replacement upmerge with no changes clears the closed PR binding", async () => {
  const state = fixture();
  state.pulls[1].state = "open";
  state.pulls[1].merged = false;
  await assert.rejects(
    () => coordinateTask(state),
    /Merge the docs upmerge PR/
  );
  state.pulls[1].state = "closed";
  const remote = state.remote;
  state.remote = async (route, request) => {
    const result = await remote(route, request);
    if (route.endsWith("/rerun")) state.step.conclusion = "skipped";
    return result;
  };
  const result = await coordinateTask(state);
  assert.equal(result.state, "success");
  assert.equal(result.pullRequest, undefined);
  assert.equal(state.statuses.at(-1).environment_url, "");
});

test("rejected replacements preserve bound PRs and retry without redispatching", async () => {
  for (const [status, manuallyBound] of [
    [403, true],
    [429, true],
    [403, false],
    [429, false]
  ]) {
    const state = fixture();
    state.pulls[1].state = "open";
    state.pulls[1].merged = false;
    if (manuallyBound) {
      state.pulls[0].created_at = state.pulls[1].created_at;
      await assert.rejects(
        () => coordinateTask(state),
        /2 upmerge pull requests match/
      );
      state.statuses.push({
        id: state.statuses.length + 1,
        deployment_id: state.deployments[0].id,
        state: "in_progress",
        log_url: state.run.html_url,
        environment_url: state.pulls[1].html_url
      });
    }
    await assert.rejects(
      () => coordinateTask(state),
      /Merge the docs upmerge PR/
    );
    state.pulls[1].state = "closed";
    const remote = state.remote;
    state.remote = async (route, request) => {
      if (route.endsWith("/rerun")) {
        state.calls.push({ route, request });
        throw Object.assign(new Error("replacement rejected"), { status });
      }
      return remote(route, request);
    };
    await assert.rejects(() => coordinateTask(state), /replacement rejected/);
    const rejection = state.statuses.at(-1);
    assert.equal(state.run.run_attempt, 1);
    assert.equal(
      state.statuses.find(
        (entry) => entry.description === "Retry after attempt 1"
      ).environment_url,
      ""
    );
    state.remote = remote;
    await assert.rejects(() => coordinateTask(state), /upmerge PR .*\/pull\/7/);
    assert.equal(rejection.state, "failure");
    assert.equal(rejection.environment_url, state.pulls[1].html_url);
    assert.equal(state.run.run_attempt, 2);
    assert.equal(
      state.statuses.at(-1).environment_url,
      state.pulls[2].html_url
    );
    assert.equal(
      state.calls.filter(({ route }) => route.endsWith("/pulls")).length,
      2,
      "resume must not rediscover the original bound PR"
    );
    assert.equal(
      state.calls.filter(({ route }) => route.endsWith("/dispatches")).length,
      1
    );
    assert.equal(
      state.calls.filter(({ route }) => route.endsWith("/rerun")).length,
      2
    );
  }
});

test("an uncertain replacement request is not repeated on resume", async () => {
  const state = fixture();
  state.pulls[1].state = "open";
  state.pulls[1].merged = false;
  await assert.rejects(
    () => coordinateTask(state),
    /Merge the docs upmerge PR/
  );
  state.pulls[0].created_at = state.pulls[1].created_at;
  state.pulls[1].state = "closed";
  const remote = state.remote;
  state.remote = async (route, request) => {
    if (route.endsWith("/rerun")) {
      state.calls.push({ route, request });
      throw new Error("lost replacement response");
    }
    return remote(route, request);
  };
  await assert.rejects(
    () => coordinateTask(state),
    /lost replacement response/
  );
  assert.equal(state.statuses.at(-1).description, "Retry after attempt 1");
  assert.equal(state.statuses.at(-1).environment_url, "");
  let clock = 0;
  state.now = () => clock;
  state.deadline = 10;
  state.sleep = async () => {
    clock += 10;
  };
  await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
  assert.equal(
    state.calls.filter(({ route }) => route.endsWith("/rerun")).length,
    1
  );

  state.step.started_at = "2026-09-10T10:01:00Z";
  state.step.completed_at = "2026-09-10T10:01:03Z";
  state.run.run_attempt = 2;
  state.pulls.push({
    ...state.pulls[1],
    number: 7,
    html_url: "https://github.com/radius-project/docs/pull/7",
    created_at: "2026-09-10T10:01:01Z",
    state: "open",
    head: { ref: "upmerge/77-2" }
  });
  await assert.rejects(() => coordinateTask(state), /upmerge PR .*\/pull\/7/);
  assert.equal(
    state.calls.filter(({ route }) => route.endsWith("/rerun")).length,
    1
  );
});

test("a replacement closed immediately requires another resume rather than a loop", async () => {
  const state = fixture();
  state.pulls[1].state = "closed";
  state.pulls[1].merged = false;
  const remote = state.remote;
  state.remote = async (route, request) => {
    const result = await remote(route, request);
    if (route.endsWith("/rerun")) state.pulls.at(-1).state = "closed";
    return result;
  };
  await assert.rejects(
    () => coordinateTask(state),
    /replacement upmerge PR closed/
  );
  assert.equal(
    state.calls.filter(({ route }) => route.endsWith("/rerun")).length,
    1
  );
  assert.equal(state.statuses.at(-1).state, "failure");
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

test("an explicitly rejected rerun can be requested again on resume", async () => {
  for (const status of [403, 429]) {
    const state = fixture();
    const remote = state.remote;
    state.run.conclusion = "failure";
    await assert.rejects(() => coordinateTask(state), /docs-upmerge failed/);
    state.remote = async (route, request) => {
      if (route.endsWith("/rerun-failed-jobs")) {
        state.calls.push({ route, request });
        throw Object.assign(new Error("rerun rejected"), { status });
      }
      return remote(route, request);
    };
    await assert.rejects(() => coordinateTask(state), /rerun rejected/);
    assert.equal(state.statuses.at(-1).state, "failure", `${status}`);
    state.remote = remote;
    await coordinateTask(state);
    assert.equal(state.run.run_attempt, 2, `${status}`);
    assert.equal(
      state.calls.filter((call) => call.route.endsWith("/rerun-failed-jobs"))
        .length,
      2,
      `${status}`
    );
  }
});

test("an uncertain rerun outcome is never requested twice", async () => {
  const state = fixture();
  const remote = state.remote;
  let clock = 0;
  state.deadline = 10;
  state.now = () => clock;
  state.sleep = async () => {
    clock += 10;
  };
  state.run.conclusion = "failure";
  await assert.rejects(() => coordinateTask(state), /docs-upmerge failed/);
  state.remote = async (route, request) => {
    if (route.endsWith("/rerun-failed-jobs")) {
      state.calls.push({ route, request });
      throw new Error("lost response after acceptance");
    }
    return remote(route, request);
  };
  await assert.rejects(() => coordinateTask(state), /lost response/);
  assert.equal(state.statuses.at(-1).description, "Retry after attempt 1");
  await assert.rejects(() => coordinateTask(state), /Timed out monitoring/);
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
      environment: "release-coordination",
      task: `release-coordination:${state.version}:${task.id}`,
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
    state.pulls[1].state = "open";
    state.pulls[1].merged = false;
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
