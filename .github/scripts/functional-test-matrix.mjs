// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

const suites = [
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
];

const legacySuites = new Set([
  "daprrp-noncloud",
  "datastoresrp-noncloud",
  "msgrp-noncloud"
]);

const dryRunSuites = [
  "corerp-noncloud",
  "dynamicrp-noncloud",
  "kubernetes-noncloud",
  "ucp-noncloud"
];

// Suites that only exercise non-default authorization execution modes. PR-profile runs
// skip them unless authorization-related paths changed; full runs always include them.
const authzModeSuites = new Set(["authz-noncloud"]);

export function selectNonCloudTests({
  eventName,
  payload = {},
  authzChanged = true
}) {
  if (!eventName) {
    throw new Error("GitHub event name is required");
  }

  let profile = "full";
  if (eventName === "pull_request") {
    const base = payload.pull_request?.base?.ref;
    if (!base) {
      throw new Error("Pull request base branch is required");
    }
    if (base === "main" || /^features\/[^/]+$/.test(base)) {
      profile = "pr";
    }
  } else if (eventName === "merge_group") {
    const base = payload.merge_group?.base_ref;
    if (!base) {
      throw new Error("Merge group base branch is required");
    }
    if (base === "refs/heads/main") {
      profile = "pr";
    }
  } else if (eventName === "workflow_dispatch") {
    profile = payload.inputs?.["test-profile"] ?? "full";
    if (profile !== "full" && profile !== "pr") {
      throw new Error(`Invalid functional test profile: ${profile}`);
    }
  }

  const includeAuthzModes = profile === "full" || authzChanged !== false;
  // Authorization changes also need the legacy paths exercised with authorization off.
  const selected = suites.filter(
    (name) =>
      includeAuthzModes ||
      (!legacySuites.has(name) && !authzModeSuites.has(name))
  );
  const include = [
    ...selected.map((name) => ({
      os: "ubuntu-24.04",
      name,
      authzDryRun: false
    })),
    ...(includeAuthzModes ? dryRunSuites : []).map((name) => ({
      os: "ubuntu-24.04",
      name,
      authzDryRun: true
    }))
  ];
  return { profile, authzModes: includeAuthzModes, matrix: { include } };
}
