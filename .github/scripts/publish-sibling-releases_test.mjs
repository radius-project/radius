import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import publishSiblingReleases, {
  notesBase,
  publishRepositoryRelease,
  releaseTargets
} from "./publish-sibling-releases.mjs";
import { mandatoryChecks } from "./publish-draft-release.mjs";

function fixture(releaseType = "final", version = "v0.62.0") {
  const sourceSha = "a".repeat(40);
  const names = ["recipes", "dashboard", "bicep-types-aws"];
  const plan = {
    schemaVersion: 2,
    version,
    releaseType,
    channel: "0.62",
    previousVersion: releaseType === "patch" ? "v0.62.0" : "v0.62.0-rc.1",
    siblingRepositories: names.map((name, index) => ({
      name,
      repository: `radius-project/${name}`,
      sourceCommit: String(index + 1).repeat(40)
    }))
  };
  const manifest = {
    schemaVersion: 1,
    tag: version,
    sourceSha,
    releaseType,
    channel: "0.62",
    planSha256: createHash("sha256").update(JSON.stringify(plan)).digest("hex"),
    checks: Object.fromEntries(
      mandatoryChecks.map((name) => [name, "verified"])
    ),
    observed: {
      imageLocks: [{ name: "deployment-engine", sourceSha: "4".repeat(40) }]
    }
  };
  const stores = new Map();
  const calls = [];
  const sleeps = [];
  for (const target of [
    ...releaseTargets(plan, manifest, sourceSha),
    { owner: "radius-project", repo: "radius", sha: sourceSha }
  ]) {
    const stable = {
      id: 1,
      tag_name: "v0.61.1",
      draft: false,
      prerelease: false
    };
    stores.set(`${target.owner}/${target.repo}`, {
      sha: target.sha,
      releases: [stable],
      latest: stable,
      tags: [
        { name: "v0.61.1" },
        { name: plan.previousVersion },
        { name: version }
      ]
    });
  }
  const client = (allowed) => {
    const operation = (method, fn) => async (request) => {
      const key = `${request.owner}/${request.repo}`;
      assert.ok(
        allowed.includes(key),
        `${method} used the wrong authenticated client for ${key}`
      );
      calls.push({ method, ...request });
      return fn(stores.get(key), request);
    };
    return {
      paginate: async (method, request) => method(request),
      rest: {
        git: {
          getRef: operation("getRef", (store) => ({
            data: { object: { type: "commit", sha: store.sha } }
          }))
        },
        repos: {
          getContent: operation("getContent", () => ({
            data: {
              encoding: "base64",
              content: Buffer.from("name: Build\n").toString("base64")
            }
          })),
          listReleases: operation("listReleases", (store) =>
            structuredClone(store.releases)
          ),
          listTags: operation("listTags", (store) => store.tags),
          generateReleaseNotes: operation(
            "generateReleaseNotes",
            (_, request) => ({
              data: {
                name: request.tag_name,
                body: `## What's Changed\n\nGitHub-generated notes since ${request.previous_tag_name ?? "the beginning"}.\n`
              }
            })
          ),
          createRelease: operation("createRelease", (store, request) => {
            assert.equal(request.request.retries, 0);
            const release = {
              ...request,
              id: 2,
              html_url: `https://github.com/${request.owner}/${request.repo}/releases/tag/${request.tag_name}`
            };
            store.releases.push(release);
            return { data: structuredClone(release) };
          }),
          updateRelease: operation("updateRelease", (store, request) => {
            assert.equal(request.request, undefined);
            const release = store.releases.find(
              (entry) => entry.id === request.release_id
            );
            Object.assign(release, {
              draft: request.draft,
              prerelease: request.prerelease
            });
            if (request.make_latest === "true") store.latest = release;
            return { data: structuredClone(release) };
          }),
          getRelease: operation("getRelease", (store, request) => ({
            data: structuredClone(
              store.releases.find((entry) => entry.id === request.release_id)
            )
          })),
          getLatestRelease: operation("getLatestRelease", (store) => ({
            data: structuredClone(store.latest)
          })),
          getReleaseByTag: operation("getReleaseByTag", () => ({
            data: {
              tag_name: version,
              draft: false,
              prerelease: releaseType === "rc"
            }
          }))
        }
      }
    };
  };
  const outputs = {};
  const options = {
    github: client(["radius-project/radius"]),
    siblings: client(names.map((name) => `radius-project/${name}`)),
    engine: client(["azure-octo/deployment-engine"]),
    context: {
      repo: { owner: "radius-project", repo: "radius" },
      ref: `refs/tags/${version}`,
      sha: sourceSha
    },
    core: {
      info() {},
      setOutput: (name, value) => {
        outputs[name] = value;
      }
    },
    plan,
    manifest,
    sleep: async (delay) => {
      sleeps.push(delay);
    }
  };
  const one = {
    github: options.siblings,
    core: options.core,
    plan,
    sleep: options.sleep,
    target: releaseTargets(plan, manifest, sourceSha)[0]
  };
  return { options, one, calls, stores, outputs, sleeps };
}

function dashboardFixture() {
  const state = fixture();
  const { one, calls, stores } = state;
  one.target = releaseTargets(
    state.options.plan,
    state.options.manifest,
    state.options.context.sha
  )[1];
  const archive = `radius-backstage-plugin-${one.plan.version}.tar.gz`;
  const bundle = Buffer.from("the exact tested plugin bundle");
  state.files = {
    [archive]: bundle,
    [`${archive}.sha256`]: `${createHash("sha256").update(bundle).digest("hex")}  ${archive}\n`,
    [`radius-backstage-plugin-${one.plan.version}.qualification.json`]:
      JSON.stringify({
        stage: "installed-external-frontend",
        bundle: { source: { revision: one.target.sha, dirty: false } }
      })
  };
  state.runs = [
    {
      id: 7,
      head_sha: one.target.sha,
      head_branch: one.plan.version,
      event: "push",
      status: "completed",
      conclusion: "success",
      repository: { full_name: "radius-project/dashboard" }
    }
  ];
  state.artifacts = [
    { id: 8, name: "radius-plugin-release-assets", expired: false }
  ];
  state.assets = [];
  one.github.rest.repos.getContent = async (request) => {
    assert.equal(request.ref, one.target.sha);
    assert.equal(request.path, ".github/workflows/build.yaml");
    return {
      data: {
        encoding: "base64",
        content: Buffer.from("name: radius-plugin-release-assets\n").toString(
          "base64"
        )
      }
    };
  };
  one.github.auth = async () => ({ token: "sibling-token" });
  one.github.rest.actions = {
    listWorkflowRuns: async (request) => {
      calls.push({ method: "listWorkflowRuns", ...request });
      assert.equal(request.workflow_id, "build.yaml");
      assert.equal(request.head_sha, one.target.sha);
      assert.equal(request.event, "push");
      assert.equal(request.status, "success");
      return state.runs;
    },
    listWorkflowRunArtifacts: async (request) => {
      assert.equal(request.run_id, 7);
      return state.artifacts;
    }
  };
  one.github.rest.repos.listReleaseAssets = async () => state.assets;
  one.execute = async (command, args, options) => {
    assert.equal(command, "gh");
    assert.equal(options.env.GH_TOKEN, "sibling-token");
    assert.equal(args[args.indexOf("--repo") + 1], "radius-project/dashboard");
    calls.push({ method: args[0], args });
    if (args[0] === "run") {
      assert.deepEqual(args.slice(0, 4), ["run", "download", "7", "--repo"]);
      assert.equal(
        args[args.indexOf("--name") + 1],
        "radius-plugin-release-assets"
      );
      state.directory = args.at(-1);
      for (const [name, data] of Object.entries(state.files)) {
        await writeFile(path.join(state.directory, name), data);
      }
    } else {
      assert.deepEqual(args.slice(0, 3), [
        "release",
        "upload",
        one.plan.version
      ]);
      assert.ok(args.includes("--clobber"));
      for (const name of Object.keys(state.files)) {
        const file = path.join(state.directory, name);
        assert.ok(args.includes(file));
        assert.deepEqual(await readFile(file), Buffer.from(state.files[name]));
      }
      state.assets = Object.keys(state.files).map((name) => ({
        name,
        state: "uploaded",
        size: Buffer.byteLength(state.files[name])
      }));
    }
  };
  const update = one.github.rest.repos.updateRelease;
  one.github.rest.repos.updateRelease = async (request) => {
    assert.equal(
      state.assets.length,
      3,
      "all assets must be attached before publication"
    );
    return update(request);
  };
  state.store = stores.get("radius-project/dashboard");
  return state;
}

test("attaches the exact qualified Dashboard bundle after draft creation and before publication", async () => {
  const state = dashboardFixture();
  await publishRepositoryRelease(state.one);
  const methods = state.calls.map((call) => call.method);
  assert.ok(methods.indexOf("createRelease") < methods.indexOf("run"));
  assert.ok(methods.indexOf("release") < methods.indexOf("updateRelease"));
  await assert.rejects(
    readFile(path.join(state.directory, Object.keys(state.files)[0])),
    { code: "ENOENT" }
  );
  // Published releases remain resumable after the workflow artifact expires.
  state.artifacts = [];
  await publishRepositoryRelease(state.one);
  assert.equal(state.calls.filter((call) => call.method === "run").length, 1);
});

test("resumes a Dashboard draft after a failed upload without publishing or replacing notes", async () => {
  const state = dashboardFixture();
  const execute = state.one.execute;
  state.one.execute = async (command, args, options) => {
    if (args[0] === "release") throw new Error("upload failed");
    return execute(command, args, options);
  };
  await assert.rejects(publishRepositoryRelease(state.one), /upload failed/);
  assert.equal(state.store.releases[1].draft, true);
  assert.equal(
    state.calls.some((call) => call.method === "updateRelease"),
    false
  );
  await assert.rejects(
    readFile(path.join(state.directory, Object.keys(state.files)[0])),
    { code: "ENOENT" }
  );
  state.one.execute = execute;
  await publishRepositoryRelease(state.one);
  assert.equal(state.store.releases[1].draft, false);
  assert.equal(
    state.calls.filter((call) => call.method === "createRelease").length,
    1
  );
  assert.equal(
    state.calls.filter((call) => call.method === "generateReleaseNotes").length,
    1
  );
});

test("Dashboard never publishes with missing, expired, ambiguous, or unqualified producer evidence", async () => {
  for (const change of [
    (state) => {
      state.runs = [];
    },
    (state) => {
      state.runs[0].head_sha = "f".repeat(40);
    },
    (state) => {
      state.runs[0].head_branch = "main";
    },
    (state) => {
      state.runs[0].event = "pull_request";
    },
    (state) => {
      state.runs[0].conclusion = "failure";
    },
    (state) => {
      state.runs[0].status = "in_progress";
    },
    (state) => {
      state.runs[0].repository.full_name = "fork/dashboard";
    },
    (state) => {
      state.artifacts = [];
    },
    (state) => {
      state.artifacts[0].expired = true;
    },
    (state) => {
      state.artifacts.push({ ...state.artifacts[0], id: 9 });
    },
    (state) => {
      delete state.files[Object.keys(state.files)[0]];
    },
    (state) => {
      state.files["unexpected.txt"] = "extra";
    },
    (state) => {
      state.files[Object.keys(state.files)[1]] = "wrong checksum";
    },
    ...[
      (receipt) => {
        receipt.stage = "build";
      },
      (receipt) => {
        receipt.bundle.source.revision = "f".repeat(40);
      },
      (receipt) => {
        receipt.bundle.source.dirty = true;
      }
    ].map((changeReceipt) => (state) => {
      const name = Object.keys(state.files)[2];
      const receipt = JSON.parse(state.files[name]);
      changeReceipt(receipt);
      state.files[name] = JSON.stringify(receipt);
    })
  ]) {
    const state = dashboardFixture();
    change(state);
    await assert.rejects(publishRepositoryRelease(state.one));
    assert.equal(state.store.releases[1].draft, true);
    assert.equal(
      state.calls.some((call) =>
        ["release", "updateRelease"].includes(call.method)
      ),
      false
    );
  }
});

test("a published bundle-capable Dashboard release cannot silently omit assets", async () => {
  const state = dashboardFixture();
  await publishRepositoryRelease(state.one);
  state.assets.pop();
  await assert.rejects(
    publishRepositoryRelease(state.one),
    /missing plugin bundle assets/
  );
  assert.equal(
    state.calls.filter((call) => call.method === "release").length,
    1
  );
});

for (const [type, version, latest] of [
  ["rc", "v0.62.0-rc.2", "false"],
  ["final", "v0.62.0", "true"],
  ["patch", "v0.62.1", "true"]
]) {
  test(`${type} publishes native notes for exactly the approved siblings and DE`, async () => {
    const state = fixture(type, version);
    await publishSiblingReleases(state.options);
    assert.equal(JSON.parse(state.outputs.releases).length, 4);
    const creates = state.calls.filter(
      (call) => call.method === "createRelease"
    );
    const updates = state.calls.filter(
      (call) => call.method === "updateRelease"
    );
    assert.equal(creates.length, 4);
    assert.equal(updates.length, 4);
    for (const request of creates) {
      assert.equal(request.draft, true);
      assert.equal(request.make_latest, "false");
      assert.equal(request.prerelease, type === "rc");
      assert.match(request.target_commitish, /^[a-f0-9]{40}$/);
      assert.match(request.body, /^## What's Changed/);
      assert.equal(
        request.body,
        state.stores.get(`${request.owner}/${request.repo}`).releases[1].body
      );
    }
    assert.ok(
      updates.every(
        (request) => request.make_latest === latest && !request.draft
      )
    );
    await publishSiblingReleases(state.options);
    assert.equal(
      state.calls.filter((call) => call.method === "createRelease").length,
      4
    );
    assert.equal(
      state.calls.filter((call) => call.method === "generateReleaseNotes")
        .length,
      4
    );
    assert.equal(
      state.calls.filter((call) => call.method === "updateRelease").length,
      4
    );
  });
}

test("notes use the preceding RC or patch, but the preceding stable for a final", () => {
  const tags = ["v0.60.9", "v0.61.1", "v0.62.0-rc.1", "v0.62.0"].map(
    (name) => ({ name })
  );
  assert.equal(
    notesBase({ releaseType: "final", version: "v0.62.0" }, tags),
    "v0.61.1"
  );
  assert.equal(
    notesBase(
      {
        releaseType: "rc",
        version: "v0.62.0-rc.2",
        previousVersion: "v0.62.0-rc.1"
      },
      tags
    ),
    "v0.62.0-rc.1"
  );
  assert.equal(
    notesBase(
      { releaseType: "patch", version: "v0.62.1", previousVersion: "v0.62.0" },
      tags
    ),
    "v0.62.0"
  );
  assert.equal(
    notesBase(
      {
        releaseType: "rc",
        version: "v0.62.0-rc.1",
        previousVersion: "v0.61.1"
      },
      tags
    ),
    "v0.61.1"
  );
  assert.equal(
    notesBase({ releaseType: "final", version: "v0.62.0" }, []),
    undefined
  );
  assert.throws(
    () =>
      notesBase(
        {
          releaseType: "patch",
          version: "v0.62.1",
          previousVersion: "v0.62.0"
        },
        []
      ),
    /Previous release tag/
  );
});

test("verification, source and published Radius gates precede every remote write", async () => {
  for (const change of [
    (state) => {
      state.options.manifest.checks.installation = "pending";
    },
    (state) => {
      state.options.manifest.sourceSha = "b".repeat(40);
    },
    (state) => {
      state.options.plan.channel = "0.63";
    },
    (state) => {
      state.options.context.ref = "refs/heads/main";
    },
    (state) => {
      state.stores.get("radius-project/radius").sha = "b".repeat(40);
    },
    (state) => {
      state.options.github.rest.repos.getReleaseByTag = async () => ({
        data: { draft: true }
      });
    }
  ]) {
    const state = fixture();
    change(state);
    await assert.rejects(publishSiblingReleases(state.options));
    assert.equal(
      state.calls.filter((call) =>
        ["createRelease", "updateRelease"].includes(call.method)
      ).length,
      0
    );
  }
});

test("rejects unapproved destinations, duplicate sources and missing DE locks", () => {
  for (const change of [
    (plan) => {
      plan.siblingRepositories[0].repository = "another/repo";
    },
    (plan) => {
      plan.siblingRepositories[0].sourceCommit = "main";
    },
    (plan) => {
      plan.siblingRepositories[1] = plan.siblingRepositories[0];
    }
  ]) {
    const { options } = fixture();
    change(options.plan);
    options.manifest.planSha256 = createHash("sha256")
      .update(JSON.stringify(options.plan))
      .digest("hex");
    assert.throws(
      () => releaseTargets(options.plan, options.manifest, options.context.sha),
      /approved release source/
    );
  }
  const { options } = fixture();
  options.manifest.observed.imageLocks = [];
  assert.throws(
    () => releaseTargets(options.plan, options.manifest, options.context.sha),
    /Deployment Engine source lock/
  );
});

test("publishes the first GitHub Release when Latest returns 404", async () => {
  const state = fixture();
  const store = state.stores.get("radius-project/recipes");
  store.releases = [];
  store.latest = undefined;
  const latest = state.one.github.rest.repos.getLatestRelease;
  state.one.github.rest.repos.getLatestRelease = async (request) => {
    if (!store.latest) {
      throw Object.assign(new Error("Not Found"), { status: 404 });
    }
    return latest(request);
  };
  await publishRepositoryRelease(state.one);
  assert.equal(store.releases.length, 1);
  assert.equal(store.latest.tag_name, state.one.plan.version);
});

test("older patches never replace a newer stable Latest, even with a stale listing", async () => {
  const state = fixture("patch", "v0.62.1");
  const store = state.stores.get("radius-project/recipes");
  store.latest = { tag_name: "v0.63.0", draft: false, prerelease: false };
  await publishRepositoryRelease(state.one);
  assert.equal(store.latest.tag_name, "v0.63.0");
  assert.equal(
    state.calls.find((call) => call.method === "updateRelease").make_latest,
    "false"
  );
});

test("rejects missing or moved tags without creating a release", async () => {
  for (const missing of [false, true]) {
    const state = fixture();
    if (missing) {
      state.one.github.rest.git.getRef = async () => {
        throw Object.assign(new Error("Not Found"), { status: 404 });
      };
    } else {
      state.stores.get("radius-project/recipes").sha = "f".repeat(40);
    }
    await assert.rejects(publishRepositoryRelease(state.one));
    assert.equal(
      state.calls.filter((call) => call.method === "createRelease").length,
      0
    );
  }
});

test("rechecks the existing tag before publishing its draft", async () => {
  const state = fixture();
  const create = state.one.github.rest.repos.createRelease;
  state.one.github.rest.repos.createRelease = async (request) => {
    const response = await create(request);
    state.stores.get("radius-project/recipes").sha = "f".repeat(40);
    return response;
  };
  await assert.rejects(publishRepositoryRelease(state.one), /approved source/);
  assert.equal(
    state.calls.filter((call) => call.method === "updateRelease").length,
    0
  );
});

test("lost create responses adopt the same source-bound release without a second write", async () => {
  const state = fixture();
  const create = state.one.github.rest.repos.createRelease;
  state.one.github.rest.repos.createRelease = async (request) => {
    await create(request);
    throw Object.assign(new Error("response lost"), { status: 502 });
  };
  await publishRepositoryRelease(state.one);
  assert.equal(
    state.calls.filter((call) => call.method === "createRelease").length,
    1
  );
});

test("successful create responses survive delayed release-list visibility", async () => {
  const state = fixture();
  state.one.github.rest.repos.listReleases = async () => [];
  await publishRepositoryRelease(state.one);
  assert.equal(
    state.calls.filter((call) => call.method === "createRelease").length,
    1
  );
  assert.equal(
    state.calls.find((call) => call.method === "getRelease").release_id,
    2
  );
});

test("lost publication responses reconcile by release ID", async () => {
  const state = fixture();
  const update = state.one.github.rest.repos.updateRelease;
  state.one.github.rest.repos.updateRelease = async (request) => {
    await update(request);
    throw Object.assign(new Error("response lost"), { status: 503 });
  };
  await publishRepositoryRelease(state.one);
  assert.equal(
    state.calls.filter((call) => call.method === "updateRelease").length,
    1
  );
});

test("rejected writes and failed reads are not disguised as visibility timeouts", async () => {
  for (const method of [
    "createRelease",
    "updateRelease",
    "getLatestRelease",
    "getRelease"
  ]) {
    const state = fixture();
    state.one.github.rest.repos[method] = async () => {
      throw Object.assign(new Error(`${method} denied`), { status: 403 });
    };
    await assert.rejects(
      publishRepositoryRelease(state.one),
      new RegExp(`${method} denied`)
    );
    assert.deepEqual(state.sleeps, []);
  }
});

test("visibility polling is bounded and preserves the existing draft", async () => {
  const state = fixture();
  state.one.github.rest.repos.updateRelease = async () => {
    throw Object.assign(new Error("unknown write"), { status: 502 });
  };
  await assert.rejects(
    publishRepositoryRelease(state.one),
    /not visible after 5 attempts/
  );
  assert.deepEqual(state.sleeps, [1000, 2000, 4000, 8000]);
  assert.equal(
    state.stores.get("radius-project/recipes").releases[1].draft,
    true
  );
});

test("resume can finish a Latest update without replacing notes or assets", async () => {
  const state = fixture();
  await publishRepositoryRelease(state.one);
  const store = state.stores.get("radius-project/recipes");
  const body = store.releases[1].body;
  store.latest = store.releases[0];
  await publishRepositoryRelease(state.one);
  assert.equal(store.latest.tag_name, state.one.plan.version);
  assert.equal(store.releases[1].body, body);
  assert.equal(
    state.calls.filter((call) => call.method === "generateReleaseNotes").length,
    1
  );
  assert.equal(
    state.calls.filter((call) => call.method === "createRelease").length,
    1
  );
  assert.ok(
    state.calls
      .filter((call) => call.method === "updateRelease")
      .every((call) => call.body === undefined)
  );
});

test("conflicting existing releases and duplicates fail without replacement", async () => {
  for (const change of [
    (store) => {
      store.sha = "f".repeat(40);
    },
    (store) => {
      store.releases[1].prerelease = true;
    },
    (store) => {
      store.releases.push({ ...store.releases[1], id: 3 });
    }
  ]) {
    const state = fixture();
    await publishRepositoryRelease(state.one);
    change(state.stores.get("radius-project/recipes"));
    await assert.rejects(
      publishRepositoryRelease(state.one),
      /approved source|conflicts|Multiple releases/
    );
    assert.equal(
      state.calls.filter((call) => call.method === "createRelease").length,
      1
    );
    assert.equal(
      state.calls.filter((call) => call.method === "updateRelease").length,
      1
    );
  }
});

test("existing release targets may name a branch; the resolved tag binds the source", async () => {
  const state = fixture();
  await publishRepositoryRelease(state.one);
  const release = state.stores.get("radius-project/recipes").releases[1];
  release.target_commitish = "main";
  release.body = null;
  await publishRepositoryRelease(state.one);
  assert.equal(
    state.calls.filter((call) => call.method === "createRelease").length,
    1
  );
  assert.equal(
    state.calls.filter((call) => call.method === "updateRelease").length,
    1
  );
});

test("workflow uses independently authenticated sibling and DE clients", async (t) => {
  const workflow = (
    await readFile(
      new URL("../workflows/build-release.yaml", import.meta.url),
      "utf8"
    )
  ).replace(/\r\n/g, "\n");
  assert.match(
    workflow,
    /repositories: recipes,dashboard,bicep-types-aws\n\s+permission-actions: read/
  );
  const step = workflow.match(
    /      - name: Reconcile sibling release entries\n[\s\S]*?          script: \|\n([\s\S]*?)(?=\n      - name:)/
  );
  assert.ok(step);
  assert.match(step[0], /retries: 5/);
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const run = new AsyncFunction(
    "github",
    "getOctokit",
    "context",
    "core",
    "process",
    step[1].replace(/^            /gm, "")
  );
  const state = fixture();
  const directory = await mkdtemp(path.join(os.tmpdir(), "sibling-auth-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const files = {
    PLAN_FILE: path.join(directory, "plan.json"),
    MANIFEST_FILE: path.join(directory, "manifest.json")
  };
  await writeFile(files.PLAN_FILE, JSON.stringify(state.options.plan));
  await writeFile(files.MANIFEST_FILE, JSON.stringify(state.options.manifest));
  const tokens = [];
  const getOctokit = (token) => {
    tokens.push(token);
    return token === "siblings-test-token" ?
        state.options.siblings
      : state.options.engine;
  };
  await run(
    state.options.github,
    getOctokit,
    state.options.context,
    {
      ...state.options.core,
      getInput: (name) => files[name] ?? ""
    },
    {
      env: {
        SIBLING_RELEASE_TOKEN: "siblings-test-token",
        ENGINE_RELEASE_TOKEN: "engine-test-token",
        GITHUB_WORKSPACE: new URL("../../", import.meta.url).href.replace(
          /\/$/,
          ""
        )
      }
    }
  );
  assert.deepEqual(tokens, ["siblings-test-token", "engine-test-token"]);
  assert.equal(JSON.parse(state.outputs.releases).length, 4);
});
