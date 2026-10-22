import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import publishDraftRelease, {
  mandatoryChecks
} from "./publish-draft-release.mjs";

function fixture({
  draft = true,
  prerelease = false,
  updateFailures = []
} = {}) {
  const updates = [];
  const delays = [];
  const messages = [];
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
        getRelease: async ({ release_id }) => {
          assert.equal(release_id, release.id);
          return { data: { ...release } };
        },
        updateRelease: async (request) => {
          const retries = request.request?.retries ?? 5;
          for (let attempt = 0; ; attempt++) {
            updates.push(request);
            const failure = updateFailures.shift();
            if (!failure || failure.accepted) {
              release.draft = request.draft;
              release.prerelease = request.prerelease;
            }
            if (!failure) return { data: { ...release } };
            if (failure.error.status !== 503 || attempt >= retries) {
              throw failure.error;
            }
          }
        },
        getLatestRelease: async () => ({ data: { tag_name: release.tag_name } })
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
    delays,
    github,
    inputs,
    messages,
    outputs,
    release,
    sleep,
    updates
  };
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

for (const accepted of [false, true]) {
  test(`retries a publication ${accepted ? "accepted" : "rejected"} before an HTTP 503 response`, async () => {
    const failure = Object.assign(new Error("503 Service Unavailable"), {
      status: 503
    });
    const state = fixture({ updateFailures: [{ error: failure, accepted }] });
    const get = state.github.rest.repos.getRelease;
    const draft = { ...state.release };
    let reads = 0;
    state.github.rest.repos.getRelease = async (request) =>
      ++reads < 3 ? { data: draft } : get(request);

    await withNotes(state, () => publishDraftRelease(state));

    assert.equal(state.updates.length, 2);
    assert.deepEqual(state.updates[1], state.updates[0]);
    assert.equal(state.updates[0].request?.retries, undefined);
    assert.equal(state.updates[0].release_id, state.release.id);
    assert.equal(state.updates[0].make_latest, "true");
    assert.equal(state.release.draft, false);
    assert.equal(reads, 3);
    assert.deepEqual(state.delays, [1000, 2000]);
    assert.equal(state.outputs.release_url, state.release.html_url);
  });

  test(`bounds publication HTTP retries when the write is ${accepted ? "accepted" : "rejected"}`, async () => {
    const failure = Object.assign(new Error("503 Service Unavailable"), {
      status: 503
    });
    const state = fixture({
      updateFailures: Array(6).fill({ error: failure, accepted })
    });
    const get = state.github.rest.repos.getRelease;
    let reads = 0;
    state.github.rest.repos.getRelease = async (request) => {
      reads++;
      return get(request);
    };

    if (accepted) {
      await withNotes(state, () => publishDraftRelease(state));
      assert.equal(state.outputs.release_url, state.release.html_url);
    } else {
      await assert.rejects(
        () => withNotes(state, () => publishDraftRelease(state)),
        (error) => {
          assert.match(error.message, /not visible after 5 attempts/);
          assert.equal(error.cause, failure);
          return true;
        }
      );
      assert.equal(state.outputs.release_url, undefined);
    }

    assert.equal(state.updates.length, 6);
    for (const request of state.updates) {
      assert.deepEqual(request, state.updates[0]);
    }
    assert.equal(state.release.draft, !accepted);
    assert.equal(reads, accepted ? 1 : 5);
    assert.deepEqual(state.delays, accepted ? [] : [1000, 2000, 4000, 8000]);
  });
}

for (const responseLost of [false, true]) {
  test(`waits for publication by ID when the update response is ${responseLost ? "lost" : "received"}`, async () => {
    const state = fixture();
    const draft = { ...state.release };
    const get = state.github.rest.repos.getRelease;
    const update = state.github.rest.repos.updateRelease;
    let listings = 0;
    let reads = 0;
    state.github.rest.repos.listReleases = async () => {
      assert.equal(++listings, 1, "must verify publication by release ID");
      return [state.release];
    };
    state.github.rest.repos.getRelease = async (request) =>
      ++reads < 3 ? { data: draft } : get(request);
    state.github.rest.repos.updateRelease = async (request) => {
      assert.equal(request.request?.retries, undefined);
      const response = await update(request);
      if (responseLost) throw new Error("connection reset after publication");
      return response;
    };
    await withNotes(state, () => publishDraftRelease(state));
    assert.deepEqual(state.delays, [1000, 2000]);
    assert.equal(state.updates.length, 1);
    assert.equal(state.outputs.release_url, state.release.html_url);
  });
}

test("waits for a release missing from the initial publication lookup", async () => {
  const state = fixture();
  const list = state.github.rest.repos.listReleases;
  let listings = 0;
  state.github.rest.repos.listReleases = async () =>
    ++listings < 3 ? [] : list();
  await withNotes(state, () => publishDraftRelease(state));
  assert.equal(listings, 3);
  assert.deepEqual(state.delays, [1000]);
  assert.equal(state.updates.length, 1);
});

for (const newerTag of ["v0.61.1", "v0.62.0"]) {
  test(`does not replace latest ${newerTag} after recovering a missing release`, async () => {
    const state = fixture();
    const newer = { tag_name: newerTag, draft: false, prerelease: false };
    const update = state.github.rest.repos.updateRelease;
    let latest = newer;
    let listings = 0;
    let latestReads = 0;
    state.github.rest.repos.listReleases = async () =>
      ++listings < 3 ? [] : [state.release, newer];
    state.github.rest.repos.updateRelease = async (request) => {
      const response = await update(request);
      if (request.make_latest === "true") latest = state.release;
      return response;
    };
    state.github.rest.repos.getLatestRelease = async () => {
      latestReads++;
      return { data: latest };
    };

    await withNotes(state, () => publishDraftRelease(state));

    assert.equal(listings, 3);
    assert.deepEqual(state.delays, [1000]);
    assert.equal(state.updates.length, 1);
    assert.equal(state.updates[0].make_latest, "false");
    assert.equal(state.release.draft, false);
    assert.equal(latest, newer);
    assert.equal(latestReads, 0);
    assert.equal(state.outputs.release_url, state.release.html_url);
  });
}

test("waits for a post-publication 404 without repeating the update", async () => {
  const state = fixture();
  const get = state.github.rest.repos.getRelease;
  let reads = 0;
  state.github.rest.repos.getRelease = async (request) => {
    if (++reads < 3) {
      throw Object.assign(new Error("not visible yet"), { status: 404 });
    }
    return get(request);
  };
  await withNotes(state, () => publishDraftRelease(state));
  assert.deepEqual(state.delays, [1000, 2000]);
  assert.equal(state.updates.length, 1);
});

test("bounds publication readback and retains the original update error", async () => {
  const state = fixture();
  const failure = new Error("connection reset during publication");
  state.github.rest.repos.updateRelease = async (request) => {
    state.updates.push(request);
    throw failure;
  };
  await assert.rejects(
    () => withNotes(state, () => publishDraftRelease(state)),
    (error) => {
      assert.match(
        error.message,
        /Published release v0\.61\.0 \(ID 42\).*not visible after 5 attempts/
      );
      assert.equal(error.cause, failure);
      return true;
    }
  );
  assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
  assert.equal(state.updates.length, 1);
  assert.equal(state.outputs.release_url, undefined);
});

test("propagates a forbidden publication without reading or waiting", async () => {
  const state = fixture();
  const failure = Object.assign(new Error("publication forbidden"), {
    status: 403
  });
  let reads = 0;
  state.github.rest.repos.updateRelease = async (request) => {
    state.updates.push(request);
    throw failure;
  };
  state.github.rest.repos.getRelease = async () => {
    reads++;
    return { data: { ...state.release } };
  };
  await assert.rejects(
    () => withNotes(state, () => publishDraftRelease(state)),
    (error) => error === failure
  );
  assert.equal(reads, 0);
  assert.equal(state.updates.length, 1);
  assert.equal(state.release.draft, true);
  assert.deepEqual(state.delays, []);
  assert.deepEqual(state.messages, []);
  assert.equal(state.outputs.release_url, undefined);
});

for (const [change, expected] of [
  [{ prerelease: true }, /wrong classification/],
  [{ tag_name: "v0.62.0" }, /does not match tag v0\.61\.0/]
]) {
  test(`rejects conflicting published ${Object.keys(change)[0]} without treating it as a visibility delay`, async () => {
    const state = fixture();
    state.github.rest.repos.getRelease = async () => ({
      data: { ...state.release, draft: false, ...change }
    });
    await assert.rejects(
      () => withNotes(state, () => publishDraftRelease(state)),
      expected
    );
    assert.deepEqual(state.delays, []);
    assert.equal(state.updates.length, 1);
    assert.equal(state.outputs.release_url, undefined);
  });
}

test("does not hide forbidden publication readback", async () => {
  const state = fixture();
  const failure = Object.assign(new Error("forbidden"), { status: 403 });
  state.github.rest.repos.getRelease = async () => {
    throw failure;
  };
  await assert.rejects(
    () => withNotes(state, () => publishDraftRelease(state)),
    (error) => error === failure
  );
  assert.deepEqual(state.delays, []);
  assert.equal(state.updates.length, 1);
});

for (const missing of [false, true]) {
  test(`waits for latest-release ${missing ? "visibility" : "freshness"} after publication`, async () => {
    const state = fixture();
    let reads = 0;
    state.github.rest.repos.getLatestRelease = async () => {
      if (++reads < 3) {
        if (missing) {
          throw Object.assign(new Error("not visible yet"), { status: 404 });
        }
        return { data: { tag_name: "v0.60.0" } };
      }
      return { data: { ...state.release } };
    };
    await withNotes(state, () => publishDraftRelease(state));
    assert.deepEqual(state.delays, [1000, 2000]);
    assert.equal(state.updates.length, 1);
    assert.equal(state.outputs.release_url, state.release.html_url);
  });
}

test("fails when latest never reaches the requested release without publishing again", async () => {
  const state = fixture();
  state.github.rest.repos.getLatestRelease = async () => ({
    data: { tag_name: "v0.60.0" }
  });
  await assert.rejects(
    () => withNotes(state, () => publishDraftRelease(state)),
    /Latest release v0\.61\.0.*not visible after 5 attempts/
  );
  assert.deepEqual(state.delays, [1000, 2000, 4000, 8000]);
  assert.equal(state.updates.length, 1);
  assert.equal(state.outputs.release_url, undefined);
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
