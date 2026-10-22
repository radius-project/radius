import assert from "node:assert/strict";
import test from "node:test";

import {
  findRelease,
  waitForRelease,
  waitForReleaseState
} from "./release-reconciliation.mjs";

function fixture() {
  const delays = [];
  const messages = [];
  return {
    delays,
    messages,
    core: { info: (message) => messages.push(message) },
    sleep: async (milliseconds) => {
      delays.push(milliseconds);
    },
    description: "Release v0.61.0"
  };
}

test("returns visible state without delaying or reading again", async () => {
  const state = fixture();
  const result = { id: 42 };
  assert.equal(await waitForReleaseState(async () => result, state), result);
  assert.deepEqual(state.delays, []);
  assert.deepEqual(state.messages, []);
});

test("waits for missing state with exponential backoff", async () => {
  const state = fixture();
  let reads = 0;
  const result = { id: 42 };
  assert.equal(
    await waitForReleaseState(
      async () => (++reads < 3 ? undefined : result),
      state
    ),
    result
  );
  assert.equal(reads, 3);
  assert.deepEqual(state.delays, [1000, 2000]);
  assert.equal(state.messages.length, 2);
});

test("bounds visibility reads and retains an uncertain write cause", async () => {
  const state = fixture();
  const failure = new Error("write response lost");
  let reads = 0;
  await assert.rejects(
    () =>
      waitForReleaseState(
        async () => {
          reads++;
          return undefined;
        },
        { ...state, cause: failure }
      ),
    (error) => {
      assert.match(error.message, /v0\.61\.0.*not visible after 5 attempts/);
      assert.equal(error.cause, failure);
      return true;
    }
  );
  assert.equal(reads, 5);
  assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
});

for (const status of [401, 403, 404, 500]) {
  test(`does not convert HTTP ${status} read errors into missing state`, async () => {
    const state = fixture();
    const failure = Object.assign(new Error("read failed"), { status });
    await assert.rejects(
      () =>
        waitForReleaseState(async () => {
          throw failure;
        }, state),
      (error) => error === failure
    );
    assert.deepEqual(state.delays, []);
  });
}

test("finds a release through paginated discovery after a visibility delay", async () => {
  const state = fixture();
  const release = { id: 42, tag_name: "v0.61.0" };
  let listings = 0;
  const github = {
    rest: { repos: { listReleases: () => {} } },
    paginate: async (method, inputs) => {
      assert.equal(method, github.rest.repos.listReleases);
      assert.deepEqual(inputs, {
        owner: "radius-project",
        repo: "radius",
        per_page: 100
      });
      return ++listings === 1 ? [] : [release];
    }
  };
  assert.equal(
    await waitForRelease(github, "radius-project", "radius", "v0.61.0", state),
    release
  );
  assert.deepEqual(state.delays, [1000]);
});

test("rejects duplicate releases without treating them as delayed visibility", async () => {
  const state = fixture();
  const release = { id: 42, tag_name: "v0.61.0" };
  await assert.rejects(
    () =>
      waitForReleaseState(
        async () => findRelease([release, { ...release, id: 43 }], "v0.61.0"),
        state
      ),
    /Multiple releases/
  );
  assert.deepEqual(state.delays, []);
});

test("distinguishes an absent release from a published release", () => {
  const published = { id: 42, tag_name: "v0.61.0", draft: false };
  assert.equal(findRelease([published], "v0.61.1"), undefined);
  assert.equal(findRelease([published], "v0.61.0"), published);
});
