// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";

import { selectNonCloudTests } from "./functional-test-matrix.mjs";

const full = selectNonCloudTests({ eventName: "schedule" }).matrix.include;
const legacyNames = [
  "daprrp-noncloud",
  "datastoresrp-noncloud",
  "msgrp-noncloud"
];

for (const { name, context, profile } of [
  {
    name: "main PR",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "main" } } }
    },
    profile: "pr"
  },
  {
    name: "feature PR",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "features/example" } } }
    },
    profile: "pr"
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
    profile: "pr"
  },
  {
    name: "release PR uses the base, not the head branch",
    context: {
      eventName: "pull_request",
      payload: {
        pull_request: { base: { ref: "release/0.60" }, head: { ref: "main" } }
      }
    },
    profile: "full"
  },
  {
    name: "main merge queue",
    context: {
      eventName: "merge_group",
      payload: { merge_group: { base_ref: "refs/heads/main" } }
    },
    profile: "pr"
  },
  {
    name: "unknown PR base retains full coverage",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "other" } } }
    },
    profile: "full"
  },
  {
    name: "other merge queue",
    context: {
      eventName: "merge_group",
      payload: { merge_group: { base_ref: "refs/heads/release/0.60" } }
    },
    profile: "full"
  },
  {
    name: "manual defaults to full",
    context: { eventName: "workflow_dispatch", payload: { inputs: {} } },
    profile: "full"
  },
  {
    name: "manual PR profile",
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { "test-profile": "pr" } }
    },
    profile: "pr"
  },
  {
    name: "manual full profile",
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { "test-profile": "full" } }
    },
    profile: "full"
  },
  ...["schedule", "repository_dispatch", "push", "other"].map((eventName) => ({
    name: eventName,
    context: {
      eventName,
      payload: { inputs: { "test-profile": "pr" } }
    },
    profile: "full"
  }))
]) {
  test(`${name} retains all suites for authorization changes or missing detection`, () => {
    for (const authzChanged of [true, undefined]) {
      const result = selectNonCloudTests({ ...context, authzChanged });
      assert.deepEqual(result.matrix.include, full);
      assert.equal(result.profile, profile);
      for (const name of legacyNames) {
        assert.deepEqual(
          result.matrix.include.filter((leg) => leg.name === name),
          [{ os: "ubuntu-24.04", name, authzDryRun: false }]
        );
      }
    }
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

const prBaseOnly = full.filter(
  ({ name, authzDryRun }) =>
    !legacyNames.includes(name) && !authzDryRun && name !== "authz-noncloud"
);

for (const { name, context, expected } of [
  {
    name: "main PR",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "main" } } }
    },
    expected: prBaseOnly
  },
  {
    name: "feature PR",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "features/example" } } }
    },
    expected: prBaseOnly
  },
  {
    name: "main merge queue",
    context: {
      eventName: "merge_group",
      payload: { merge_group: { base_ref: "refs/heads/main" } }
    },
    expected: prBaseOnly
  },
  {
    name: "manual PR profile",
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { "test-profile": "pr" } }
    },
    expected: prBaseOnly
  },
  {
    name: "release PR keeps every authorization mode",
    context: {
      eventName: "pull_request",
      payload: { pull_request: { base: { ref: "release/0.60" } } }
    },
    expected: full
  },
  ...["schedule", "repository_dispatch", "workflow_dispatch"].map(
    (eventName) => ({
      name: `${eventName} keeps every authorization mode`,
      context: { eventName },
      expected: full
    })
  )
]) {
  test(`${name} without authorization changes selects the intended jobs`, () => {
    const result = selectNonCloudTests({ ...context, authzChanged: false });
    assert.deepEqual(result.matrix.include, expected);
    assert.equal(result.authzModes, expected === full);
  });
}

test("PR profile without authorization changes keeps default-mode suites", () => {
  assert.deepEqual(
    prBaseOnly.map(({ name }) => name),
    [
      "cli-noncloud",
      "corerp-noncloud",
      "database-noncloud",
      "kubernetes-noncloud",
      "multicluster-noncloud",
      "statestore-noncloud",
      "samples-noncloud",
      "ucp-noncloud",
      "dynamicrp-noncloud",
      "upgrade-noncloud"
    ]
  );
});

test("missing authorization change result keeps legacy suites and every authorization mode", () => {
  const context = {
    eventName: "pull_request",
    payload: { pull_request: { base: { ref: "main" } } }
  };
  assert.deepEqual(selectNonCloudTests(context).matrix.include, full);
  assert.deepEqual(
    selectNonCloudTests({ ...context, authzChanged: true }).matrix.include,
    full
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

for (const { name, context, authzChanged, expected } of [
  ...[undefined, "pr"].map((profile) => ({
    name: `manual ${profile ?? "default"} profile`,
    context: {
      eventName: "workflow_dispatch",
      payload: { inputs: { branch: "release/old", "test-profile": profile } }
    },
    authzChanged: "true",
    expected: full
  })),
  ...["pull_request", "merge_group"].flatMap((eventName) =>
    ["true", "false", undefined].map((authzChanged) => ({
      name: `${eventName} with AUTHZ_CHANGED=${authzChanged}`,
      context: {
        eventName,
        payload: {
          pull_request: { base: { ref: "main" } },
          merge_group: { base_ref: "refs/heads/main" }
        }
      },
      authzChanged,
      expected: authzChanged === "false" ? prBaseOnly : full
    }))
  )
]) {
  test(`${name} works when the source checkout lacks the selector`, async (t) => {
    const workflow = readFileSync(
      new URL("../workflows/functional-test-noncloud.yaml", import.meta.url),
      "utf8"
    );
    const build = workflow.match(
      /^  build:\n[\s\S]*?(?=^  [a-z][\w-]*:)/m
    )?.[0];
    assert.ok(build, "build job must exist");
    const sourceCheckout = build.match(
      /      - name: Checkout\n[\s\S]*?(?=\n      - )/
    )?.[0];
    assert.ok(sourceCheckout, "source checkout must exist");
    assert.ok(
      sourceCheckout.includes(
        "ref: ${{ github.event_name == 'workflow_dispatch' && format('refs/heads/{0}', github.event.inputs.branch) || github.ref }}"
      ),
      "build artifacts must still come from the selected source branch"
    );
    const checkout = build.match(
      /      - name: Checkout workflow test selector\n[\s\S]*?(?=\n      - )/
    )?.[0];
    assert.ok(checkout, "workflow selector checkout must exist");
    assert.match(checkout, /repository: \$\{\{ github\.repository \}\}/);
    assert.match(checkout, /ref: \$\{\{ github\.workflow_sha \}\}/);
    assert.match(
      checkout,
      /sparse-checkout: \.github\/scripts\/functional-test-matrix\.mjs/
    );
    assert.match(checkout, /sparse-checkout-cone-mode: false/);
    assert.match(checkout, /persist-credentials: false/);
    const checkoutPath = checkout.match(/path: (.+)/)?.[1];
    assert.ok(checkoutPath, "workflow selector needs a separate checkout path");
    const selection = build.match(
      /      - name: Select functional test matrix\n[\s\S]*?(?=\n      - )/
    )?.[0];
    assert.ok(selection, "matrix selection step must exist");
    assert.ok(
      selection.includes(
        "AUTHZ_CHANGED: ${{ needs.changes.outputs.authz_changed }}"
      ),
      "matrix selection must receive the authorization change result"
    );
    assert.ok(build.indexOf(checkout) > build.indexOf(sourceCheckout));
    assert.ok(build.indexOf(selection) > build.indexOf(checkout));

    const root = mkdtempSync(join(tmpdir(), "functional matrix #checkout-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const selectorPath = ".github/scripts/functional-test-matrix.mjs";
    assert.equal(existsSync(join(root, selectorPath)), false);
    const scriptPath = join(root, checkoutPath, selectorPath);
    mkdirSync(dirname(scriptPath), { recursive: true });
    copyFileSync(
      new URL("./functional-test-matrix.mjs", import.meta.url),
      scriptPath
    );

    const script = selection
      .split("          script: |\n")[1]
      .replace(/^ {12}/gm, "");
    const AsyncFunction = Object.getPrototypeOf(
      async function () {}
    ).constructor;
    const runSelector = new AsyncFunction("context", "core", "process", script);
    const outputs = [];
    const summary = {
      addHeading() {
        return this;
      },
      addRaw() {
        return this;
      },
      async write() {}
    };
    await runSelector(
      context,
      {
        setOutput: (name, value) => outputs.push([name, value]),
        info() {},
        summary
      },
      { env: { GITHUB_WORKSPACE: root, AUTHZ_CHANGED: authzChanged } }
    );
    assert.deepEqual(outputs, [["matrix", { include: expected }]]);
  });
}
