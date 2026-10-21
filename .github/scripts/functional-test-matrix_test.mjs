// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import test from "node:test";

import { selectNonCloudTests } from "./functional-test-matrix.mjs";

const full = selectNonCloudTests({ eventName: "schedule" }).matrix.include;
const legacyNames = [
  "daprrp-noncloud",
  "datastoresrp-noncloud",
  "msgrp-noncloud"
];
const pr = full.filter(({ name }) => !legacyNames.includes(name));

for (const { name, context, expected } of [
  {
    name: "main PR",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "main" } } }
    },
    expected: pr
  },
  {
    name: "feature PR",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "features/example" } } }
    },
    expected: pr
  },
  {
    name: "fork PR",
    context: {
      eventName: "pull_request",
      payload: {
        pull_request: {
          base: { ref: "main" },
          head: { repo: { fork: true } }
        }
      }
    },
    expected: pr
  },
  {
    name: "release PR uses the base, not the head branch",
    context: {
      eventName: "pull_request",
      payload: {
        pull_request: { base: { ref: "release/0.60" }, head: { ref: "main" } }
      }
    },
    expected: full
  },
  {
    name: "main merge queue",
    context: {
      eventName: "merge_group",
      payload: { merge_group: { base_ref: "refs/heads/main" } }
    },
    expected: pr
  },
  {
    name: "unknown PR base retains full coverage",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "other" } } }
    },
    expected: full
  },
  {
    name: "other merge queue",
    context: {
      eventName: "merge_group",
      payload: { merge_group: { base_ref: "refs/heads/release/0.60" } }
    },
    expected: full
  },
  {
    name: "manual defaults to full",
    context: { eventName: "workflow_dispatch", payload: { inputs: {} } },
    expected: full
  },
  {
    name: "manual PR profile",
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { "test-profile": "pr" } }
    },
    expected: pr
  },
  {
    name: "manual full profile",
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { "test-profile": "full" } }
    },
    expected: full
  },
  ...["schedule", "repository_dispatch", "push", "other"].map((eventName) => ({
    name: eventName,
    context: {
      eventName,
      payload: { inputs: { "test-profile": "pr" } }
    },
    expected: full
  }))
]) {
  test(`${name} selects the intended jobs and authorization variants`, () => {
    const result = selectNonCloudTests(context);
    assert.deepEqual(result.matrix.include, expected);
    assert.equal(result.profile, expected === pr ? "pr" : "full");
  });
}

test("full runs preserve all base suites and authorization dry runs", () => {
  assert.deepEqual(
    full.filter(({ authzDryRun }) => !authzDryRun).map(({ name }) => name),
    [
      "cli-noncloud",
      "corerp-noncloud",
      "daprrp-noncloud",
      "database-noncloud",
      "kubernetes-noncloud",
      "msgrp-noncloud",
      "multicluster-noncloud",
      "statestore-noncloud",
      "samples-noncloud",
      "ucp-noncloud",
      "authz-noncloud",
      "datastoresrp-noncloud",
      "dynamicrp-noncloud",
      "upgrade-noncloud"
    ]
  );
  assert.deepEqual(
    full.filter(({ authzDryRun }) => authzDryRun).map(({ name }) => name),
    [
      "corerp-noncloud",
      "dynamicrp-noncloud",
      "kubernetes-noncloud",
      "ucp-noncloud"
    ]
  );
});

for (const { context, error } of [
  { context: {}, error: /event name is required/ },
  {
    context: { eventName: "pull_request" },
    error: /Pull request base branch is required/
  },
  {
    context: { eventName: "merge_group" },
    error: /Merge group base branch is required/
  },
  {
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { "test-profile": "unknown" } }
    },
    error: /Invalid functional test profile/
  }
]) {
  test(`invalid selection fails: ${JSON.stringify(context)}`, () => {
    assert.throws(() => selectNonCloudTests(context), error);
  });
}
