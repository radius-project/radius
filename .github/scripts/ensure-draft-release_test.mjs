import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import ensureDraftRelease from "./ensure-draft-release.mjs";

async function fixture(existing) {
  const root = await mkdtemp(path.join(os.tmpdir(), "ensure-release-"));
  const notesFile = path.join(root, "notes.md");
  await writeFile(notesFile, "prepared notes\n");
  const releases = existing ? [existing] : [];
  const created = [];
  const delays = [];
  const messages = [];
  const inputs = {
    OWNER: "radius-project",
    REPO: "radius",
    TAG: "v0.61.0",
    SOURCE_SHA: "a".repeat(40),
    NOTES_FILE: notesFile,
    PRERELEASE: "false"
  };
  const outputs = {};
  const github = {
    paginate: async (method) => method(),
    rest: {
      repos: {
        listReleases: async () => releases,
        createRelease: async (request) => {
          created.push(request);
          const release = {
            id: 42,
            tag_name: request.tag_name,
            target_commitish: request.target_commitish,
            name: request.name,
            body: request.body,
            draft: request.draft,
            prerelease: request.prerelease
          };
          releases.push(release);
          return { data: release };
        }
      }
    }
  };
  const core = {
    getInput: (name) => inputs[name] ?? "",
    info: (message) => messages.push(message),
    setOutput: (name, value) => {
      outputs[name] = value;
    }
  };
  const sleep = async (milliseconds) => {
    delays.push(milliseconds);
  };
  return {
    core,
    created,
    delays,
    github,
    inputs,
    messages,
    outputs,
    releases,
    root,
    sleep
  };
}

test("creates a source-bound draft release", async () => {
  const state = await fixture();
  try {
    await ensureDraftRelease(state);
    assert.equal(state.created.length, 1);
    assert.equal(state.created[0].make_latest, "false");
    assert.equal(state.outputs.release_id, "42");
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("reuses a matching draft release", async () => {
  const state = await fixture({
    id: 7,
    tag_name: "v0.61.0",
    target_commitish: "a".repeat(40),
    name: "Radius v0.61.0",
    body: "prepared notes\n",
    draft: true,
    prerelease: false
  });
  try {
    await ensureDraftRelease(state);
    assert.deepEqual(state.created, []);
    assert.equal(state.outputs.release_id, "7");
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("rejects a draft with different prepared notes", async () => {
  const state = await fixture({
    id: 7,
    tag_name: "v0.61.0",
    target_commitish: "a".repeat(40),
    name: "Radius v0.61.0",
    body: "different notes\n",
    draft: true,
    prerelease: false
  });
  try {
    await assert.rejects(() => ensureDraftRelease(state), /prepared notes/);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("reconciles a create accepted before a network error", async () => {
  const state = await fixture();
  const create = state.github.rest.repos.createRelease;
  state.github.rest.repos.createRelease = async (request) => {
    await create(request);
    throw new Error("connection reset after create");
  };
  try {
    await ensureDraftRelease(state);
    assert.equal(state.releases.length, 1);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("uses the created draft while release listings are stale", async () => {
  const state = await fixture();
  let listings = 0;
  state.github.rest.repos.listReleases = async () => {
    listings++;
    return [];
  };
  try {
    await ensureDraftRelease(state);
    assert.equal(state.outputs.release_id, "42");
    assert.equal(state.created.length, 1);
    assert.equal(state.created[0].request.retries, 0);
    assert.equal(listings, 1);
    assert.deepEqual(state.delays, []);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("waits for a draft after a lost create response without creating again", async () => {
  const state = await fixture();
  const create = state.github.rest.repos.createRelease;
  let listings = 0;
  state.github.rest.repos.createRelease = async (request) => {
    await create(request);
    throw new Error("connection reset after create");
  };
  state.github.rest.repos.listReleases = async () => {
    listings++;
    if (listings <= 3) return [];
    if (listings === 4) return state.releases;
    throw new Error("must retain the recovered draft");
  };
  try {
    await ensureDraftRelease(state);
    assert.equal(state.outputs.release_id, "42");
    assert.equal(state.created.length, 1);
    assert.equal(listings, 4);
    assert.deepEqual(state.delays, [1000, 2000]);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("rejects a create response bound to a different tag", async () => {
  const state = await fixture();
  const create = state.github.rest.repos.createRelease;
  state.github.rest.repos.createRelease = async (request) => {
    const response = await create(request);
    response.data.tag_name = "v0.62.0";
    return response;
  };
  try {
    await assert.rejects(
      () => ensureDraftRelease(state),
      /does not match tag v0\.61\.0/
    );
    assert.equal(state.created.length, 1);
    assert.equal(state.outputs.release_id, undefined);
    assert.deepEqual(state.delays, []);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("reports a visibility timeout with the original create error", async () => {
  const state = await fixture();
  const failure = new Error("connection reset during create");
  state.github.rest.repos.createRelease = async (request) => {
    state.created.push(request);
    throw failure;
  };
  try {
    await assert.rejects(
      () => ensureDraftRelease(state),
      (error) => {
        assert.match(error.message, /v0\.61\.0.*not visible after 5 attempts/);
        assert.equal(error.cause, failure);
        return true;
      }
    );
    assert.equal(state.created.length, 1);
    assert.equal(state.outputs.release_id, undefined);
    assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
    assert.equal(state.messages.length, 4);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

test("reuses a draft on the next invocation without another create", async () => {
  const state = await fixture();
  try {
    await ensureDraftRelease(state);
    await ensureDraftRelease(state);
    assert.equal(state.created.length, 1);
    assert.equal(state.outputs.release_id, "42");
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});

for (const [change, expected] of [
  [{ draft: false }, /is not a draft/],
  [{ name: "unexpected title" }, /wrong title/],
  [{ prerelease: true }, /wrong classification/]
]) {
  test(`rejects a conflicting existing release: ${Object.keys(change)[0]}`, async () => {
    const state = await fixture({
      id: 7,
      tag_name: "v0.61.0",
      target_commitish: "a".repeat(40),
      name: "Radius v0.61.0",
      body: "prepared notes\n",
      draft: true,
      prerelease: false,
      ...change
    });
    try {
      await assert.rejects(() => ensureDraftRelease(state), expected);
      assert.deepEqual(state.created, []);
      assert.deepEqual(state.delays, []);
      assert.equal(state.outputs.release_id, undefined);
    } finally {
      await rm(state.root, { recursive: true, force: true });
    }
  });
}

test("rejects duplicate releases without creating another draft", async () => {
  const state = await fixture();
  state.releases.push(
    { id: 1, tag_name: "v0.61.0", draft: true },
    { id: 2, tag_name: "v0.61.0", draft: true }
  );
  try {
    await assert.rejects(() => ensureDraftRelease(state), /Multiple releases/);
    assert.deepEqual(state.created, []);
    assert.deepEqual(state.delays, []);
  } finally {
    await rm(state.root, { recursive: true, force: true });
  }
});
