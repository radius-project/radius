import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import releaseAssets from "./release-assets.mjs";

function fixture({ draft = true, assets = [] } = {}) {
  const outputs = {};
  const calls = { deleted: [], uploaded: [] };
  let nextID = 100;
  const release = { id: 42, tag_name: "v0.61.0", draft };
  const contents = new Map(
    assets.map((asset) => [asset.id, Buffer.from(asset.contents)])
  );
  const currentAssets = assets.map(({ id, name }) => ({ id, name }));
  const github = {
    paginate: async (method) => method(),
    request: async (_route, { asset_id }) => ({ data: contents.get(asset_id) }),
    rest: {
      repos: {
        listReleases: async () => [release],
        listReleaseAssets: async () => currentAssets,
        deleteReleaseAsset: async ({ asset_id }) => {
          calls.deleted.push(asset_id);
          const index = currentAssets.findIndex(
            (asset) => asset.id === asset_id
          );
          currentAssets.splice(index, 1);
          contents.delete(asset_id);
        },
        uploadReleaseAsset: async ({ name, data }) => {
          const asset = { id: nextID++, name };
          calls.uploaded.push(name);
          currentAssets.push(asset);
          contents.set(asset.id, Buffer.from(data));
          return { data: asset };
        }
      }
    }
  };
  const inputs = {};
  const core = {
    getInput: (name) => inputs[name] ?? "",
    setOutput: (name, value) => {
      outputs[name] = value;
    }
  };
  return { calls, core, github, inputs, outputs };
}

function spdxDocument(overrides = {}) {
  return JSON.stringify({
    spdxVersion: "SPDX-2.3",
    SPDXID: "SPDXRef-DOCUMENT",
    dataLicense: "CC0-1.0",
    documentNamespace: "https://anchore.com/syft/file/rad-test",
    creationInfo: {
      created: "2026-08-28T00:00:00Z",
      creators: ["Organization: Anchore, Inc", "Tool: syft-1.51.0"]
    },
    packages: [{ SPDXID: "SPDXRef-Package-radius", name: "radius" }],
    relationships: [],
    ...overrides
  });
}

test("downloads exact release assets", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const state = fixture({
      assets: [
        { id: 1, name: "one.json", contents: "one" },
        { id: 2, name: "two.json", contents: "two" }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "download",
      NAMES: '["one.json","two.json"]',
      OUTPUT_DIR: root
    });
    await releaseAssets(state);
    assert.equal(await readFile(path.join(root, "one.json"), "utf8"), "one");
    assert.equal(state.outputs.all_found, "true");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("reports optional missing assets without writing", async () => {
  const state = fixture();
  Object.assign(state.inputs, {
    OWNER: "radius-project",
    REPO: "radius",
    TAG: "v0.61.0",
    MODE: "download",
    NAMES: '["missing.json"]',
    OUTPUT_DIR: os.tmpdir(),
    OPTIONAL: "true"
  });
  await releaseAssets(state);
  assert.equal(state.outputs.all_found, "false");
});

test("reuses an identical release asset", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "same");
    const state = fixture({
      assets: [{ id: 7, name: "lock.json", contents: "same" }]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file
    });
    await releaseAssets(state);
    assert.equal(state.outputs.reused, "true");
    assert.deepEqual(state.calls.deleted, []);
    assert.deepEqual(state.calls.uploaded, []);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("replaces and verifies a changed draft asset", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "new");
    const state = fixture({
      assets: [{ id: 7, name: "lock.json", contents: "old" }]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file
    });
    await releaseAssets(state);
    assert.equal(state.outputs.reused, "false");
    assert.deepEqual(state.calls.deleted, [7]);
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("reconciles an upload accepted before a network error", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "accepted");
    const state = fixture();
    const upload = state.github.rest.repos.uploadReleaseAsset;
    state.github.rest.repos.uploadReleaseAsset = async (request) => {
      await upload(request);
      throw new Error("connection reset after upload");
    };
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file
    });
    await releaseAssets(state);
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);
    assert.equal(state.outputs.reused, "false");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("never changes an asset on a published release", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "new");
    const state = fixture({
      draft: false,
      assets: [{ id: 7, name: "lock.json", contents: "old" }]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file
    });
    await assert.rejects(() => releaseAssets(state), /Published release asset/);
    assert.deepEqual(state.calls.deleted, []);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("uploads and verifies a batch of release assets", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const one = path.join(root, "one.json");
    const two = path.join(root, "two.json");
    await writeFile(one, "one");
    await writeFile(two, "two");
    const state = fixture();
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILES: JSON.stringify([one, two])
    });
    await releaseAssets(state);
    assert.deepEqual(state.calls.uploaded, ["one.json", "two.json"]);
    assert.equal(state.outputs.uploaded_files, "2");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("verifies release binaries against split checksums", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const binary = Buffer.from("radius");
    const checksum = createHash("sha256").update(binary).digest("hex");
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [
        { id: 1, name: "rad_linux_amd64", contents: binary },
        {
          id: 2,
          name: "rad_linux_amd64.sha256",
          contents: `${checksum} *rad_linux_amd64\n`
        }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "verify-cli",
      TARGETS_FILE: targets
    });
    await releaseAssets(state);
    assert.equal(state.outputs.verified_assets, "1");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("verifies the exact release SBOM set as SPDX JSON", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [
        {
          id: 1,
          name: "rad_linux_amd64.sbom.json",
          contents: spdxDocument()
        }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "verify-sboms",
      TARGETS_FILE: targets
    });
    await releaseAssets(state);
    assert.equal(state.outputs.verified_sboms, "1");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects a malformed release SBOM", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [
        {
          id: 1,
          name: "rad_linux_amd64.sbom.json",
          contents: spdxDocument({ packages: [] })
        }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "verify-sboms",
      TARGETS_FILE: targets
    });
    await assert.rejects(() => releaseAssets(state), /valid SPDX document/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects an unexpected release SBOM asset", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [
        {
          id: 1,
          name: "rad_linux_amd64.sbom.json",
          contents: spdxDocument()
        },
        { id: 2, name: "unexpected.sbom.json", contents: spdxDocument() }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "verify-sboms",
      TARGETS_FILE: targets
    });
    await assert.rejects(() => releaseAssets(state), /expected set/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("normalizes a native GoReleaser split checksum on a draft", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const binary = Buffer.from("radius");
    const checksum = createHash("sha256").update(binary).digest("hex");
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [
        { id: 1, name: "rad_linux_amd64", contents: binary },
        { id: 2, name: "rad_linux_amd64.sha256", contents: checksum }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "normalize-cli",
      TARGETS_FILE: targets
    });
    await releaseAssets(state);
    assert.deepEqual(state.calls.deleted, [2]);
    assert.deepEqual(state.calls.uploaded, ["rad_linux_amd64.sha256"]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("recreates a checksum missing after an interrupted replacement", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const binary = Buffer.from("radius");
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [{ id: 1, name: "rad_linux_amd64", contents: binary }]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "normalize-cli",
      TARGETS_FILE: targets
    });
    await releaseAssets(state);
    assert.deepEqual(state.calls.uploaded, ["rad_linux_amd64.sha256"]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rejects a release binary with a mismatched checksum", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const targets = path.join(root, "targets.json");
    await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
    const state = fixture({
      assets: [
        { id: 1, name: "rad_linux_amd64", contents: "radius" },
        { id: 2, name: "rad_linux_amd64.sha256", contents: "0".repeat(64) }
      ]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "verify-cli",
      TARGETS_FILE: targets
    });
    await assert.rejects(() => releaseAssets(state), /checksum mismatch/);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("never replaces a changed immutable draft asset", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "new");
    const state = fixture({
      assets: [{ id: 7, name: "lock.json", contents: "old" }]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file,
      IMMUTABLE: "true"
    });
    await assert.rejects(() => releaseAssets(state), /Immutable release asset/);
    assert.deepEqual(state.calls.deleted, []);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
