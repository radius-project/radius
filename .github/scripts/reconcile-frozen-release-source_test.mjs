import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import test from "node:test";
import reconcileSource, {
  updateFrozenPreparation
} from "./reconcile-frozen-release-source.mjs";
import reconcileLock from "./reconcile-release-controller-lock.mjs";
import { resumeReleasePublication } from "./dispatch-release-controller.mjs";

const product = "a".repeat(40);
const metadata = "b".repeat(40);
const tree = "c".repeat(40);
const release = "d".repeat(40);
const version = "v0.61.0-rc.1";
const branch = "automation/release-source-0.61.0-rc.1";
const metadataFiles = [
  "versions.yaml",
  "CHANGELOG.md",
  `docs/release-notes/${version}.md`,
  `.github/release-plans/${version}.yaml`
];
const scripts = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(scripts, "../..");
const context = { repo: { owner: "radius-project", repo: "radius" } };
const notFound = () => Object.assign(new Error("Not found"), { status: 404 });
const users = {
  getByUsername: async () => ({
    data: { id: 123, login: "radius-release[bot]", type: "Bot" }
  })
};

function coreFixture(inputs) {
  const outputs = {};
  return {
    inputs,
    outputs,
    getInput: (name) => inputs[name] ?? "",
    setOutput: (name, value) => {
      outputs[name] = value;
    },
    notice: () => {}
  };
}

function fixture() {
  const core = coreFixture({
    MODE: "create",
    APP_SLUG: "radius-release",
    VERSION: version,
    PRODUCT_COMMIT: product,
    SOURCE_PR_COMMIT: metadata,
    RELEASE_TREE: tree,
    RELEASE_COMMIT: metadata
  });
  const state = {
    head: undefined,
    creates: 0,
    mutations: [],
    metadataVerified: true,
    conflict: false,
    lostResponse: false,
    commit: {
      parents: [{ sha: product }],
      author: { type: "Bot", login: "radius-release[bot]" },
      commit: {
        tree: { sha: tree },
        message: `chore(release): freeze ${version}\n\n(cherry picked from commit ${metadata})`,
        verification: { verified: true }
      }
    }
  };
  const github = {
    rest: {
      users,
      git: {
        getRef: async ({ ref }) => {
          if (ref === `heads/${branch}` && state.head) {
            return { data: { object: { sha: state.head, type: "commit" } } };
          }
          if (state.conflict === ref) {
            return { data: { object: { sha: metadata, type: "commit" } } };
          }
          throw notFound();
        },
        createRef: async ({ ref, sha }) => {
          assert.equal(ref, `refs/heads/${branch}`);
          assert.equal(sha, product);
          state.creates++;
          state.head = sha;
        }
      },
      repos: {
        getCommit: async ({ ref }) => ({
          data:
            ref === metadata ?
              { commit: { verification: { verified: state.metadataVerified } } }
            : structuredClone(state.commit)
        }),
        getContent: async ({ ref, path: file }) => {
          assert.equal(ref, metadata);
          assert.ok(metadataFiles.includes(file));
          return {
            data: {
              type: "file",
              encoding: "base64",
              content: Buffer.from(`approved ${file}\n`).toString("base64")
            }
          };
        }
      }
    },
    graphql: async (query, { input }) => {
      assert.match(query, /createCommitOnBranch/);
      state.mutations.push(input);
      state.head = release;
      if (state.lostResponse) {
        throw Object.assign(new Error("Response lost"), { status: 504 });
      }
      return { createCommitOnBranch: { commit: { oid: release } } };
    }
  };
  return {
    github,
    core,
    state,
    run: () => reconcileSource({ github, core, context })
  };
}

test("read-only preflight never creates a source before approval", async () => {
  const f = fixture();
  f.core.inputs.MODE = "read";
  await f.run();
  assert.equal(f.core.outputs.resolved, "false");
  assert.equal(f.state.creates, 0);
  assert.equal(f.state.mutations.length, 0);
});

test("rejects the wrong App before any source mutation", async () => {
  const f = fixture();
  f.core.inputs.APP_SLUG = "radius-publisher";
  await assert.rejects(f.run(), /Only the radius-release App/);
  assert.equal(f.state.creates, 0);
  assert.equal(f.state.mutations.length, 0);
});

test("creates one four-file App commit with a frozen-parent CAS; resume is a no-op", async () => {
  const f = fixture();
  await f.run();
  assert.equal(f.core.outputs["release-commit"], release);
  assert.equal(f.core.outputs["release-identifier"], `${version}-${release}`);
  const [input] = f.state.mutations;
  assert.equal(input.expectedHeadOid, product);
  assert.deepEqual(
    input.fileChanges.additions.map((file) => file.path),
    metadataFiles
  );
  assert.equal(input.branch.branchName, branch);
  assert.match(
    input.message.body,
    /Signed-off-by: radius-release\[bot\] <123\+/
  );
  await f.run();
  f.core.inputs.RELEASE_COMMIT = release;
  await f.run();
  assert.equal(f.state.creates, 1);
  assert.equal(f.state.mutations.length, 1);
});

test("resumes an interrupted empty source ref and adopts a lost commit response", async () => {
  const f = fixture();
  f.state.head = product;
  f.state.lostResponse = true;
  await f.run();
  assert.equal(f.core.outputs["release-commit"], release);
  assert.equal(f.state.creates, 0);
  assert.equal(f.state.mutations.length, 1);
});

for (const [name, mutate, error] of [
  [
    "parent",
    (f) => {
      f.state.commit.parents = [{ sha: metadata }];
    },
    /history\/tree/
  ],
  [
    "tree",
    (f) => {
      f.state.commit.commit.tree.sha = metadata;
    },
    /history\/tree/
  ],
  [
    "metadata identity",
    (f) => {
      f.state.commit.commit.message = "not approved";
    },
    /history\/tree/
  ],
  [
    "signature",
    (f) => {
      f.state.commit.commit.verification.verified = false;
    },
    /release App commit/
  ],
  [
    "App author",
    (f) => {
      f.state.commit.author.login = "another-app[bot]";
    },
    /release App commit/
  ],
  [
    "human author",
    (f) => {
      f.state.commit.author.type = "User";
    },
    /release App commit/
  ],
  [
    "requested source",
    (f) => {
      f.core.inputs.RELEASE_COMMIT = tree;
    },
    /differs from/
  ],
  [
    "metadata signature",
    (f) => {
      f.state.metadataVerified = false;
    },
    /metadata commit is not/
  ]
]) {
  test(`rejects frozen source ${name} tampering without mutation`, async () => {
    const f = fixture();
    f.state.head = release;
    mutate(f);
    await assert.rejects(f.run(), error);
    assert.equal(f.state.creates, 0);
    assert.equal(f.state.mutations.length, 0);
  });
}

for (const conflictingRef of [
  "heads/release/0.61",
  `tags/${version}`,
  "heads/automation/release-state-0.61.0-rc.1"
]) {
  test(`rejects missing frozen source alongside ${conflictingRef}`, async () => {
    const f = fixture();
    f.state.conflict = conflictingRef;
    for (const mode of ["read", "create"]) {
      f.core.inputs.MODE = mode;
      await assert.rejects(f.run(), /exists without a resolved frozen source/);
    }
    assert.equal(f.state.creates, 0);
    assert.equal(f.state.mutations.length, 0);
  });
}

test("rejects an unresolved explicit release SHA and invalid source input", async () => {
  const f = fixture();
  f.core.inputs.RELEASE_COMMIT = release;
  await assert.rejects(f.run(), /no matching frozen source ref/);
  f.core.inputs.PRODUCT_COMMIT = "main";
  await assert.rejects(f.run(), /full SHAs/);
  assert.equal(f.state.creates, 0);
});

for (const scenario of [
  "update",
  "unchanged",
  "PM edit",
  "unsigned",
  "closed"
]) {
  test(`preparation CAS retains PM history: ${scenario}`, async (t) => {
    const f = fixture();
    const temp = mkdtempSync(path.join(os.tmpdir(), "frozen-preparation-"));
    t.after(() => rmSync(temp, { recursive: true, force: true }));
    for (const file of metadataFiles) {
      mkdirSync(path.dirname(path.join(temp, file)), { recursive: true });
      writeFileSync(
        path.join(temp, file),
        scenario === "unchanged" ? `approved ${file}\n` : `updated ${file}\n`
      );
    }
    writeFileSync(
      path.join(temp, "release-pr-body.md"),
      "approved frozen plan"
    );
    Object.assign(f.core.inputs, {
      HEAD_SHA: metadata,
      PR_NUMBER: "123",
      WORKSPACE: temp,
      OUTPUT_DIR: temp
    });
    let bodyWrites = 0;
    f.github.rest.pulls = {
      get: async () => ({
        data: {
          state: scenario === "closed" ? "closed" : "open",
          base: { ref: "main" },
          head: {
            ref: "automation/prepare-release-0.61.0-rc.1",
            sha: metadata,
            repo: { full_name: "radius-project/radius" }
          },
          html_url: "https://example.test/pull/123"
        }
      }),
      update: async ({ body }) => {
        assert.equal(body, "approved frozen plan");
        bodyWrites++;
      }
    };
    f.state.commit.parents = [{ sha: metadata }];
    f.state.commit.commit.verification.verified = scenario !== "unsigned";
    const commit = f.github.graphql;
    f.github.graphql = async (query, variables) => {
      assert.equal(variables.input.expectedHeadOid, metadata);
      if (scenario === "PM edit") {
        throw Object.assign(new Error("Head changed after notes snapshot"), {
          status: 409
        });
      }
      return commit(query, variables);
    };
    const update = () =>
      updateFrozenPreparation({ github: f.github, core: f.core, context });
    if (["PM edit", "unsigned", "closed"].includes(scenario)) {
      await assert.rejects(
        update(),
        /Head changed|release App commit|PR changed/
      );
      assert.equal(bodyWrites, 0);
    } else {
      await update();
      assert.equal(
        f.core.outputs["pull-request-url"],
        "https://example.test/pull/123"
      );
      assert.equal(bodyWrites, 1);
    }
    if (["unchanged", "PM edit", "closed"].includes(scenario)) {
      assert.equal(f.state.mutations.length, 0);
    }
  });
}

test("offline lifecycle: A + curated metadata, never later main/queue B; immutable resume", async (t) => {
  const channel = process.env.FROZEN_RELEASE_CHANNEL ?? "0.61";
  const version = `v${channel}.0-rc.1`;
  const branch = `automation/release-source-${version.slice(1)}`;
  const releaseBranch = `release/${channel}`;
  const metadataFiles = [
    "versions.yaml",
    "CHANGELOG.md",
    `docs/release-notes/${version}.md`,
    `.github/release-plans/${version}.yaml`
  ];
  const temp = mkdtempSync(path.join(os.tmpdir(), "frozen-release-lifecycle-"));
  t.after(() => rmSync(temp, { recursive: true, force: true }));
  const repo = path.join(temp, "radius");
  const origin = path.join(temp, "radius.git");
  mkdirSync(repo);
  const env = { ...process.env, GITHUB_TOKEN: "", GH_TOKEN: "" };
  delete env.GITHUB_OUTPUT;
  const run = (command, args, options = {}) =>
    execFileSync(command, args, {
      cwd: repo,
      env,
      encoding: "utf8",
      stdio: ["pipe", "pipe", "pipe"],
      ...options
    }).trimEnd();
  const git = (...args) => run("git", args);
  const bash = (script, args = [], extraEnv = {}) =>
    run("bash", [path.join(scripts, script), ...args], {
      env: { ...env, ...extraEnv }
    });
  const write = (file, content) => {
    mkdirSync(path.dirname(path.join(repo, file)), { recursive: true });
    writeFileSync(path.join(repo, file), content);
  };
  const read = (file) => readFileSync(file, "utf8").trimEnd();
  const commit = (message) => {
    git("add", ".");
    git("commit", "-qm", message);
    return git("rev-parse", "HEAD");
  };
  git("init", "-q", "-b", "main");
  git("config", "user.name", "Release Fixture");
  git("config", "user.email", "release-fixture@example.test");
  git("config", "commit.gpgsign", "false");
  git("config", "tag.gpgsign", "false");
  git("init", "-q", "--bare", "--initial-branch=main", origin);
  git("remote", "add", "origin", origin);

  // The optional archive comes from an actual pre-fix cutoff. Only tagged
  // build/publication readers are copied; new controller code runs externally.
  const compatibilitySource = process.env.FROZEN_RELEASE_COMPAT_DIR ?? root;
  const frozenFiles = [
    ".goreleaser.yaml",
    ".github/workflows/build-release.yaml",
    ".github/scripts/verify-release-manifest.mjs",
    ".github/scripts/publish-draft-release.mjs",
    ".github/scripts/release-assets.mjs",
    ".github/scripts/verify-release-publication.sh",
    ".github/release-parity/targets.json",
    "cliff.toml",
    "docs/release-notes/template.md",
    "docs/release-notes/template_patch.md"
  ];
  const productPack = process.env.FROZEN_RELEASE_PRODUCT_PACK;
  let a;
  if (productPack) {
    a = process.env.FROZEN_RELEASE_PRODUCT_SHA;
    assert.match(a, /^[0-9a-f]{40}$/);
    run("git", ["unpack-objects", "-q"], { input: readFileSync(productPack) });
    // Full history is required by git-cliff, including in its regeneration worktree.
    git("rev-list", "--count", a);
    const parent = git("rev-parse", `${a}^`);
    git("reset", "-q", "--hard", a);
    for (const tag of run("yq", [
      "-r",
      ".supported[].version",
      "versions.yaml"
    ]).split("\n")) {
      git("tag", tag, parent);
    }
  } else {
    for (const file of frozenFiles) {
      mkdirSync(path.dirname(path.join(repo, file)), { recursive: true });
      cpSync(path.join(compatibilitySource, file), path.join(repo, file));
    }
    write(
      "versions.yaml",
      "supported:\n  - channel: '0.60'\n    version: 'v0.60.0'\ndeprecated: []\n"
    );
    write(
      "CHANGELOG.md",
      "# Changelog\n\n## [Unreleased]\n\n[Unreleased]: https://github.com/radius-project/radius/compare/v0.60.0...HEAD\n"
    );
    write("product.txt", "stable\n");
    commit("chore: previous stable release");
    git("tag", "v0.60.0");
    write("product.txt", "product at cutoff A\n");
    a = commit("feat: included at cutoff A");
  }
  const productFile = productPack ? "go.mod" : "product.txt";
  const productContents = git("show", `${a}:${productFile}`);
  git("push", "-q", "origin", "main");

  const siblingsRoot = path.join(temp, "siblings");
  mkdirSync(siblingsRoot);
  const siblings = ["recipes", "dashboard", "bicep-types-aws"].map((name) => {
    const remote = path.join(siblingsRoot, `${name}.git`);
    git("init", "-q", "--bare", "--initial-branch=main", remote);
    const emptyTree = run("git", ["mktree"], { input: "" });
    const siblingCommit = git(
      "commit-tree",
      emptyTree,
      "-m",
      `Initial ${name}`
    );
    git("push", "-q", remote, `${siblingCommit}:refs/heads/main`);
    return {
      name,
      repository: `radius-project/${name}`,
      sourceRef: "main",
      sourceCommit: siblingCommit
    };
  });
  const backports = path.join(temp, "backports.json");
  const siblingsFile = path.join(temp, "siblings.json");
  writeFileSync(backports, "[]\n");
  writeFileSync(siblingsFile, JSON.stringify(siblings));
  const prepared = path.join(temp, "prepared");
  const prepareArgs = [
    "--release-type",
    "rc",
    "--channel",
    channel,
    "--release-date",
    "2026-08-24",
    "--backports-file",
    backports,
    "--sibling-repositories-file",
    siblingsFile,
    "--output-dir",
    prepared
  ];
  bash("verify-release-build-contract.sh", [a]);
  bash("prepare-release.sh", prepareArgs);
  const originalPlan = path.join(temp, "original-plan.yaml");
  cpSync(path.join(prepared, "release-plan.yaml"), originalPlan);
  const body = path.join(temp, "body.md");
  cpSync(path.join(prepared, "release-pr-body.md"), body);
  git("switch", "-qc", "preparation");
  commit(`chore(release): prepare ${version}`);
  const notesFile = metadataFiles[2];
  write(
    notesFile,
    read(path.join(repo, notesFile))
      .replace("## Highlights", "## Highlights\n\nPM-approved highlight.")
      .replace(
        `## Upgrading to Radius ${version}`,
        `## Upgrading to Radius ${version}\n\nPM-approved upgrade guidance.`
      ) + "\n"
  );
  const prHead = commit("docs: curate release highlights and upgrading");
  const reviewed = path.join(temp, "reviewed");
  for (const file of metadataFiles) {
    mkdirSync(path.dirname(path.join(reviewed, file)), { recursive: true });
    cpSync(path.join(repo, file), path.join(reviewed, file));
  }
  git("switch", "-q", "main");
  write(productFile, `${productContents}\n// product at later main B\n`);
  write("after-cutoff.txt", "must not be released\n");
  const b = commit("feat: excluded after cutoff B");
  git("push", "-q", "origin", "main");
  const advancedSibling = git(
    "commit-tree",
    git("rev-parse", `${siblings[0].sourceCommit}^{tree}`),
    "-p",
    siblings[0].sourceCommit,
    "-m",
    "Later recipes change"
  );
  git(
    "push",
    "-q",
    path.join(siblingsRoot, "recipes.git"),
    `${advancedSibling}:refs/heads/main`
  );
  writeFileSync(
    siblingsFile,
    JSON.stringify(
      siblings.map((sibling) => ({ ...sibling, sourceCommit: advancedSibling }))
    )
  );
  bash("prepare-release.sh", [...prepareArgs, "--existing-plan", originalPlan]);
  bash("preserve-release-note-sections.sh", [
    notesFile,
    path.join(reviewed, notesFile)
  ]);
  for (const file of metadataFiles) {
    assert.equal(
      read(path.join(repo, file)),
      read(path.join(reviewed, file)),
      file
    );
  }
  assert.doesNotMatch(
    read(path.join(repo, notesFile)),
    /Excluded after cutoff B/
  );
  const m = commit(`chore(release): prepare ${version}`);
  git("push", "-q", "origin", "main");
  git("switch", "-q", "--detach", b);

  const filesFile = path.join(temp, "files.json");
  writeFileSync(filesFile, JSON.stringify(metadataFiles));
  const validatorArgs = [
    "--body-file",
    body,
    "--files-file",
    filesFile,
    "--base-sha",
    b,
    "--head-dir",
    reviewed,
    "--repository",
    "radius-project/radius"
  ];
  bash("validate-release-plan.sh", validatorArgs, {
    EXPECTED_BACKPORTS_FILE: backports,
    SIBLING_REPOSITORY_ROOT: siblingsRoot
  });
  git("switch", "-qc", "merge-queue", b);
  write("queued-product.txt", "queued predecessor\n");
  commit("feat: queued after cutoff");
  git(
    "restore",
    "--source",
    prHead,
    "--staged",
    "--worktree",
    "--",
    ...metadataFiles
  );
  const queued = commit("chore(release): queued metadata");
  const candidates = path.join(temp, "candidates.json");
  const selected = path.join(temp, "selected.txt");
  writeFileSync(
    candidates,
    JSON.stringify([{ number: 123, head_sha: prHead, files: metadataFiles }])
  );
  bash("validate-release-merge-group.sh", [
    "--candidates-file",
    candidates,
    "--merge-group-sha",
    queued,
    "--base-sha",
    b,
    "--output-file",
    selected
  ]);
  assert.equal(read(selected), "123");
  git("switch", "-q", "main");

  const controllerOutput = path.join(temp, "controller");
  const validateController = (sha) =>
    bash("validate-release-controller-plan.sh", [
      "--plan-file",
      originalPlan,
      "--version",
      version,
      "--source-pr-commit",
      m,
      "--release-commit",
      sha,
      "--trigger",
      "dispatch",
      "--output-dir",
      controllerOutput
    ]);
  validateController(m);
  assert.equal(
    read(path.join(controllerOutput, "source-resolved.txt")),
    "false"
  );
  const expectedTree = read(path.join(controllerOutput, "release-tree.txt"));
  const sourceCore = coreFixture({
    MODE: "read",
    APP_SLUG: "radius-release",
    VERSION: version,
    PRODUCT_COMMIT: a,
    SOURCE_PR_COMMIT: m,
    RELEASE_TREE: expectedTree,
    RELEASE_COMMIT: m
  });
  let sourceCreates = 0;
  let verified = true;
  const reference = (ref) => {
    try {
      return git("--git-dir", origin, "rev-parse", "--verify", `refs/${ref}`);
    } catch {
      throw notFound();
    }
  };
  const apiCommit = (parent, additions, message, target) => {
    const indexEnv = { ...env, GIT_INDEX_FILE: path.join(temp, "api-index") };
    const indexedGit = (...args) => run("git", args, { env: indexEnv });
    indexedGit("read-tree", parent);
    for (const file of additions) {
      const blob = run("git", ["hash-object", "-w", "--stdin"], {
        input: Buffer.from(file.contents, "base64")
      });
      indexedGit(
        "update-index",
        "--add",
        "--cacheinfo",
        `100644,${blob},${file.path}`
      );
    }
    const sha = git(
      "commit-tree",
      indexedGit("write-tree"),
      "-p",
      parent,
      "-m",
      message
    );
    git("push", "-q", "origin", `${sha}:refs/heads/${target}`);
    return sha;
  };
  const github = {
    rest: {
      users,
      git: {
        getRef: async ({ ref }) => ({
          data: { object: { sha: reference(ref), type: "commit" } }
        }),
        createRef: async ({ ref, sha }) => {
          git("push", "-q", "origin", `${sha}:${ref}`);
        }
      },
      repos: {
        getCommit: async ({ ref }) => ({
          data: {
            parents: git("rev-list", "--parents", "-n", "1", ref)
              .split(" ")
              .slice(1)
              .map((sha) => ({ sha })),
            author: { type: "Bot", login: "radius-release[bot]" },
            commit: {
              tree: { sha: git("rev-parse", `${ref}^{tree}`) },
              message: git("show", "-s", "--format=%B", ref),
              verification: { verified }
            }
          }
        }),
        getContent: async ({ ref, path: file }) => {
          let content;
          const sha =
            /^[a-f0-9]{40}$/.test(ref) ? ref : reference(`heads/${ref}`);
          try {
            content = execFileSync("git", ["show", `${sha}:${file}`], {
              cwd: repo,
              env,
              stdio: ["pipe", "pipe", "pipe"]
            });
          } catch {
            throw notFound();
          }
          return {
            data: {
              type: "file",
              encoding: "base64",
              content: content.toString("base64")
            }
          };
        },
        createOrUpdateFileContents: async ({
          branch: target,
          path: file,
          content,
          message
        }) => {
          apiCommit(
            reference(`heads/${target}`),
            [{ path: file, contents: content }],
            message,
            target
          );
        }
      }
    },
    graphql: async (_query, { input }) => {
      assert.equal(reference(`heads/${branch}`), input.expectedHeadOid);
      sourceCreates++;
      const sha = apiCommit(
        input.expectedHeadOid,
        input.fileChanges.additions,
        `${input.message.headline}\n\n${input.message.body}`,
        input.branch.branchName
      );
      return { createCommitOnBranch: { commit: { oid: sha } } };
    }
  };
  await reconcileSource({ github, context, core: sourceCore });
  assert.equal(sourceCreates, 0);
  sourceCore.inputs.MODE = "create";
  await reconcileSource({ github, context, core: sourceCore });
  const c = sourceCore.outputs["release-commit"];
  assert.notEqual(c, a);
  assert.notEqual(c, m);
  assert.equal(git("rev-parse", `${c}^`), a);
  assert.equal(git("rev-parse", `${c}^{tree}`), expectedTree);
  assert.deepEqual(
    git("diff", "--no-renames", "--name-only", a, c).split("\n").sort(),
    [...metadataFiles].sort()
  );
  assert.equal(git("show", `${c}:${productFile}`), productContents);
  assert.throws(() => git("cat-file", "-e", `${c}:after-cutoff.txt`));
  assert.throws(() => git("cat-file", "-e", `${c}:queued-product.txt`));
  for (const file of metadataFiles) {
    assert.equal(
      git("rev-parse", `${c}:${file}`),
      git("rev-parse", `${prHead}:${file}`)
    );
  }
  validateController(c);
  assert.equal(read(path.join(controllerOutput, "release-commit.txt")), c);
  bash("verify-release-build-contract.sh", [c]);

  const lockCore = coreFixture({
    MODE: "create",
    VERSION: version,
    RELEASE_SOURCE_SHA: c,
    DE_SIGNED_TAG: version,
    DE_SOURCE_SHA: "e".repeat(40),
    DE_IMAGE_TAG: version.slice(1),
    DE_DIGEST: `sha256:${"f".repeat(64)}`
  });
  await reconcileLock({ github, context, core: lockCore });
  const lockCommit = lockCore.outputs.commit;
  bash("release-create-tag-and-branch.sh", [repo, version, releaseBranch, c]);
  for (const requested of [m, c]) {
    validateController(requested);
    assert.equal(read(path.join(controllerOutput, "release-commit.txt")), c);
    sourceCore.inputs.RELEASE_COMMIT = requested;
    await reconcileSource({ github, context, core: sourceCore });
    assert.equal(sourceCore.outputs["release-commit"], c);
    await reconcileLock({ github, context, core: lockCore });
    assert.equal(lockCore.outputs.commit, lockCommit);
    bash("release-create-tag-and-branch.sh", [repo, version, releaseBranch, c]);
  }
  assert.equal(sourceCreates, 1);
  assert.equal(reference(`tags/${version}`), c);
  assert.equal(reference(`heads/${releaseBranch}`), c);
  const rcCore = coreFixture({
    MODE: "release-candidate",
    VERSION: `v${channel}.0`,
    RELEASE_CANDIDATE: version
  });
  await reconcileLock({ github, context, core: rcCore });
  assert.equal(rcCore.outputs["de-source-commit"], "e".repeat(40));
  const publicationCore = coreFixture({ VERSION: version, SOURCE_SHA: c });
  const runs = async (query) => {
    assert.equal(query.head_sha, c);
    return [
      {
        id: 1,
        head_sha: c,
        head_branch: version,
        event: "push",
        status: "completed",
        conclusion: "success",
        html_url: "https://example.test/runs/1"
      }
    ];
  };
  await resumeReleasePublication({
    context,
    core: publicationCore,
    github: {
      rest: { actions: { listWorkflowRuns: runs } },
      paginate: (method, query) => method(query)
    }
  });
  assert.equal(
    publicationCore.outputs["run-url"],
    "https://example.test/runs/1"
  );

  const plan = JSON.parse(run("yq", ["-o=json", ".", originalPlan]));
  const { verifyReleaseManifest } = await import(
    pathToFileURL(
      path.join(repo, ".github/scripts/verify-release-manifest.mjs")
    ).href
  );
  const manifest = verifyReleaseManifest({
    plan,
    targets: plan.expectedOutputs,
    sourceSha: c,
    parentSha: a,
    versions: JSON.parse(run("yq", ["-o=json", ".", "versions.yaml"])),
    observed: {
      release: { tag: version, sourceCommit: c },
      cli: {},
      helm: {},
      images: [],
      downstream: {}
    },
    imageLocks: [],
    cliLock: { artifacts: [] },
    controllerLock: {
      releaseSourceCommit: c,
      deploymentEngine: { signedTag: version }
    }
  });
  assert.equal(manifest.checks.plan, "verified");
  t.diagnostic(
    `Frozen product: ${a}; tagged readers: ${productPack ? "original product tree" : compatibilitySource}; resolved release: ${c}`
  );

  verified = false;
  await assert.rejects(
    reconcileSource({ github, context, core: sourceCore }),
    /not GitHub-verified/
  );
  verified = true;
  lockCore.inputs.DE_DIGEST = `sha256:${"0".repeat(64)}`;
  await assert.rejects(
    reconcileLock({ github, context, core: lockCore }),
    /locked at/
  );
  git("push", "-q", "--force", "origin", `${m}:refs/tags/${version}`);
  assert.throws(() => validateController(c), /points at/);
  assert.equal(sourceCreates, 1);
});
