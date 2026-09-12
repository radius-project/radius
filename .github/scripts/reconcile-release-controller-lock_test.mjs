import assert from "node:assert/strict";
import test from "node:test";

import reconcileLock, {
  canonicalLock
} from "./reconcile-release-controller-lock.mjs";

const releaseSource = "a".repeat(40);
const deSource = "b".repeat(40);
const digest = `sha256:${"c".repeat(64)}`;

function fixture() {
  const inputs = {
    MODE: "read",
    VERSION: "v0.61.0-rc.1",
    RELEASE_SOURCE_SHA: releaseSource,
    DE_SIGNED_TAG: "v0.61.0-rc.1",
    DE_SOURCE_SHA: deSource,
    DE_IMAGE_TAG: "0.61.0-rc.1",
    DE_DIGEST: ""
  };
  const outputs = {};
  const state = { ref: undefined, content: undefined };
  const github = {
    rest: {
      git: {
        getRef: async () => {
          if (!state.ref) {
            throw Object.assign(new Error("not found"), { status: 404 });
          }
          return { data: state.ref };
        },
        createRef: async ({ sha }) => {
          state.ref = { object: { sha } };
        }
      },
      repos: {
        getContent: async () => {
          if (!state.content) {
            throw Object.assign(new Error("not found"), { status: 404 });
          }
          return { data: state.content };
        },
        createOrUpdateFileContents: async ({ content }) => {
          state.ref = { object: { sha: "d".repeat(40) } };
          state.content = { type: "file", content };
        },
        getCommit: async () => ({
          data: { parents: [{ sha: releaseSource }] }
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
  return {
    github,
    context: { repo: { owner: "radius-project", repo: "radius" } },
    core,
    inputs,
    outputs,
    state
  };
}

test("reports a missing release lock", async () => {
  const state = fixture();
  await reconcileLock(state);
  assert.equal(state.outputs.exists, "false");
});

test("creates and reuses an immutable release lock", async () => {
  const state = fixture();
  state.inputs.MODE = "create";
  state.inputs.DE_DIGEST = digest;
  await reconcileLock(state);
  assert.equal(state.outputs.exists, "true");
  assert.equal(state.outputs.digest, digest);
  assert.equal(state.outputs.commit, "d".repeat(40));

  state.inputs.MODE = "read";
  state.inputs.DE_DIGEST = "";
  state.outputs = {};
  await reconcileLock({
    ...state,
    core: {
      ...state.core,
      setOutput: (name, value) => {
        state.outputs[name] = value;
      }
    }
  });
  assert.equal(state.outputs.exists, "true");
  assert.equal(state.outputs.digest, digest);
});

test("rejects a changed digest after the lock exists", async () => {
  const state = fixture();
  const lock = canonicalLock(
    {
      version: state.inputs.VERSION,
      releaseSourceCommit: releaseSource,
      signedTag: state.inputs.DE_SIGNED_TAG,
      deSourceCommit: deSource,
      imageTag: state.inputs.DE_IMAGE_TAG
    },
    digest
  );
  state.state.ref = { object: { sha: "d".repeat(40) } };
  state.state.content = {
    type: "file",
    content: Buffer.from(JSON.stringify(lock)).toString("base64")
  };
  state.inputs.MODE = "create";
  state.inputs.DE_DIGEST = `sha256:${"e".repeat(64)}`;
  await assert.rejects(() => reconcileLock(state), /digest is locked/);
});

test("rejects release state with unexpected history", async () => {
  const state = fixture();
  const lock = canonicalLock(
    {
      version: state.inputs.VERSION,
      releaseSourceCommit: releaseSource,
      signedTag: state.inputs.DE_SIGNED_TAG,
      deSourceCommit: deSource,
      imageTag: state.inputs.DE_IMAGE_TAG
    },
    digest
  );
  state.state.ref = { object: { sha: "d".repeat(40) } };
  state.state.content = {
    type: "file",
    content: Buffer.from(JSON.stringify(lock)).toString("base64")
  };
  state.github.rest.repos.getCommit = async () => ({
    data: { parents: [{ sha: "f".repeat(40) }] }
  });
  await assert.rejects(() => reconcileLock(state), /unexpected history/);
});

test("does not write a lock on a conflicting state branch", async () => {
  const state = fixture();
  let writes = 0;
  state.state.ref = { object: { sha: "f".repeat(40) } };
  state.inputs.MODE = "create";
  state.inputs.DE_DIGEST = digest;
  state.github.rest.repos.createOrUpdateFileContents = async () => {
    writes += 1;
  };
  await assert.rejects(() => reconcileLock(state), /is rooted at/);
  assert.equal(writes, 0);
});
