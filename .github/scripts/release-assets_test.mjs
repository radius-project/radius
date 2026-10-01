import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import releaseAssets, { readBicepExtensionLock } from "./release-assets.mjs";
import { bicepExtensionLockName } from "./release-bicep-extensions.mjs";

function fixture({
  draft = true,
  assets = [],
  starterFailures = 0,
  uploadFailures = []
} = {}) {
  const outputs = {};
  const calls = {
    deleted: [],
    downloaded: [],
    uploaded: [],
    uploadAttempts: []
  };
  const delays = [];
  const messages = [];
  let nextID = 100;
  const release = { id: 42, tag_name: "v0.61.0", draft };
  const contents = new Map(
    assets.map((asset) => [asset.id, Buffer.from(asset.contents)])
  );
  const currentAssets = assets.map(({ id, name, state = "uploaded" }) => ({
    id,
    name,
    state
  }));
  const github = {
    paginate: async (method) => method(),
    request: async (_route, { asset_id }) => {
      calls.downloaded.push(asset_id);
      if (
        currentAssets.find((asset) => asset.id === asset_id)?.state ===
        "starter"
      ) {
        throw Object.assign(new Error("Asset is not uploaded"), {
          status: 404
        });
      }
      return { data: contents.get(asset_id) };
    },
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
        uploadReleaseAsset: async (request) => {
          const { name, data } = request;
          const retries = request.request?.retries ?? 5;
          for (let attempt = 0; ; attempt++) {
            calls.uploadAttempts.push(request);
            try {
              if (currentAssets.some((asset) => asset.name === name)) {
                throw Object.assign(new Error(`Asset ${name} already exists`), {
                  status: 422
                });
              }
              const failure = uploadFailures.shift();
              if (failure && !failure.accepted) throw failure.error;
              const incomplete = starterFailures > 0;
              const asset = {
                id: nextID++,
                name,
                state: incomplete ? "starter" : "uploaded"
              };
              calls.uploaded.push(name);
              currentAssets.push(asset);
              contents.set(
                asset.id,
                incomplete ? Buffer.alloc(0) : Buffer.from(data)
              );
              if (incomplete) {
                starterFailures--;
                throw Object.assign(new Error("502 Bad Gateway"), {
                  status: 502
                });
              }
              if (failure) throw failure.error;
              return { data: asset };
            } catch (error) {
              if (![502, 503].includes(error.status) || attempt >= retries) {
                throw error;
              }
            }
          }
        }
      }
    }
  };
  const inputs = {};
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
  return { calls, core, delays, github, inputs, messages, outputs, sleep };
}

async function withUpload(state, callback) {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "complete");
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file,
      IMMUTABLE: "true"
    });
    return await callback();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

function spdxPackage(overrides = {}) {
  return {
    SPDXID: "SPDXRef-Package-radius",
    name: "radius",
    downloadLocation: "NOASSERTION",
    ...overrides
  };
}

function spdxDocument(overrides = {}) {
  return JSON.stringify({
    spdxVersion: "SPDX-2.3",
    SPDXID: "SPDXRef-DOCUMENT",
    dataLicense: "CC0-1.0",
    name: "rad_linux_amd64",
    documentNamespace: "https://anchore.com/syft/file/rad-test",
    creationInfo: {
      created: "2026-08-28T00:00:00Z",
      creators: ["Organization: Anchore, Inc", "Tool: syft-1.51.0"]
    },
    packages: [spdxPackage()],
    relationships: [],
    ...overrides
  });
}

test("Bicep reconciliation distinguishes unstaged drafts from missing published locks", async () => {
  const draft = fixture();
  assert.equal(
    await readBicepExtensionLock(draft.github, "v0.61.0"),
    undefined
  );
  await assert.rejects(
    readBicepExtensionLock(fixture({ draft: false }).github, "v0.61.0"),
    /Published GHCR release is missing/
  );
  const locked = fixture({
    draft: false,
    assets: [
      { id: 1, name: bicepExtensionLockName, contents: '{"schemaVersion":1}' }
    ]
  });
  assert.deepEqual(await readBicepExtensionLock(locked.github, "v0.61.0"), {
    schemaVersion: 1
  });
  const duplicate = fixture({
    assets: [
      { id: 1, name: bicepExtensionLockName, contents: "{}" },
      { id: 2, name: bicepExtensionLockName, contents: "{}" }
    ]
  });
  await assert.rejects(
    readBicepExtensionLock(duplicate.github, "v0.61.0"),
    /multiple assets/
  );
  draft.github.rest.repos.listReleases = async () => {
    throw new Error("denied");
  };
  await assert.rejects(
    readBicepExtensionLock(draft.github, "v0.61.0"),
    /denied/
  );
});

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

test("optional staging downloads recover locks from a previous attempt", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const locks = [
      { id: 1, name: "production-image-digests.json", contents: "image lock" },
      { id: 2, name: "release-cli-oci.json", contents: "CLI lock" }
    ];
    const state = fixture({ assets: locks });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "download",
      OUTPUT_DIR: root,
      OPTIONAL: "true"
    });
    for (const lock of locks) {
      state.inputs.NAMES = JSON.stringify([lock.name]);
      await releaseAssets(state);
      assert.equal(state.outputs.all_found, "true");
      assert.equal(
        await readFile(path.join(root, lock.name), "utf8"),
        lock.contents
      );
    }
    assert.deepEqual(state.calls.deleted, []);
    assert.deepEqual(state.calls.uploaded, []);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("Bicep release evidence cannot be optional or overwritten on a draft", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "bicep-release-lock-"));
  try {
    const state = fixture();
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "download",
      NAMES: JSON.stringify([bicepExtensionLockName]),
      OUTPUT_DIR: root,
      OPTIONAL: "true"
    });
    await assert.rejects(() => releaseAssets(state), /missing assets/);
    const file = path.join(root, bicepExtensionLockName);
    await writeFile(file, "original");
    Object.assign(state.inputs, { MODE: "upload", FILE: file });
    await releaseAssets(state);
    await releaseAssets(state);
    assert.equal(state.outputs.reused, "true");
    await writeFile(file, "replacement");
    await assert.rejects(() => releaseAssets(state), /Immutable release asset/);
    assert.deepEqual(state.calls.deleted, []);
    assert.deepEqual(state.calls.uploaded, [bicepExtensionLockName]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
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

test("Bicep release evidence recovers an incomplete upload without replacing completed content", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "bicep-release-lock-"));
  try {
    const file = path.join(root, bicepExtensionLockName);
    await writeFile(file, "complete");
    const state = fixture({ starterFailures: 1 });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file
    });
    await releaseAssets(state);
    assert.deepEqual(state.calls.deleted, [100]);
    assert.deepEqual(state.calls.uploaded, [
      bicepExtensionLockName,
      bicepExtensionLockName
    ]);
    await releaseAssets(state);
    assert.equal(state.outputs.reused, "true");
    await writeFile(file, "replacement");
    await assert.rejects(() => releaseAssets(state), /Immutable release asset/);
    assert.deepEqual(state.calls.deleted, [100]);
    assert.equal(state.calls.uploaded.length, 2);
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

test("retries an upload rejected with HTTP 503 before acceptance", async () => {
  const failure = Object.assign(new Error("503 Service Unavailable"), {
    status: 503
  });
  const state = fixture({ uploadFailures: [{ error: failure }] });

  await withUpload(state, () => releaseAssets(state));

  assert.equal(state.calls.uploadAttempts.length, 2);
  assert.deepEqual(
    state.calls.uploadAttempts[1],
    state.calls.uploadAttempts[0]
  );
  assert.equal(state.calls.uploadAttempts[0].request?.retries, undefined);
  assert.equal(state.calls.uploadAttempts[0].release_id, 42);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.deepEqual(state.calls.downloaded, [100]);
  assert.deepEqual(state.calls.deleted, []);
  assert.deepEqual(state.delays, []);
  assert.equal(state.outputs.asset_id, "100");
});

for (const matching of [true, false]) {
  test(`${matching ? "adopts matching" : "rejects conflicting"} bytes after an accepted upload retries into HTTP 422`, async () => {
    const failure = Object.assign(new Error("503 Service Unavailable"), {
      status: 503
    });
    const state = fixture({
      uploadFailures: [{ error: failure, accepted: true }]
    });
    const list = state.github.rest.repos.listReleaseAssets;
    const download = state.github.request;
    let listings = 0;
    state.github.rest.repos.listReleaseAssets = async () =>
      ++listings < 3 ? [] : list();
    state.github.request = async (...args) => {
      const response = await download(...args);
      return matching ? response : { data: Buffer.from("different") };
    };

    if (matching) {
      await withUpload(state, () => releaseAssets(state));
      assert.equal(state.outputs.asset_id, "100");
      assert.equal(state.outputs.uploaded_files, "1");
    } else {
      await assert.rejects(
        () => withUpload(state, () => releaseAssets(state)),
        (error) => {
          assert.equal(error.status, 422);
          assert.match(error.message, /lock\.json already exists/);
          return true;
        }
      );
      assert.equal(state.outputs.asset_id, undefined);
      assert.equal(state.outputs.uploaded_files, undefined);
    }

    assert.equal(listings, 3);
    assert.equal(state.calls.uploadAttempts.length, 2);
    assert.deepEqual(
      state.calls.uploadAttempts[1],
      state.calls.uploadAttempts[0]
    );
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);
    assert.deepEqual(state.calls.downloaded, matching ? [100, 100] : [100]);
    assert.deepEqual(state.calls.deleted, []);
    assert.deepEqual(state.delays, [1000]);
    assert.deepEqual(await list(), [
      { id: 100, name: "lock.json", state: "uploaded" }
    ]);
  });
}

test("bounds upload HTTP retries before polling for uncertain state", async () => {
  const failure = Object.assign(new Error("503 Service Unavailable"), {
    status: 503
  });
  const state = fixture({
    uploadFailures: Array(6).fill({ error: failure })
  });
  const list = state.github.rest.repos.listReleaseAssets;
  let listings = 0;
  state.github.rest.repos.listReleaseAssets = async () => {
    listings++;
    return list();
  };

  await assert.rejects(
    () => withUpload(state, () => releaseAssets(state)),
    (error) => {
      assert.match(error.message, /lock\.json.*not visible after 5 attempts/);
      assert.equal(error.cause, failure);
      return true;
    }
  );

  assert.equal(state.calls.uploadAttempts.length, 6);
  for (const request of state.calls.uploadAttempts) {
    assert.deepEqual(request, state.calls.uploadAttempts[0]);
  }
  assert.equal(listings, 6);
  assert.deepEqual(state.calls.uploaded, []);
  assert.deepEqual(state.calls.downloaded, []);
  assert.deepEqual(state.calls.deleted, []);
  assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
  assert.equal(state.outputs.asset_id, undefined);
});

test("retries an immutable draft upload after removing a starter asset", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "complete");
    const state = fixture({ starterFailures: 1 });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file,
      IMMUTABLE: "true"
    });
    await releaseAssets(state);
    assert.deepEqual(state.calls.deleted, [100]);
    assert.deepEqual(state.calls.uploaded, ["lock.json", "lock.json"]);
    assert.equal(state.calls.uploadAttempts.length, 3);
    assert.equal(state.outputs.asset_id, "101");
    assert.equal(state.outputs.reused, "false");
    assert.deepEqual(await state.github.rest.repos.listReleaseAssets(), [
      { id: 101, name: "lock.json", state: "uploaded" }
    ]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("bounds starter upload retries and permits a later rerun", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "complete");
    const state = fixture({ starterFailures: 3 });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file,
      IMMUTABLE: "true"
    });
    await assert.rejects(
      () => releaseAssets(state),
      (error) => {
        assert.equal(error.status, 422);
        assert.match(error.message, /lock\.json already exists/);
        return true;
      }
    );
    assert.equal(state.calls.uploaded.length, 3);
    assert.equal(state.calls.uploadAttempts.length, 6);
    assert.deepEqual(state.calls.deleted, [100, 101, 102]);
    assert.equal(state.outputs.uploaded_files, undefined);

    await releaseAssets(state);
    assert.equal(state.calls.uploaded.length, 4);
    assert.equal(state.calls.uploadAttempts.length, 7);
    assert.equal(state.outputs.asset_id, "103");
    assert.equal(state.outputs.uploaded_files, "1");
    assert.deepEqual(await state.github.rest.repos.listReleaseAssets(), [
      { id: 103, name: "lock.json", state: "uploaded" }
    ]);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

for (const localContents of ["complete", ""]) {
  test(`replaces a pre-existing draft starter for ${localContents ? "nonempty" : "empty"} data`, async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
    try {
      const file = path.join(root, "lock.json");
      await writeFile(file, localContents);
      const state = fixture({
        assets: [{ id: 7, name: "lock.json", state: "starter", contents: "" }]
      });
      Object.assign(state.inputs, {
        OWNER: "radius-project",
        REPO: "radius",
        TAG: "v0.61.0",
        MODE: "upload",
        FILE: file,
        IMMUTABLE: "true"
      });
      await releaseAssets(state);
      assert.deepEqual(state.calls.deleted, [7]);
      assert.deepEqual(state.calls.uploaded, ["lock.json"]);
      assert.equal(state.outputs.reused, "false");
      assert.equal(state.outputs.asset_id, "100");
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
}

test("never removes a starter asset from a published release", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
  try {
    const file = path.join(root, "lock.json");
    await writeFile(file, "complete");
    const state = fixture({
      draft: false,
      assets: [{ id: 7, name: "lock.json", state: "starter", contents: "" }]
    });
    Object.assign(state.inputs, {
      OWNER: "radius-project",
      REPO: "radius",
      TAG: "v0.61.0",
      MODE: "upload",
      FILE: file,
      IMMUTABLE: "true"
    });
    await assert.rejects(() => releaseAssets(state), /published release/i);
    assert.deepEqual(state.calls.deleted, []);
    assert.deepEqual(state.calls.uploaded, []);
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

test("waits for a newly staged release before uploading an asset", async () => {
  const state = fixture();
  const list = state.github.rest.repos.listReleases;
  let listings = 0;
  state.github.rest.repos.listReleases = async () =>
    ++listings < 3 ? [] : list();
  await withUpload(state, () => releaseAssets(state));
  assert.deepEqual(state.delays, [1000, 2000]);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.equal(state.outputs.asset_id, "100");
});

test("verifies an accepted upload by ID while asset listings are stale", async () => {
  const state = fixture();
  let listings = 0;
  state.github.rest.repos.listReleaseAssets = async () => {
    assert.equal(++listings, 1, "must not rediscover an accepted upload");
    return [];
  };
  const upload = state.github.rest.repos.uploadReleaseAsset;
  state.github.rest.repos.uploadReleaseAsset = async (request) => {
    assert.equal(request.request?.retries, undefined);
    return upload(request);
  };
  await withUpload(state, () => releaseAssets(state));
  assert.deepEqual(state.delays, []);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.equal(state.outputs.asset_id, "100");
});

for (const responseLost of [false, true]) {
  test(`waits for uploaded asset content when the response is ${responseLost ? "lost" : "received"}`, async () => {
    const state = fixture();
    const request = state.github.request;
    let downloads = 0;
    state.github.request = async (...args) => {
      if (++downloads < 3) {
        throw Object.assign(new Error("not visible yet"), { status: 404 });
      }
      return request(...args);
    };
    if (responseLost) {
      const upload = state.github.rest.repos.uploadReleaseAsset;
      state.github.rest.repos.uploadReleaseAsset = async (input) => {
        await upload(input);
        throw new Error("connection reset after upload");
      };
    }
    await withUpload(state, () => releaseAssets(state));
    assert.deepEqual(state.delays, [1000, 2000]);
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);
    assert.deepEqual(state.calls.deleted, []);
    assert.equal(state.outputs.asset_id, "100");
  });
}

test("waits for an upload with a lost response to appear in asset listings", async () => {
  const state = fixture();
  const list = state.github.rest.repos.listReleaseAssets;
  const upload = state.github.rest.repos.uploadReleaseAsset;
  let listings = 0;
  state.github.rest.repos.listReleaseAssets = async () =>
    ++listings < 4 ? [] : list();
  state.github.rest.repos.uploadReleaseAsset = async (request) => {
    await upload(request);
    throw new Error("connection reset after upload");
  };
  await withUpload(state, () => releaseAssets(state));
  assert.equal(listings, 4);
  assert.deepEqual(state.delays, [1000, 2000]);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.equal(state.outputs.asset_id, "100");
});

test("preserves an uncertain upload on visibility timeout and reuses it on resume", async () => {
  const state = fixture();
  const list = state.github.rest.repos.listReleaseAssets;
  const upload = state.github.rest.repos.uploadReleaseAsset;
  const failure = new Error("connection reset after upload");
  state.github.rest.repos.listReleaseAssets = async () => [];
  state.github.rest.repos.uploadReleaseAsset = async (request) => {
    await upload(request);
    throw failure;
  };
  await withUpload(state, async () => {
    await assert.rejects(
      () => releaseAssets(state),
      (error) => {
        assert.match(error.message, /lock\.json.*not visible after 5 attempts/);
        assert.equal(error.cause, failure);
        return true;
      }
    );
    assert.equal(state.outputs.asset_id, undefined);
    assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
    assert.deepEqual(state.calls.deleted, []);
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);

    state.github.rest.repos.listReleaseAssets = list;
    await releaseAssets(state);
    assert.equal(state.outputs.asset_id, "100");
    assert.equal(state.outputs.reused, "true");
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  });
});

test("bounds content visibility retries without deleting an accepted upload", async () => {
  const state = fixture();
  state.github.request = async () => {
    throw Object.assign(new Error("not visible yet"), { status: 404 });
  };
  await assert.rejects(
    () => withUpload(state, () => releaseAssets(state)),
    /lock\.json \(ID 100\).*not visible after 5 attempts/
  );
  assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.deepEqual(state.calls.deleted, []);
  assert.equal(state.outputs.asset_id, undefined);
});

for (const status of [403, 500]) {
  test(`does not treat HTTP ${status} asset download errors as visibility delays`, async () => {
    const state = fixture();
    const failure = Object.assign(new Error("download failed"), { status });
    state.github.request = async () => {
      throw failure;
    };
    await assert.rejects(
      () => withUpload(state, () => releaseAssets(state)),
      (error) => error === failure
    );
    assert.deepEqual(state.delays, []);
    assert.deepEqual(state.calls.uploaded, ["lock.json"]);
    assert.deepEqual(state.calls.deleted, []);
  });
}

test("rejects changed uploaded bytes without retrying or replacing the asset", async () => {
  const state = fixture();
  state.github.request = async () => ({ data: Buffer.from("different") });
  await assert.rejects(
    () => withUpload(state, () => releaseAssets(state)),
    /failed verification/
  );
  assert.deepEqual(state.delays, []);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.deepEqual(state.calls.deleted, []);
  assert.equal(state.outputs.asset_id, undefined);
});

test("rejects an upload response with a different asset name", async () => {
  const state = fixture();
  const upload = state.github.rest.repos.uploadReleaseAsset;
  state.github.rest.repos.uploadReleaseAsset = async (request) => {
    const response = await upload(request);
    return { data: { ...response.data, name: "unexpected.json" } };
  };
  await assert.rejects(
    () => withUpload(state, () => releaseAssets(state)),
    /asset 100 is not named lock\.json/
  );
  assert.deepEqual(state.delays, []);
  assert.deepEqual(state.calls.uploaded, ["lock.json"]);
  assert.deepEqual(state.calls.deleted, []);
  assert.equal(state.outputs.asset_id, undefined);
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

for (const downloadLocation of [
  "NOASSERTION",
  "NONE",
  "https://example.test/rad"
]) {
  test(`verifies the release SBOM set with downloadLocation ${downloadLocation}`, async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
    try {
      const targets = path.join(root, "targets.json");
      await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
      const state = fixture({
        assets: [
          {
            id: 1,
            name: "rad_linux_amd64.sbom.json",
            contents: spdxDocument({
              packages: [spdxPackage({ downloadLocation })]
            })
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
}

const malformedSpdxCases = [
  ["empty packages", { packages: [] }],
  ["missing document name", { name: undefined }],
  ["mistyped document name", { name: 42 }],
  ["empty package object", { packages: [{}] }],
  ["null package", { packages: [null] }],
  ["string package", { packages: ["radius"] }],
  ["array package", { packages: [[]] }],
  ["invalid second package", { packages: [spdxPackage(), {}] }]
];
for (const field of ["SPDXID", "name", "downloadLocation"]) {
  malformedSpdxCases.push(
    [
      `missing package ${field}`,
      { packages: [spdxPackage({ [field]: undefined })] }
    ],
    [`mistyped package ${field}`, { packages: [spdxPackage({ [field]: 42 })] }]
  );
}

for (const [description, overrides] of malformedSpdxCases) {
  test(`rejects a release SBOM with ${description}`, async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), "release-assets-"));
    try {
      const targets = path.join(root, "targets.json");
      await writeFile(targets, '{"cliAssets":[{"name":"rad_linux_amd64"}]}');
      const state = fixture({
        assets: [
          {
            id: 1,
            name: "rad_linux_amd64.sbom.json",
            contents: spdxDocument(overrides)
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
      assert.equal(state.outputs.verified_sboms, undefined);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
}

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
