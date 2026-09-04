import assert from "node:assert/strict";
import test from "node:test";

import resolveReleaseController from "./resolve-release-controller.mjs";

const repository = "radius-project/radius";
const sourceCommit = "a".repeat(40);
const backportCommit = "b".repeat(40);

function releasePull(overrides = {}) {
  return {
    number: 42,
    body: "approved release plan",
    base: { ref: "main" },
    head: {
      ref: "automation/prepare-release-0.61.0-rc.1",
      repo: { full_name: repository }
    },
    merged: true,
    merged_at: "2026-08-28T12:00:00Z",
    merge_commit_sha: sourceCommit,
    ...overrides
  };
}

function fixture({ mode = "event", pull, pulls = [] } = {}) {
  const inputs = {
    MODE: mode,
    VERSION: "v0.61.0-rc.1",
    SOURCE_SHA: sourceCommit
  };
  const outputs = {};
  const context = {
    repo: { owner: "radius-project", repo: "radius" },
    payload: { pull_request: pull }
  };
  const github = {
    paginate: async (method, request) => method(request),
    rest: {
      pulls: {
        list: async () => pulls,
        get: async ({ pull_number }) => ({
          data: pulls.find((candidate) => candidate.number === pull_number)
        })
      }
    }
  };
  const core = {
    getInput: (name) => inputs[name] ?? "",
    setOutput: (name, value) => {
      outputs[name] = value;
    }
  };
  return { context, core, github, inputs, outputs };
}

test("selects a merged generated release pull request", async () => {
  const state = fixture({ pull: releasePull() });
  await resolveReleaseController(state);
  assert.deepEqual(state.outputs, {
    selected: "true",
    "source-pr-number": "42",
    "source-pr-commit": sourceCommit,
    version: "v0.61.0-rc.1",
    "release-commit": sourceCommit,
    trigger: "release-pr"
  });
});

test("selects the generated release metadata backport", async () => {
  const source = releasePull();
  const state = fixture({
    pull: {
      number: 43,
      base: { ref: "release/0.61" },
      head: {
        ref: "automation/backport-42-to-0.61",
        repo: { full_name: repository }
      },
      merged: true,
      merge_commit_sha: backportCommit
    },
    pulls: [source]
  });
  await resolveReleaseController(state);
  assert.equal(state.outputs.selected, "true");
  assert.equal(state.outputs["source-pr-number"], "42");
  assert.equal(state.outputs["source-pr-commit"], sourceCommit);
  assert.equal(state.outputs["release-commit"], backportCommit);
  assert.equal(state.outputs.trigger, "release-backport");
});

test("ignores a backport whose source is not a release pull", async () => {
  const ordinary = releasePull({
    number: 41,
    head: { ref: "fix/ordinary", repo: { full_name: repository } }
  });
  const state = fixture({
    pull: {
      base: { ref: "release/0.61" },
      head: {
        ref: "automation/backport-41-to-0.61",
        repo: { full_name: repository }
      },
      merged: true,
      merge_commit_sha: backportCommit
    },
    pulls: [ordinary]
  });
  await resolveReleaseController(state);
  assert.deepEqual(state.outputs, { selected: "false" });
});

test("resolves explicit version and source inputs to one plan", async () => {
  const state = fixture({ mode: "dispatch", pulls: [releasePull()] });
  await resolveReleaseController(state);
  assert.equal(state.outputs.selected, "true");
  assert.equal(state.outputs.trigger, "dispatch");
});

test("rejects ambiguous generated release pull requests", async () => {
  const state = fixture({
    mode: "dispatch",
    pulls: [releasePull(), releasePull({ number: 44 })]
  });
  await assert.rejects(
    () => resolveReleaseController(state),
    /Expected one merged generated release pull request/
  );
});

test("rejects a malformed explicit source commit", async () => {
  const state = fixture({ mode: "dispatch", pulls: [releasePull()] });
  state.inputs.SOURCE_SHA = "main";
  await assert.rejects(
    () => resolveReleaseController(state),
    /SOURCE_SHA must be a full commit SHA/
  );
});
