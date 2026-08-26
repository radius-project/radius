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
  const inputs = {
    OWNER: "radius-project",
    REPO: "radius",
    TAG: "v0.61.0",
    SOURCE_SHA: "a".repeat(40),
    NOTES_FILE: notesFile,
    PRERELEASE: "false",
  };
  const outputs = {};
  const github = {
    paginate: async (method) => method(),
    rest: {
      repos: {
        listReleases: async () => releases,
        createRelease: async (request) => {
          created.push(request);
          releases.push({
            id: 42,
            tag_name: request.tag_name,
            target_commitish: request.target_commitish,
            name: request.name,
            body: request.body,
            draft: request.draft,
            prerelease: request.prerelease,
          });
        },
      },
    },
  };
  const core = {
    getInput: (name) => inputs[name] ?? "",
    setOutput: (name, value) => {
      outputs[name] = value;
    },
  };
  return { core, created, github, inputs, outputs, releases, root };
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
    prerelease: false,
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
    prerelease: false,
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