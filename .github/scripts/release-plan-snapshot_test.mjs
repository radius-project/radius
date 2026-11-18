import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

assert.ok(
  process.env.SNAPSHOT_SOURCE_DIR && process.env.SNAPSHOT_BASE_SHA,
  "Run bash .github/scripts/validate-release-plan_test.sh --snapshots-only"
);

const require = createRequire(import.meta.url);
const workflow = readFileSync(
  new URL("../workflows/release-plan.yaml", import.meta.url),
  "utf8"
).replace(/\r\n/g, "\n");
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
const steps = Object.fromEntries(
  [
    ["pull_request_target", "Read pull request body and changed files"],
    ["merge_group", "Read queued release pull request data"]
  ].map(([event, name]) => {
    const step = workflow
      .split(`      - name: ${name}\n`)[1]
      ?.split("\n      - name:")[0];
    const script = step?.match(/          script: \|\n([\s\S]*)/)?.[1];
    assert.ok(script, `${name} must have a production script`);
    return [
      event,
      new AsyncFunction(
        "github",
        "context",
        "process",
        "require",
        script.replace(/^            /gm, "")
      )
    ];
  })
);
const changedFiles = JSON.parse(readFileSync("files.json", "utf8"));
const body = readFileSync("body.md", "utf8");
const headSha = "a".repeat(40);
const repository = { owner: "radius-project", repo: "radius" };

function fixture(t, event, files = changedFiles) {
  const directory = mkdtempSync(path.join(tmpdir(), "release-plan-snapshot-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const env = {
    BODY_FILE: path.join(directory, "body.md"),
    FILES_FILE: path.join(directory, "files.json"),
    HEAD_DIR: path.join(directory, "head"),
    CANDIDATES_FILE: path.join(directory, "candidates.json"),
    PULL_NUMBER: "42"
  };
  const snapshot = {
    number: 42,
    head_sha: headSha,
    body,
    files: [...files].sort()
  };
  writeFileSync(env.CANDIDATES_FILE, JSON.stringify([snapshot]));
  const state = {
    env,
    files: [...files],
    pull: { number: 42, head: { sha: headSha }, body },
    requests: [],
    contents: new Map(
      [...new Set([...changedFiles, "CHANGELOG.md"])].map((filename) => [
        filename,
        {
          type: "file",
          content: readFileSync(
            path.join(process.env.SNAPSHOT_SOURCE_DIR, filename)
          ).toString("base64")
        }
      ])
    )
  };
  const github = {
    paginate: async (method, request) => method(request),
    rest: {
      pulls: {
        get: async () => ({ data: state.pull }),
        listFiles: async (request) => {
          assert.deepEqual(request, {
            ...repository,
            pull_number: 42,
            per_page: 100
          });
          return state.files.map((filename) => ({ filename }));
        }
      },
      repos: {
        getContent: async (request) => {
          state.requests.push(request);
          assert.deepEqual(request, {
            ...repository,
            path: request.path,
            ref: headSha
          });
          if (!state.contents.has(request.path)) {
            throw new Error(`Not Found: ${request.path} at ${request.ref}`);
          }
          const data = state.contents.get(request.path);
          if (data instanceof Error) throw data;
          return { data };
        }
      }
    }
  };
  state.run = () =>
    steps[event](
      github,
      {
        repo: repository,
        eventName: event,
        sha: process.env.SNAPSHOT_BASE_SHA,
        payload: { pull_request: state.pull }
      },
      { env },
      require
    );
  state.assertSnapshot = () => {
    const expected = event === "merge_group" ? snapshot.files : files;
    assert.deepEqual(
      JSON.parse(readFileSync(env.FILES_FILE, "utf8")),
      expected
    );
    assert.equal(readFileSync(env.BODY_FILE, "utf8"), body);
    assert.deepEqual(
      state.requests.map((request) => request.path),
      [...new Set([...expected, "CHANGELOG.md"])]
    );
    for (const filename of new Set([...files, "CHANGELOG.md"])) {
      assert.deepEqual(
        readFileSync(path.join(env.HEAD_DIR, filename)),
        Buffer.from(state.contents.get(filename).content, "base64")
      );
    }
  };
  assert.equal(existsSync(env.HEAD_DIR), false);
  return state;
}

function validate(state, error) {
  const result = spawnSync(
    "bash",
    [
      fileURLToPath(new URL("./validate-release-plan.sh", import.meta.url)),
      "--body-file",
      state.env.BODY_FILE,
      "--files-file",
      state.env.FILES_FILE,
      "--base-sha",
      process.env.SNAPSHOT_BASE_SHA,
      "--head-dir",
      state.env.HEAD_DIR,
      "--repository",
      "radius-project/radius",
      "--backports-file",
      process.env.EXPECTED_BACKPORTS_FILE
    ],
    { encoding: "utf8" }
  );
  assert.ifError(result.error);
  const output = `${result.stdout}${result.stderr}`;
  if (error) {
    assert.notEqual(result.status, 0, output);
    assert.match(output, error);
  } else {
    assert.equal(result.status, 0, output);
    assert.match(output, /Release plan v0\.61\.0-rc\.1 is valid/);
  }
}

for (const event of Object.keys(steps)) {
  test(`${event} materializes stable-only RC inputs for the validator`, async (t) => {
    const state = fixture(t, event);
    assert.equal(changedFiles.length, 3);
    assert.ok(!changedFiles.includes("CHANGELOG.md"));
    await state.run();
    validate(state);
    state.assertSnapshot();
  });

  for (const [releaseType, version] of [
    ["final", "v0.61.0"],
    ["patch", "v0.61.1"]
  ]) {
    test(`${event} downloads a changed ${releaseType} changelog only once`, async (t) => {
      const files = [
        `.github/release-plans/${version}.yaml`,
        `docs/release-notes/${version}.md`,
        "versions.yaml",
        "CHANGELOG.md"
      ];
      const state = fixture(t, event, files);
      for (const filename of files) {
        state.contents.set(filename, {
          type: "file",
          content: Buffer.from(`${releaseType}: ${filename}\n`).toString(
            "base64"
          )
        });
      }
      await state.run();
      state.assertSnapshot();
    });
  }

  test(`${event} fails when the required unchanged changelog is missing`, async (t) => {
    const state = fixture(t, event);
    state.contents.delete("CHANGELOG.md");
    await assert.rejects(state.run, /Not Found: CHANGELOG\.md at/);
    assert.equal(
      existsSync(path.join(state.env.HEAD_DIR, "CHANGELOG.md")),
      false
    );
  });

  test(`${event} propagates errors reading the required changelog`, async (t) => {
    const state = fixture(t, event);
    const error = new Error("Forbidden: CHANGELOG.md");
    state.contents.set("CHANGELOG.md", error);
    await assert.rejects(state.run, error);
  });

  for (const [name, data] of [
    ["directory listing", []],
    ["directory", { type: "dir", content: "YQ==" }],
    ["symlink", { type: "symlink", content: "YQ==" }],
    ["missing content", { type: "file" }],
    ["empty content", { type: "file", content: "" }]
  ]) {
    test(`${event} rejects a required changelog with ${name}`, async (t) => {
      const state = fixture(t, event);
      state.contents.set("CHANGELOG.md", data);
      await assert.rejects(state.run, /Expected a file at CHANGELOG\.md/);
      assert.equal(
        existsSync(path.join(state.env.HEAD_DIR, "CHANGELOG.md")),
        false
      );
    });
  }

  for (const filename of ["../escape.md", ".", "../head-other/escape.md"]) {
    test(`${event} rejects the unsafe path ${filename}`, async (t) => {
      const state = fixture(t, event, [filename]);
      await assert.rejects(state.run, /Unsafe pull request path:/);
      assert.deepEqual(state.requests, []);
      assert.equal(existsSync(state.env.HEAD_DIR), false);
    });
  }

  test(`${event} still rejects an RC that changes the changelog`, async (t) => {
    const state = fixture(t, event, [...changedFiles, "CHANGELOG.md"]);
    state.contents.set("CHANGELOG.md", {
      type: "file",
      content: Buffer.from("Unplanned entry\n").toString("base64")
    });
    await state.run();
    state.assertSnapshot();
    validate(state, /release PR changes files outside the generated contract/);
  });

  test(`${event} still compares unchanged changelog contents`, async (t) => {
    const state = fixture(t, event);
    state.contents.set("CHANGELOG.md", {
      type: "file",
      content: Buffer.from("Unplanned entry\n").toString("base64")
    });
    await state.run();
    validate(state, /CHANGELOG\.md differs from trusted regeneration/);
  });

  test(`${event} fails if a required input disappears before validation`, async (t) => {
    const state = fixture(t, event);
    await state.run();
    rmSync(path.join(state.env.HEAD_DIR, "CHANGELOG.md"));
    validate(state, /CHANGELOG\.md.*No such file or directory/);
  });
}

for (const [name, change] of [
  ["head", (state) => (state.pull.head.sha = "b".repeat(40))],
  ["body", (state) => (state.pull.body += "\nChanged")],
  ["files", (state) => state.files.push("unexpected.md")]
]) {
  test(`merge_group rejects ${name} drift before downloading inputs`, async (t) => {
    const state = fixture(t, "merge_group");
    change(state);
    await assert.rejects(
      state.run,
      /Release PR #42 changed during merge-group validation/
    );
    assert.deepEqual(state.requests, []);
    assert.equal(existsSync(state.env.BODY_FILE), false);
    assert.equal(existsSync(state.env.FILES_FILE), false);
    assert.equal(existsSync(state.env.HEAD_DIR), false);
  });
}

test("merge_group requires the captured PR snapshot", async (t) => {
  const state = fixture(t, "merge_group");
  writeFileSync(state.env.CANDIDATES_FILE, "[]");
  await assert.rejects(state.run, /No snapshot for release PR #42/);
  assert.deepEqual(state.requests, []);
});
