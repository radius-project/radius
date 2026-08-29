import assert from "node:assert/strict";
import test from "node:test";

import dispatchReleaseController from "./dispatch-release-controller.mjs";

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
