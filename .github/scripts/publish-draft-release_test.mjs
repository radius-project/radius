import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import publishDraftRelease, {
  mandatoryChecks
} from "./publish-draft-release.mjs";

function fixture({ draft = true, prerelease = false } = {}) {
  const updates = [];
  const release = {
    id: 42,
    tag_name: "v0.61.0",
    draft,
    prerelease,
    name: "Radius v0.61.0",
    body: "prepared notes\n",
    html_url: "https://example.test/releases/v0.61.0"
  };
  const outputs = {};
  const inputs = {
    OWNER: "radius-project",
    REPO: "radius",
    TAG: "v0.61.0",
    PRERELEASE: "false",
    MAKE_LATEST: "true",
    SOURCE_SHA: "a".repeat(40),
    NOTES_FILE: ""
  };
  const github = {
    paginate: async (method) => method(),
    rest: {
      repos: {
        listReleases: async () => [release],
        updateRelease: async (request) => {
          updates.push(request);
          release.draft = request.draft;
          release.prerelease = request.prerelease;
        },
        getLatestRelease: async () => ({ data: { tag_name: release.tag_name } })
      }
    }
  };
  const core = {
    getInput: (name) => inputs[name] ?? "",
    setOutput: (name, value) => {
      outputs[name] = value;
    }
  };
  return { core, github, inputs, outputs, release, updates };
}

async function withNotes(state, callback) {
  const root = await mkdtemp(path.join(os.tmpdir(), "publish-release-"));
  try {
    state.inputs.NOTES_FILE = path.join(root, "notes.md");
    await writeFile(state.inputs.NOTES_FILE, "prepared notes\n");
    state.inputs.MANIFEST_FILE = path.join(root, "release-manifest.json");
    await writeFile(
      state.inputs.MANIFEST_FILE,
      JSON.stringify({
        schemaVersion: 1,
        tag: state.inputs.TAG,
        sourceSha: state.inputs.SOURCE_SHA,
        checks: Object.fromEntries(
          mandatoryChecks.map((name) => [name, "verified"])
        )
      })
    );
    return await callback();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

test("publishes a stable draft and marks it latest", async () => {
  const state = fixture();
  await withNotes(state, () => publishDraftRelease(state));
  assert.equal(state.updates.length, 1);
  assert.equal(state.updates[0].make_latest, "true");
  assert.equal(state.release.draft, false);
  assert.equal(state.outputs.release_url, state.release.html_url);
});

test("publishes an older-channel patch without replacing latest", async () => {
  const state = fixture();
  const newer = { tag_name: "v0.62.0", draft: false, prerelease: false };
  state.github.rest.repos.listReleases = async () => [state.release, newer];
  state.github.rest.repos.getLatestRelease = async () => ({ data: newer });

  await withNotes(state, () => publishDraftRelease(state));

  assert.equal(state.updates[0].make_latest, "false");
  assert.equal(state.release.draft, false);
});

test("selects aliases without regressing published stable versions", async () => {
  for (const [tag, published, channel, latest] of [
    ["v0.61.0", ["v0.60.9"], "true", "true"],
    ["v0.61.1", ["v0.62.0", "v0.61.0"], "true", "false"],
    ["v0.61.1", ["v0.61.2"], "false", "false"],
    ["v0.100.0", ["v0.99.9"], "true", "true"],
    ["v0.61.0-rc.1", ["v0.60.0"], "false", "false"]
  ]) {
    const state = fixture();
    state.inputs.TAG = tag;
    state.inputs.MODE = "select-aliases";
    state.github.rest.repos.listReleases = async () => [
      ...published.map((version) => ({
        tag_name: version,
        draft: false,
        prerelease: false
      })),
      { tag_name: "v99.0.0", draft: true, prerelease: false },
      { tag_name: "v99.0.0-rc.1", draft: false, prerelease: true }
    ];

    await withNotes(state, () => publishDraftRelease(state));

    assert.equal(state.outputs.promote_channel, channel, tag);
    assert.equal(state.outputs.promote_latest, latest, tag);
    assert.deepEqual(state.updates, []);
  }
});

test("publishes a prerelease without marking it latest", async () => {
  const state = fixture();
  state.release.tag_name = "v0.61.0-rc.1";
  state.inputs.TAG = state.release.tag_name;
  state.inputs.PRERELEASE = "true";
  state.inputs.MAKE_LATEST = "false";
  state.release.name = `Radius ${state.release.tag_name}`;
  await withNotes(state, () => publishDraftRelease(state));
  assert.equal(state.updates[0].prerelease, true);
  assert.equal(state.updates[0].make_latest, "false");
});

test("reconciles an already-published matching release", async () => {
  const state = fixture({ draft: false });
  await withNotes(state, () => publishDraftRelease(state));
  assert.deepEqual(state.updates, []);
});

test("reconciles a historical stable release without checking latest", async () => {
  const state = fixture({ draft: false });
  state.inputs.MAKE_LATEST = "false";
  state.github.rest.repos.getLatestRelease = async () => {
    throw new Error("historical releases must not query latest");
  };
  await withNotes(state, () => publishDraftRelease(state));
  assert.deepEqual(state.updates, []);
});

test("reconciles publication accepted before a network error", async () => {
  const state = fixture();
  const update = state.github.rest.repos.updateRelease;
  state.github.rest.repos.updateRelease = async (request) => {
    await update(request);
    throw new Error("connection reset after publication");
  };
  await withNotes(state, () => publishDraftRelease(state));
  assert.equal(state.release.draft, false);
  assert.equal(state.updates.length, 1);
});

test("rejects a published release with the wrong classification", async () => {
  const state = fixture({ draft: false, prerelease: true });
  await assert.rejects(
    () => withNotes(state, () => publishDraftRelease(state)),
    /wrong classification/
  );
  assert.deepEqual(state.updates, []);
});

test("rejects release notes that drift from the prepared file", async () => {
  const state = fixture();
  state.release.body = "edited on GitHub";
  await assert.rejects(
    () => withNotes(state, () => publishDraftRelease(state)),
    /prepared notes/
  );
  assert.deepEqual(state.updates, []);
});

test("blocks publication and alias selection without complete source-bound verification", async () => {
  for (const mode of ["publish", "select-aliases"]) {
    for (const failure of ["missing", "digest", "installation", "source"]) {
      const state = fixture();
      state.inputs.MODE = mode;
      await withNotes(state, async () => {
        const manifest = {
          schemaVersion: 1,
          tag: state.inputs.TAG,
          sourceSha: state.inputs.SOURCE_SHA,
          checks: Object.fromEntries(
            mandatoryChecks.map((name) => [name, "verified"])
          )
        };
        if (failure === "digest") manifest.checks.images = "failed";
        if (failure === "installation") delete manifest.checks.installation;
        if (failure === "source") manifest.sourceSha = "b".repeat(40);
        await writeFile(state.inputs.MANIFEST_FILE, JSON.stringify(manifest));
        if (failure === "missing") state.inputs.MANIFEST_FILE = "";
        await assert.rejects(() => publishDraftRelease(state), /manifest/);
      });
      assert.deepEqual(state.updates, [], `${mode}: ${failure}`);
      assert.equal(state.outputs.promote_channel, undefined);
      assert.equal(state.outputs.promote_latest, undefined);
    }
  }
});
