import assert from "node:assert/strict";
import test from "node:test";

import dispatchReleaseController, {
  resumeReleasePublication
} from "./dispatch-release-controller.mjs";

function fixture() {
  const inputs = {
    EVENT_TYPE: "release-controller.resume",
    VERSION: "v0.61.0-rc.1",
    SOURCE_SHA: "a".repeat(40)
  };
  const dispatches = [];
  const outputs = {};
  return {
    inputs,
    dispatches,
    outputs,
    github: {
      rest: {
        repos: {
          createDispatchEvent: async (request) => dispatches.push(request)
        }
      }
    },
    context: {
      actor: "release-engineer",
      repo: { owner: "radius-project", repo: "radius" }
    },
    core: {
      getInput: (name) => inputs[name] ?? "",
      setOutput: (name, value) => {
        outputs[name] = value;
      }
    }
  };
}

test("dispatches a source-bound resume request", async () => {
  const state = fixture();
  await dispatchReleaseController(state);
  assert.deepEqual(state.dispatches, [
    {
      owner: "radius-project",
      repo: "radius",
      event_type: "release-controller.resume",
      client_payload: {
        version: "v0.61.0-rc.1",
        source_commit: "a".repeat(40),
        requested_by: "release-engineer"
      }
    }
  ]);
  assert.equal(
    state.outputs["release-identifier"],
    `v0.61.0-rc.1-${"a".repeat(40)}`
  );
});

test("dispatches an explicit approval request", async () => {
  const state = fixture();
  state.inputs.EVENT_TYPE = "release-controller.approve";
  await dispatchReleaseController(state);
  assert.equal(state.dispatches[0].event_type, "release-controller.approve");
});

test("rejects malformed requests before dispatch", async () => {
  const state = fixture();
  state.inputs.SOURCE_SHA = "main";
  await assert.rejects(
    () => dispatchReleaseController(state),
    /SOURCE_SHA must be a full commit SHA/
  );
  assert.deepEqual(state.dispatches, []);
});

test("resumes failed publication jobs without creating another tag or dispatch", async () => {
  const state = fixture();
  const reruns = [];
  const run = {
    id: 77,
    head_sha: state.inputs.SOURCE_SHA,
    head_branch: state.inputs.VERSION,
    event: "push",
    status: "completed",
    conclusion: "failure",
    html_url: "https://example.test/run/77"
  };
  state.github.paginate = async (method) => method();
  state.github.rest.actions = {
    listWorkflowRuns: async () => [run],
    reRunWorkflowFailedJobs: async (request) => reruns.push(request)
  };
  await resumeReleasePublication(state);
  assert.equal(reruns[0].run_id, 77);
  assert.equal(state.outputs["run-url"], run.html_url);
  assert.deepEqual(state.dispatches, []);
  run.status = "in_progress";
  await resumeReleasePublication(state);
  assert.equal(reruns.length, 1);
});

for (const event of ["push", "workflow_dispatch"]) {
  test(`publication resume discovers a delayed ${event} run`, async () => {
    const state = fixture();
    let clock = 0;
    const intervals = [];
    const run = {
      id: 78,
      head_sha: state.inputs.SOURCE_SHA,
      head_branch: state.inputs.VERSION,
      event,
      status: "queued",
      html_url: "https://example.test/run/78"
    };
    state.now = () => clock;
    state.sleep = async (milliseconds) => {
      intervals.push(milliseconds);
      clock += milliseconds;
    };
    state.github.paginate = async () => (clock >= 120000 ? [run] : []);
    state.github.rest.actions = { listWorkflowRuns() {} };

    await resumeReleasePublication(state);

    assert.equal(clock, 120000);
    assert.deepEqual(intervals, Array(12).fill(10000));
    assert.equal(state.outputs["run-url"], run.html_url);
    assert.deepEqual(state.dispatches, []);
  });
}

test("publication resume rejects other tags at the same commit", async () => {
  const state = fixture();
  let clock = 0;
  const intervals = [];
  state.now = () => clock;
  state.sleep = async (milliseconds) => {
    intervals.push(milliseconds);
    clock += milliseconds;
  };
  state.github.paginate = async () => [
    { head_sha: state.inputs.SOURCE_SHA, head_branch: "v0.60.0", event: "push" }
  ];
  state.github.rest.actions = { listWorkflowRuns() {} };
  await assert.rejects(() => resumeReleasePublication(state), {
    message: `No tag-build run found for ${state.inputs.VERSION} after five minutes; the tag exists. Start it with gh workflow run build-release.yaml --ref ${state.inputs.VERSION}, then resume`
  });
  assert.equal(clock, 300000);
  assert.deepEqual(intervals, Array(30).fill(10000));
  assert.equal(state.outputs["run-url"], undefined);
  assert.deepEqual(state.dispatches, []);
});
