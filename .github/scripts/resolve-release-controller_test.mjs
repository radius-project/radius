import assert from "node:assert/strict";
import test from "node:test";

import resolveReleaseController, {
  verifyReleaseBuildSource
} from "./resolve-release-controller.mjs";
import { mandatoryChecks } from "./publish-draft-release.mjs";
import stage from "./stage-release-bicep.mjs";
import { execFileSync } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const repository = "radius-project/radius";
const sourceCommit = "a".repeat(40);
const backportCommit = "b".repeat(40);

test("published historical sources retain validation and recovery; unpublished sources require GHCR-only writers", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "approved-build-contract-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  for (const state of [
    "published",
    "draft",
    "absent",
    "API failure",
    "wrong tag",
    "wrong AWS tag",
    "missing manifest",
    "wrong manifest",
    "unsupported workflow"
  ]) {
    const calls = [],
      outputs = {};
    const version = "v0.61.0";
    const plan = {
      version,
      channel: "0.61",
      releaseType: "patch",
      changelogPolicy: "stable-only",
      expectedOutputs: {
        ociArtifacts: [{ repository: "biceptypes.azurecr.io/radius" }]
      },
      siblingRepositories: [
        {
          name: "bicep-types-aws",
          repository: "radius-project/bicep-types-aws",
          sourceCommit: backportCommit
        }
      ]
    };
    const manifest = {
      schemaVersion: 1,
      tag: version,
      sourceSha: state === "wrong manifest" ? backportCommit : sourceCommit,
      channel: plan.channel,
      releaseType: plan.releaseType,
      checks: Object.fromEntries(
        mandatoryChecks.map((name) => [name, "verified"])
      )
    };
    const github = {
      paginate: async (method) => {
        calls.push(method);
        if (state === "API failure") throw new Error("403 denied");
        if (method === "releases")
          return state === "absent" ?
              []
            : [{ id: 1, tag_name: version, draft: state === "draft" }];
        assert.equal(method, "assets");
        return state === "missing manifest" ?
            []
          : [{ id: 2, name: "release-manifest.json", state: "uploaded" }];
      },
      request: async (method) => {
        assert.match(method, /^GET /);
        return { data: Buffer.from(JSON.stringify(manifest)) };
      },
      rest: {
        repos: {
          listReleases: "releases",
          listReleaseAssets: "assets",
          getCommit: async ({ repo, ref }) => {
            assert.equal(ref, `refs/tags/${version}`);
            calls.push(repo);
            return {
              data: {
                sha:
                  repo === "radius" ?
                    state === "wrong tag" ?
                      backportCommit
                    : sourceCommit
                  : state === "wrong AWS tag" ? sourceCommit
                  : backportCommit
              }
            };
          }
        }
      }
    };
    const inputs = {
      VERSION: version,
      SOURCE_SHA: sourceCommit,
      PLAN_FILE: join(directory, "approved-plan.yaml")
    };
    const core = {
      getInput: (name) => inputs[name],
      setOutput: (name, value) => {
        outputs[name] = value;
      }
    };
    const run = async (args) => {
      if (args[0] === "yq") return JSON.stringify(plan);
      assert.deepEqual(args.slice(0, 3), [
        "bash",
        "./.github/scripts/verify-release-build-contract.sh",
        sourceCommit
      ]);
      assert.ok(args.includes("--require-sibling-releases"));
      assert.ok(args.includes("--expected-targets"));
      assert.deepEqual(
        JSON.parse(
          await readFile(args[args.indexOf("--expected-targets") + 1], "utf8")
        ),
        plan.expectedOutputs
      );
      calls.push("build-contract");
      if (["draft", "absent"].includes(state)) {
        assert.ok(args.includes("--require-current-bicep"));
        throw new Error("requires the retired Bicep ACR writer");
      }
      assert.ok(!args.includes("--require-current-bicep"));
      if (state === "unsupported workflow")
        throw new Error("does not enforce the publication gate");
      return "";
    };
    const promise = verifyReleaseBuildSource({ github, core, run });
    if (state === "published") {
      await promise;
      assert.equal(outputs.published, "true");
      assert.deepEqual(calls, [
        "releases",
        "radius",
        "assets",
        "bicep-types-aws",
        "build-contract"
      ]);
    } else {
      await assert.rejects(promise);
      assert.deepEqual(outputs, {});
      if (!["draft", "absent", "unsupported workflow"].includes(state))
        assert.ok(!calls.includes("build-contract"));
    }
  }
});

test("legacy published preflight is read-only while absent/draft legacy mutation fails", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "legacy-bicep-preflight-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const plan = {
    expectedOutputs: {
      ociArtifacts: [
        {
          name: "aws-bicep-types",
          repository: "biceptypes.azurecr.io/aws",
          artifactType: "application/vnd.ms.bicep.provider.artifact"
        },
        {
          name: "radius-bicep-types",
          repository: "biceptypes.azurecr.io/radius",
          artifactType: "application/vnd.ms.bicep.provider.artifact"
        }
      ]
    }
  };
  for (const state of ["published", "draft", "absent", ""]) {
    const inputs = { directory, operation: "preflight", release_state: state };
    const promise = stage({
      github: new Proxy({}, { get: () => assert.fail("No Bicep API calls") }),
      context: { ref: "refs/tags/v0.61.0" },
      core: { getInput: (name) => inputs[name], setOutput() {} },
      run: async (args) => {
        assert.equal(
          args[0],
          "yq",
          "No tag, dispatch, marker or registry mutations"
        );
        return JSON.stringify(plan);
      }
    });
    if (state === "published") await promise;
    else await assert.rejects(promise, /retired Bicep ACR writer/);
  }
});

test("published workflow gates skip Bicep producers/finalizers without disabling downstream recovery", () => {
  const read = (file) =>
    JSON.parse(
      execFileSync("yq", ["-o=json", ".", file], { encoding: "utf8" })
    );
  const jobs = read(".github/workflows/build-release.yaml").jobs;
  for (const name of [
    "publish-radius-bicep-extension",
    "build-and-push-bicep-types",
    "finalize-release"
  ]) {
    const condition = jobs[name].if
      .replace(/needs\.[\w-]+(?:\.outputs)?\.[\w-]+/g, (value) =>
        value.endsWith(".release-state") ? "'published'" : "'success'"
      )
      .replace("!cancelled()", "true")
      .replace("github.repository", "'radius-project/radius'")
      .replace("startsWith(github.ref, 'refs/tags/v')", "true")
      .replace("contains(github.ref_name, '-rc.')", "false");
    assert.equal(Function(`return (${condition})`)(), false, name);
  }
  for (const name of [
    "verify-release",
    "coordinate-release",
    "publish-sibling-releases"
  ])
    assert.match(jobs[name].if, /release-state == 'published'/);
  const controller = read(".github/workflows/__release-controller.yaml").jobs;
  assert.equal(controller.approve.if, "needs.validate.outputs.ready == 'true'");
  const aws = controller["reconcile-siblings"].steps.find(
    (step) => step.name === "Reconcile radius-project/bicep-types-aws"
  );
  assert.equal(aws.if, "needs.validate.outputs.published != 'true'");
  assert.ok(
    controller["create-radius-tag"].steps.some((step) =>
      step.with?.script?.includes("resumeReleasePublication")
    )
  );
});

test("locked pair does not hide a failed or cancelled selected producer", () => {
  const workflow = JSON.parse(
    execFileSync(
      "yq",
      ["-o=json", ".", ".github/workflows/build-release.yaml"],
      { encoding: "utf8" }
    )
  );
  const expression = workflow.jobs["build-and-push-bicep-types"].if;
  for (const locked of ["true", "false"]) {
    for (const result of ["success", "failure", "cancelled", "skipped"]) {
      const inputs = {
        "needs.release-preflight.result": "success",
        "needs.release-preflight.outputs.release-state": "draft",
        "needs.release-preflight.outputs.ghcr-bicep": "true",
        "needs.release-preflight.outputs.bicep-locked": locked,
        "needs.publish-radius-bicep-extension.result": result
      };
      const condition = expression
        .replace("!cancelled()", "true")
        .replace(/needs\.[\w-]+(?:\.outputs)?\.[\w-]+/g, (key) =>
          JSON.stringify(inputs[key])
        );
      assert.equal(
        Function(`return (${condition})`)(),
        result === "success" || (result === "skipped" && locked === "true"),
        `${result}, locked=${locked}`
      );
    }
  }
});

function releasePull(overrides = {}) {
  return {
    number: 42,
    body: "approved release plan",
    base: { ref: "main" },
    head: {
      ref: "automation/prepare-release-0.61.0-rc.1",
      repo: { full_name: repository }
    },
    merged: true,
    merged_at: "2026-08-28T12:00:00Z",
    merge_commit_sha: sourceCommit,
    ...overrides
  };
}

function fixture({ mode = "event", pull, pulls = [] } = {}) {
  const inputs = {
    MODE: mode,
    VERSION: "v0.61.0-rc.1",
    SOURCE_SHA: sourceCommit
  };
  const outputs = {};
  const context = {
    repo: { owner: "radius-project", repo: "radius" },
    payload: { pull_request: pull }
  };
  const github = {
    paginate: async (method, request) => method(request),
    rest: {
      pulls: {
        list: async () => pulls,
        get: async ({ pull_number }) => ({
          data: pulls.find((candidate) => candidate.number === pull_number)
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
  return { context, core, github, inputs, outputs };
}

test("selects a merged generated release pull request", async () => {
  const state = fixture({ pull: releasePull() });
  await resolveReleaseController(state);
  assert.deepEqual(state.outputs, {
    selected: "true",
    "source-pr-number": "42",
    "source-pr-commit": sourceCommit,
    version: "v0.61.0-rc.1",
    "release-commit": sourceCommit,
    trigger: "release-pr"
  });
});

test("selects the generated release metadata backport", async () => {
  const source = releasePull();
  const state = fixture({
    pull: {
      number: 43,
      base: { ref: "release/0.61" },
      head: {
        ref: "automation/backport-42-to-0.61",
        repo: { full_name: repository }
      },
      merged: true,
      merge_commit_sha: backportCommit
    },
    pulls: [source]
  });
  await resolveReleaseController(state);
  assert.equal(state.outputs.selected, "true");
  assert.equal(state.outputs["source-pr-number"], "42");
  assert.equal(state.outputs["source-pr-commit"], sourceCommit);
  assert.equal(state.outputs["release-commit"], backportCommit);
  assert.equal(state.outputs.trigger, "release-backport");
});

test("ignores a backport whose source is not a release pull", async () => {
  const ordinary = releasePull({
    number: 41,
    head: { ref: "fix/ordinary", repo: { full_name: repository } }
  });
  const state = fixture({
    pull: {
      base: { ref: "release/0.61" },
      head: {
        ref: "automation/backport-41-to-0.61",
        repo: { full_name: repository }
      },
      merged: true,
      merge_commit_sha: backportCommit
    },
    pulls: [ordinary]
  });
  await resolveReleaseController(state);
  assert.deepEqual(state.outputs, { selected: "false" });
});

test("resolves explicit version and source inputs to one plan", async () => {
  const state = fixture({ mode: "dispatch", pulls: [releasePull()] });
  const requests = [];
  state.github.rest.pulls.list = async (request) => {
    requests.push(request);
    return [releasePull()];
  };
  await resolveReleaseController(state);
  assert.deepEqual(requests, [
    {
      owner: "radius-project",
      repo: "radius",
      base: "main",
      head: "radius-project:automation/prepare-release-0.61.0-rc.1",
      state: "closed",
      per_page: 100
    }
  ]);
  assert.equal(state.outputs.selected, "true");
  assert.equal(state.outputs.trigger, "dispatch");
});

test("still validates repository, merge state and branch after a filtered lookup", async () => {
  const state = fixture({
    mode: "dispatch",
    pulls: [
      releasePull(),
      releasePull({ number: 43, merged_at: null }),
      releasePull({ number: 44, base: { ref: "release/0.61" } }),
      releasePull({
        number: 45,
        head: {
          ref: "automation/prepare-release-0.61.0-rc.1",
          repo: { full_name: "other/radius" }
        }
      }),
      releasePull({
        number: 46,
        head: {
          ref: "automation/prepare-release-0.61.0-rc.2",
          repo: { full_name: repository }
        }
      })
    ]
  });
  await resolveReleaseController(state);
  assert.equal(state.outputs["source-pr-number"], "42");
});

test("rejects ambiguous generated release pull requests", async () => {
  const state = fixture({
    mode: "dispatch",
    pulls: [releasePull(), releasePull({ number: 44 })]
  });
  await assert.rejects(
    () => resolveReleaseController(state),
    /Expected one merged generated release pull request/
  );
});

test("rejects a malformed explicit source commit", async () => {
  const state = fixture({ mode: "dispatch", pulls: [releasePull()] });
  state.inputs.SOURCE_SHA = "main";
  await assert.rejects(
    () => resolveReleaseController(state),
    /SOURCE_SHA must be a full commit SHA/
  );
});
