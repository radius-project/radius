// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import monitorRemoteWorkflow from "./monitor-remote-workflow.mjs";

function createCore(overrides = {}) {
  const inputs = {
    OWNER: "azure-octo",
    REPO: "radius-publisher",
    WORKFLOW_FILE: "publish-deployment-engine.yml",
    EVENT_TYPE: "deployment-engine",
    RELEASE_IDENTIFIER: "0.61.0-aaaaaaaa",
    CLIENT_PAYLOAD: JSON.stringify({ tag: "0.61" }),
    MAX_WAIT_SECONDS: "30",
    POLL_INTERVAL_SECONDS: "1",
    ...overrides,
  };
  const outputs = new Map();
  const failures = [];
  const infos = [];

  return {
    inputs,
    outputs,
    failures,
    infos,
    getInput(name, options = {}) {
      const value = inputs[name] || "";
      if (options.required && !value) {
        throw new Error(`Input required and not supplied: ${name}`);
      }
      return value;
    },
    setOutput(name, value) {
      outputs.set(name, value);
    },
    setFailed(message) {
      failures.push(message);
    },
    info(message) {
      infos.push(message);
    },
  };
}

function createClock() {
  let time = 0;
  return {
    now: () => time,
    sleep: async (milliseconds) => {
      time += milliseconds;
    },
  };
}

function apiError(status, message = `GitHub API returned ${status}`) {
  return Object.assign(new Error(message), { status });
}

function createGithub({ runs = [], getRun, jobs = [] } = {}) {
  const dispatches = [];
  const calls = { list: 0, get: 0, jobs: 0 };
  let nextRunID = 1000;

  const github = {
    dispatches,
    calls,
    runs,
    async paginate(method, parameters, map) {
      return map(await method(parameters));
    },
    rest: {
      actions: {
        async listWorkflowRuns() {
          calls.list += 1;
          return { data: { workflow_runs: [...runs] } };
        },
        async getWorkflowRun({ run_id: runID }) {
          calls.get += 1;
          const current =
            getRun?.(runID, calls.get) || runs.find((run) => run.id === runID);
          return { data: current };
        },
        async listJobsForWorkflowRun() {
          calls.jobs += 1;
          return { data: { jobs } };
        },
      },
      repos: {
        async createDispatchEvent({
          event_type: eventType,
          client_payload: payload,
        }) {
          dispatches.push({ eventType, payload });
          runs.push({
            id: nextRunID++,
            display_title: `${eventType} / ${payload.release_identifier}`,
            status: "completed",
            conclusion: "success",
            html_url: `https://example.test/runs/${nextRunID - 1}`,
          });
        },
      },
    },
  };
  return github;
}

function successfulRun(id, identifier) {
  return {
    id,
    display_title: `deployment-engine / ${identifier}`,
    status: "completed",
    conclusion: "success",
    html_url: `https://example.test/runs/${id}`,
  };
}

test("reuses an existing successful run without dispatching", async () => {
  const identifier = "0.61.0-aaaaaaaa";
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub({ runs: [successfulRun(42, identifier)] });
  const clock = createClock();

  await monitorRemoteWorkflow({ github, core, ...clock });

  assert.deepEqual(core.failures, []);
  assert.equal(github.dispatches.length, 0);
  assert.equal(core.outputs.get("run_id"), "42");
  assert.equal(core.outputs.get("conclusion"), "success");
});

test("dispatches once and injects the release identifier into the payload", async () => {
  const identifier = "0.61.0-bbbbbbbb";
  const core = createCore({
    RELEASE_IDENTIFIER: identifier,
    CLIENT_PAYLOAD: JSON.stringify({ tag: "0.61", source_sha: "b".repeat(40) }),
  });
  const github = createGithub();
  const clock = createClock();

  await monitorRemoteWorkflow({ github, core, ...clock });

  assert.deepEqual(core.failures, []);
  assert.equal(github.dispatches.length, 1);
  assert.deepEqual(github.dispatches[0], {
    eventType: "deployment-engine",
    payload: {
      tag: "0.61",
      source_sha: "b".repeat(40),
      release_identifier: identifier,
    },
  });
});

test("retries transient run lookups with bounded backoff", async () => {
  const identifier = "0.61.0-bcbcbcbc";
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub({ runs: [successfulRun(45, identifier)] });
  const listWorkflowRuns = github.rest.actions.listWorkflowRuns;
  let attempts = 0;
  github.rest.actions.listWorkflowRuns = async (parameters) => {
    attempts += 1;
    if (attempts < 3) {
      throw apiError(503);
    }
    return listWorkflowRuns(parameters);
  };

  await monitorRemoteWorkflow({
    github,
    core,
    ...createClock(),
    random: () => 0,
  });

  assert.deepEqual(core.failures, []);
  assert.equal(attempts, 3);
  assert.equal(github.dispatches.length, 0);
  assert.equal(core.outputs.get("run_id"), "45");
});

test("does not retry non-transient API failures", async () => {
  const core = createCore();
  const github = createGithub();
  let attempts = 0;
  github.rest.actions.listWorkflowRuns = async () => {
    attempts += 1;
    throw apiError(422, "Invalid workflow query");
  };

  await monitorRemoteWorkflow({
    github,
    core,
    ...createClock(),
    random: () => 0,
  });

  assert.equal(attempts, 1);
  assert.match(core.failures[0], /Invalid workflow query/);
});

test("reconciles an accepted final dispatch after a transient response error", async () => {
  const identifier = "0.61.0-bdbdbdbd";
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub();
  const createDispatchEvent = github.rest.repos.createDispatchEvent;
  let attempts = 0;
  github.rest.repos.createDispatchEvent = async (parameters) => {
    attempts += 1;
    if (attempts < 5) {
      throw apiError(502);
    }
    await createDispatchEvent(parameters);
    throw apiError(502);
  };

  await monitorRemoteWorkflow({
    github,
    core,
    ...createClock(),
    random: () => 0,
  });

  assert.deepEqual(core.failures, []);
  assert.equal(attempts, 5);
  assert.equal(github.dispatches.length, 1);
  assert.equal(core.outputs.get("conclusion"), "success");
});

test("retries a rejected transient dispatch after correlated lookup", async () => {
  const identifier = "0.61.0-bebebebe";
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub();
  const createDispatchEvent = github.rest.repos.createDispatchEvent;
  let attempts = 0;
  github.rest.repos.createDispatchEvent = async (parameters) => {
    attempts += 1;
    if (attempts === 1) {
      throw apiError(503);
    }
    return createDispatchEvent(parameters);
  };

  await monitorRemoteWorkflow({
    github,
    core,
    ...createClock(),
    random: () => 0,
  });

  assert.deepEqual(core.failures, []);
  assert.equal(attempts, 2);
  assert.equal(github.dispatches.length, 1);
  assert.equal(core.outputs.get("conclusion"), "success");
});

test("monitors an existing active run to completion", async () => {
  const identifier = "0.61.0-cccccccc";
  const active = {
    id: 43,
    display_title: `deployment-engine / ${identifier}`,
    status: "in_progress",
    conclusion: null,
  };
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub({
    runs: [active],
    getRun: (runID) => ({
      ...active,
      id: runID,
      status: "completed",
      conclusion: "success",
    }),
  });
  const clock = createClock();

  await monitorRemoteWorkflow({ github, core, ...clock });

  assert.deepEqual(core.failures, []);
  assert.equal(github.dispatches.length, 0);
  assert.equal(github.calls.get, 1);
  assert.equal(core.outputs.get("conclusion"), "success");
});

test("dispatches a retry after an existing failed run", async () => {
  const identifier = "0.61.0-dddddddd";
  const failed = {
    id: 44,
    display_title: `deployment-engine / ${identifier}`,
    status: "completed",
    conclusion: "failure",
    html_url: "https://example.test/runs/44",
  };
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub({ runs: [failed] });

  await monitorRemoteWorkflow({ github, core, ...createClock() });

  assert.deepEqual(core.failures, []);
  assert.equal(github.dispatches.length, 1);
  assert.notEqual(core.outputs.get("run_id"), "44");
  assert.equal(core.outputs.get("conclusion"), "success");
});

test("reports a newly dispatched failed retry with job diagnostics", async () => {
  const identifier = "0.61.0-ffffffff";
  const core = createCore({ RELEASE_IDENTIFIER: identifier });
  const github = createGithub({
    runs: [],
    jobs: [
      {
        name: "Publish",
        conclusion: "failure",
        steps: [{ name: "Copy", conclusion: "failure" }],
      },
    ],
  });
  github.rest.repos.createDispatchEvent = async ({
    event_type: eventType,
    client_payload: payload,
  }) => {
    github.dispatches.push({ eventType, payload });
    github.runs.push({
      id: 1001,
      display_title: `${eventType} / ${payload.release_identifier}`,
      status: "completed",
      conclusion: "failure",
      html_url: "https://example.test/runs/1001",
    });
  };

  await monitorRemoteWorkflow({ github, core, ...createClock() });

  assert.equal(github.dispatches.length, 1);
  assert.equal(github.calls.jobs, 1);
  assert.match(core.failures[0], /Correlated remote workflow failed/);
  assert.match(core.failures[0], /Step: Copy/);
});

test("ignores concurrent runs with other identifiers", async () => {
  const firstID = "0.61.0-11111111";
  const secondID = "0.61.0-22222222";
  const runs = [];
  const github = createGithub({ runs });
  const firstCore = createCore({ RELEASE_IDENTIFIER: firstID });
  const secondCore = createCore({ RELEASE_IDENTIFIER: secondID });

  await Promise.all([
    monitorRemoteWorkflow({ github, core: firstCore, ...createClock() }),
    monitorRemoteWorkflow({ github, core: secondCore, ...createClock() }),
  ]);

  assert.deepEqual(firstCore.failures, []);
  assert.deepEqual(secondCore.failures, []);
  assert.equal(github.dispatches.length, 2);
  assert.notEqual(
    firstCore.outputs.get("run_id"),
    secondCore.outputs.get("run_id"),
  );
  assert.equal(
    github.dispatches.find(
      (item) => item.payload.release_identifier === firstID,
    ).payload.release_identifier,
    firstID,
  );
  assert.equal(
    github.dispatches.find(
      (item) => item.payload.release_identifier === secondID,
    ).payload.release_identifier,
    secondID,
  );
});

test("rejects malformed identifiers and mismatched payloads before API calls", async () => {
  const malformedCore = createCore({ RELEASE_IDENTIFIER: "bad identifier" });
  const malformedGithub = createGithub();
  await monitorRemoteWorkflow({
    github: malformedGithub,
    core: malformedCore,
    ...createClock(),
  });
  assert.match(malformedCore.failures[0], /Release identifier/);
  assert.equal(malformedGithub.calls.list, 0);

  const mismatchedCore = createCore({
    CLIENT_PAYLOAD: JSON.stringify({ release_identifier: "other" }),
  });
  const mismatchedGithub = createGithub();
  await monitorRemoteWorkflow({
    github: mismatchedGithub,
    core: mismatchedCore,
    ...createClock(),
  });
  assert.match(mismatchedCore.failures[0], /does not match/);
  assert.equal(mismatchedGithub.calls.list, 0);
});

test("uses one total timeout budget for discovery and completion", async () => {
  const identifier = "0.61.0-eeeeeeee";
  const core = createCore({
    RELEASE_IDENTIFIER: identifier,
    MAX_WAIT_SECONDS: "2",
    POLL_INTERVAL_SECONDS: "1",
  });
  const github = createGithub();
  github.rest.repos.createDispatchEvent = async ({
    event_type: eventType,
    client_payload: payload,
  }) => {
    github.dispatches.push({ eventType, payload });
  };
  const clock = createClock();

  await monitorRemoteWorkflow({ github, core, ...clock });

  assert.equal(github.dispatches.length, 1);
  assert.match(core.failures[0], /Timed out waiting for publisher run/);
});

test("all publisher callers use stable identifiers without time-window discovery", async () => {
  const cases = [
    {
      file: "__build-bicep-types.yaml",
      identifier:
        "INPUT_RELEASE_IDENTIFIER: ${{ steps.release-metadata.outputs.release_version }}-${{ github.sha }}",
      eventType: "INPUT_EVENT_TYPE: bicep-types",
      timeout: "timeout-minutes: 18",
    },
    {
      file: "publish-de-image.yaml",
      identifier:
        "INPUT_RELEASE_IDENTIFIER: ${{ steps.payload.outputs.tag }}-${{ github.run_id }}",
      eventType: "INPUT_EVENT_TYPE: deployment-engine",
      timeout: "timeout-minutes: 18",
    },
    {
      file: "release.yaml",
      identifier:
        "INPUT_RELEASE_IDENTIFIER: ${{ steps.get-version.outputs.release-version }}-${{ github.sha }}",
      eventType: "INPUT_EVENT_TYPE: deployment-engine",
      timeout: "timeout-minutes: 25",
    },
  ];

  for (const fixture of cases) {
    const contents = await readFile(
      new URL(`../workflows/${fixture.file}`, import.meta.url),
      "utf8",
    );
    assert.match(contents, /monitor-remote-workflow\.mjs/);
    assert.ok(contents.includes(fixture.identifier), fixture.file);
    assert.ok(contents.includes(fixture.eventType), fixture.file);
    assert.ok(contents.includes(fixture.timeout), fixture.file);
    assert.ok(contents.includes('INPUT_MAX_WAIT_SECONDS: "720"'), fixture.file);
    assert.doesNotMatch(contents, /INPUT_DISPATCH_STARTED_AT/);
    assert.doesNotMatch(contents, /peter-evans\/repository-dispatch/);
  }
});
