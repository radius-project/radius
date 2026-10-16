import assert from "node:assert/strict";
import test from "node:test";

import verifyReleaseEnvironment from "./verify-release-environment.mjs";

const reviewers = {
  type: "required_reviewers",
  reviewers: [{ type: "Team", reviewer: { slug: "maintainers-radius" } }]
};
const mainOnly = { protected_branches: false, custom_branch_policies: true };

/** @param {any} environment @param {any} [error] */
function fixture(environment, error) {
  const calls = [];
  const warnings = [];
  return {
    calls,
    warnings,
    context: { repo: { owner: "radius-project", repo: "radius" } },
    core: {
      getInput: (name) => (name === "ENVIRONMENT" ? "release" : ""),
      info: () => {},
      warning: (message) => warnings.push(message)
    },
    github: {
      rest: {
        repos: {
          getEnvironment: async (parameters) => {
            calls.push(parameters);
            if (error) {
              throw error;
            }
            return { data: environment };
          }
        }
      }
    }
  };
}

test("accepts an environment that requires reviewers", async () => {
  const state = fixture({
    protection_rules: [reviewers],
    deployment_branch_policy: mainOnly
  });
  await verifyReleaseEnvironment(state);
  assert.deepEqual(state.calls, [
    {
      owner: "radius-project",
      repo: "radius",
      environment_name: "release"
    }
  ]);
  assert.deepEqual(state.warnings, []);
});

test("rejects an environment without required reviewers", async () => {
  const state = fixture({
    protection_rules: [{ type: "wait_timer", wait_timer: 5 }],
    deployment_branch_policy: mainOnly
  });
  await assert.rejects(
    () => verifyReleaseEnvironment(state),
    /no required reviewers/
  );
});

test("rejects a reviewer rule that lists nobody", async () => {
  const state = fixture({
    protection_rules: [{ type: "required_reviewers", reviewers: [] }],
    deployment_branch_policy: mainOnly
  });
  await assert.rejects(
    () => verifyReleaseEnvironment(state),
    /no required reviewers/
  );
});

test("rejects a missing environment", async () => {
  const state = fixture(
    undefined,
    Object.assign(new Error("Not Found"), { status: 404 })
  );
  await assert.rejects(() => verifyReleaseEnvironment(state), /does not exist/);
});

test("propagates other API failures", async () => {
  const state = fixture(
    undefined,
    Object.assign(new Error("Server Error"), { status: 500 })
  );
  await assert.rejects(() => verifyReleaseEnvironment(state), /Server Error/);
});

test("warns when any branch can use the environment", async () => {
  const state = fixture({
    protection_rules: [reviewers],
    deployment_branch_policy: null
  });
  await verifyReleaseEnvironment(state);
  assert.equal(state.warnings.length, 1);
  assert.match(state.warnings[0], /any branch/);
});
