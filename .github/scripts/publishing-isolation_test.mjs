import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { runInNewContext } from "node:vm";
import { setup, testStatus } from "./cloud-test-context.mjs";

const repository = "radius-project/radius";
const workflows = new Map();
function workflow(name) {
  if (!workflows.has(name)) {
    workflows.set(
      name,
      JSON.parse(
        execFileSync("yq", ["-o=json", ".", `.github/workflows/${name}`], {
          encoding: "utf8"
        })
      )
    );
  }
  return workflows.get(name);
}

const cloud = workflow("functional-test-cloud.yaml");
function jobStep(job, name) {
  const step = job.steps.find((item) => item.name === name);
  assert.ok(step, `Missing step: ${name}`);
  return step;
}

function context(overrides = {}) {
  return {
    github: {
      repository,
      ref: "refs/heads/main",
      ref_protected: true,
      event_name: "schedule",
      event: { pull_request: {} },
      ...overrides.github
    },
    needs: overrides.needs ?? {},
    inputs: {},
    vars: { RADIUS_REPOSITORY: repository },
    always: () => true,
    cancelled: () => overrides.cancelled ?? false,
    success: () => true,
    startsWith: (value, prefix) => value.startsWith(prefix)
  };
}

function allowed(job, values) {
  const expression = (
    job.if?.replace(/^\s*\${{\s*|\s*}}\s*$/g, "") ?? "true"
  ).replace(/\bneeds\.([A-Za-z_][A-Za-z0-9_-]*)/g, "needs['$1']");
  return Boolean(runInNewContext(expression, values, { timeout: 1000 }));
}

function completeNeeds() {
  return Object.fromEntries(
    [
      "authorize",
      "setup",
      "changes",
      "announce",
      "build",
      "upload-ghcr",
      "upload-test-types",
      "tests",
      "report-suite-results"
    ].map((name) => [
      name,
      { result: "success", outputs: { only_changed: "false" } }
    ])
  );
}

test("all validation callers reach only read-only reusable builds", () => {
  const validation = workflow("build-validation.yaml");
  for (const name of ["build-and-push-cli", "build-and-push-images"]) {
    const caller = validation.jobs[name];
    assert.deepEqual(caller.permissions, { contents: "read" });
    const callee = workflow(path.basename(caller.uses));
    for (const job of Object.values(callee.jobs)) {
      assert.deepEqual(job.permissions, { contents: "read" });
      for (const step of job.steps) {
        assert.doesNotMatch(
          JSON.stringify(step),
          /secrets\.|docker\/login|azure\/login/
        );
      }
    }
  }
  const cli = workflow("__build-cli.yaml").jobs["build-and-push-cli"];
  assert.deepEqual(
    cli.strategy.matrix.include.map(
      (item) => `${item.target_os}/${item.target_arch}`
    ),
    [
      "linux/amd64",
      "linux/arm64",
      "linux/arm",
      "windows/amd64",
      "windows/arm64",
      "darwin/amd64",
      "darwin/arm64"
    ]
  );
  const upload = jobStep(cli, "Upload CLI binary");
  assert.equal(upload.with["if-no-files-found"], "error");
  for (const callerName of ["build-main.yaml", "build-release.yaml"]) {
    assert.deepEqual(
      workflow(callerName).jobs["build-and-push-cli"].permissions,
      { contents: "read" }
    );
  }
  assert.ok(
    workflow("build-main.yaml").jobs["publish-cli"].needs.includes(
      "build-and-push-cli"
    )
  );
});

test("production publishing jobs reject PRs, merge groups, forks and unprotected refs", () => {
  const jobs = [
    workflow("__publish-cli.yaml").jobs.publish,
    workflow("__build-images.yaml").jobs["build-and-push-images"],
    workflow("__build-helm-chart.yaml").jobs["build-and-push-helm-chart"]
  ];
  for (const job of jobs) {
    assert.equal(
      allowed(job, context({ github: { event_name: "push" } })),
      true
    );
    for (const github of [
      { event_name: "pull_request", ref: "refs/pull/12/merge" },
      { event_name: "pull_request_target" },
      {
        event_name: "merge_group",
        ref: "refs/heads/gh-readonly-queue/main/pr-12"
      },
      { event_name: "workflow_dispatch", ref: "refs/heads/candidate" },
      { event_name: "push", ref_protected: false },
      { event_name: "push", repository: "fork/radius" }
    ]) {
      assert.equal(
        allowed(job, context({ github })),
        false,
        JSON.stringify(github)
      );
    }
  }
  assert.equal(
    allowed(
      jobs[0],
      context({
        github: {
          event_name: "push",
          ref: "refs/tags/v0.60.0"
        }
      })
    ),
    false
  );
});

test("production preflight validates actual main and release ref shell checks", () => {
  for (const [file, stepName, goodRefs] of [
    ["build-main.yaml", "Require protected main", ["refs/heads/main"]],
    [
      "build-release.yaml",
      "Require approved release tag",
      ["refs/tags/v0.60.0", "refs/tags/v0.61.0-rc1"]
    ]
  ]) {
    const step = jobStep(workflow(file).jobs["publication-ref"], stepName);
    for (const [ref, protectedRef, expected] of [
      ...goodRefs.map((ref) => [ref, "true", 0]),
      [goodRefs[0], "false", 1],
      ["refs/heads/candidate", "true", 1],
      ["refs/tags/vanother", "true", 1],
      ["refs/pull/12/merge", "true", 1]
    ]) {
      const result = spawnSync("bash", ["-e", "-c", step.run], {
        env: {
          ...process.env,
          GITHUB_REF: ref,
          GITHUB_REF_PROTECTED: protectedRef
        },
        encoding: "utf8"
      });
      assert.equal(
        result.status,
        expected,
        `${file}: ${ref}: ${result.stderr}`
      );
    }
    assert.ok(
      workflow(file).jobs["build-and-push-images"].needs.includes(
        "publication-ref"
      )
    );
    assert.ok(
      workflow(file).jobs["build-summary"].needs.includes("publication-ref")
    );
  }
});

test("candidate cloud jobs have no publishing or status-App authority", () => {
  const build = cloud.jobs.build;
  assert.deepEqual(build.permissions, { contents: "read" });
  assert.doesNotMatch(
    JSON.stringify(build),
    /secrets\.|azure\/login|docker\/login|create-github-app-token/
  );
  assert.match(
    jobStep(build, "Build and publish container images locally").env
      .DOCKER_REGISTRY,
    /^localhost:5000\//
  );
  assert.equal(
    jobStep(build, "Publish Bicep test recipes locally").env
      .BICEP_RECIPE_REGISTRY,
    "localhost:5000"
  );
  assert.match(
    jobStep(build, "Publish Radius types locally").run,
    /br:localhost:5000\/test\/radius:/
  );
  assert.deepEqual(cloud.jobs.tests.permissions, {
    "id-token": "write",
    contents: "read",
    packages: "read"
  });
  assert.doesNotMatch(
    JSON.stringify(cloud.jobs.tests),
    /create-github-app-token|FUNCTIONAL_TEST_APP_PRIVATE_KEY|BICEPTYPES_CLIENT_ID|sticky-pull-request/
  );
  assert.match(JSON.stringify(cloud.jobs.tests), /AZURE_SP_TESTS_APPID/);
  assert.match(JSON.stringify(cloud.jobs.tests), /AWS_GH_ACTIONS_ROLE/);
  assert.equal(
    jobStep(cloud.jobs.tests, "Run functional tests").env.GH_TOKEN,
    "${{ secrets.FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN }}"
  );
  const credentialCheck = jobStep(
    cloud.jobs.tests,
    "Require private-module read credential"
  );
  const missing = spawnSync("bash", ["-e", "-c", credentialCheck.run], {
    env: { ...process.env, MODULE_TOKEN: "" },
    encoding: "utf8"
  });
  assert.notEqual(missing.status, 0);
  assert.match(
    missing.stdout,
    /::error::Configure FUNCTIONAL_TEST_TERRAFORM_MODULES_READ_TOKEN/
  );
  assert.ok(build.needs.includes("announce"));
  assert.equal(
    allowed(
      build,
      context({
        needs: {
          ...completeNeeds(),
          announce: { result: "failure" }
        }
      })
    ),
    false
  );
});

test("TLS-local generation retains the existing secure registry configuration", () => {
  assert.equal(
    jobStep(cloud.jobs.build, "Create a job-local registry").with.secure,
    "true"
  );
  const directory = mkdtempSync(path.join(tmpdir(), "cloud-bicep-config-"));
  try {
    mkdirSync(path.join(directory, "test"));
    const generated = jobStep(
      cloud.jobs.build,
      "Generate test bicepconfig.json"
    );
    execFileSync("bash", ["-e", "-c", generated.run], {
      cwd: directory,
      env: {
        ...process.env,
        REL_VERSION: "pr-func0123456789",
        BICEP_TYPES_REGISTRY: "biceptypes.azurecr.io"
      }
    });
    const config = JSON.parse(
      readFileSync(path.join(directory, "test/bicepconfig.json"), "utf8")
    );
    assert.equal(
      config.extensions.radius,
      "br:localhost:5000/test/radius:pr-func0123456789"
    );
    assert.notEqual(config.experimentalFeaturesEnabled?.ociEnabled, true);
  } finally {
    rmSync(directory, { recursive: true });
  }
});

test("local registry certificates use validated hostnames without stdout configuration", () => {
  const action = JSON.parse(
    execFileSync(
      "yq",
      ["-o=json", ".", ".github/actions/create-local-registry/action.yaml"],
      { encoding: "utf8" }
    )
  );
  const script = jobStep(
    { steps: action.runs.steps },
    "Create certificates for local registry"
  ).run;
  const directory = mkdtempSync(path.join(tmpdir(), "cloud-registry-cert-"));
  try {
    const env = {
      ...process.env,
      INPUT_REGISTRY_NAME: "radius-registry",
      INPUT_REGISTRY_SERVER: "localhost",
      STEPS_CREATE_TEMP_CERT_DIR_OUTPUTS_TEMP_CERT_DIR: directory
    };
    const valid = spawnSync("bash", ["-e", "-c", script], {
      env,
      encoding: "utf8"
    });
    assert.equal(valid.status, 0, valid.stderr);
    assert.doesNotMatch(valid.stdout, /\[alt_names\]|DNS\.1|DNS\.2/);
    const config = readFileSync(path.join(directory, "req.cnf"), "utf8");
    assert.match(config, /DNS\.1 = radius-registry/);
    assert.match(config, /DNS\.2 = localhost/);
    const certificate = execFileSync(
      "openssl",
      [
        "x509",
        "-in",
        path.join(directory, "certs/localhost/client.crt"),
        "-noout",
        "-text"
      ],
      { encoding: "utf8" }
    );
    assert.match(certificate, /DNS:radius-registry, DNS:localhost/);
    for (const name of ["INPUT_REGISTRY_NAME", "INPUT_REGISTRY_SERVER"]) {
      const invalid = spawnSync("bash", ["-e", "-c", script], {
        env: {
          ...env,
          [name]: "localhost\n::set-output name=TEMP_CERT_DIR::/tmp/forged"
        },
        encoding: "utf8"
      });
      assert.notEqual(invalid.status, 0);
      assert.match(
        invalid.stdout,
        /::error::Local registry names must be DNS hostnames/
      );
      assert.doesNotMatch(invalid.stdout + invalid.stderr, /::set-output/);
    }
  } finally {
    rmSync(directory, { recursive: true });
  }
});

test("trusted cloud uploaders execute only reviewed data-copy helpers", () => {
  for (const name of ["upload-ghcr", "upload-test-types"]) {
    const job = cloud.jobs[name];
    assert.equal(job.permissions.actions, "read");
    assert.equal(
      job.permissions.packages,
      name === "upload-ghcr" ? "write" : undefined
    );
    assert.equal(
      job.permissions["id-token"],
      name === "upload-test-types" ? "write" : undefined
    );
    const checkout = jobStep(job, "Checkout trusted uploader");
    assert.equal(checkout.with.repository, repository);
    assert.equal(checkout.with.ref, "${{ needs.setup.outputs.TRUSTED_SHA }}");
    assert.equal(checkout.with["persist-credentials"], false);
    assert.equal(checkout.with["allow-unsafe-pr-checkout"], undefined);
    assert.equal(jobStep(job, "Setup Go").with.cache, false);
    assert.equal(job.env.ARTIFACT_ID, "${{ needs.build.outputs.ARTIFACT_ID }}");
    assert.equal(
      job.env.CHECKOUT_REF,
      "${{ needs.setup.outputs.CHECKOUT_REF }}"
    );
    for (const step of job.steps.filter((step) => step.run)) {
      assert.match(
        step.run,
        /^(make install-oras|python3 \.github\/scripts\/cloud-test-artifacts\.py (download|upload) |az acr login --name crradfunctest1b2s)/
      );
      assert.doesNotMatch(
        step.run,
        /docker (build|run|load)|rad bicep|docker-push|generate-bicep/
      );
    }
    const needs = completeNeeds();
    assert.equal(allowed(job, context({ needs })), true);
    for (const prerequisite of ["setup", "changes", "build"]) {
      for (const result of ["failure", "cancelled", "skipped"]) {
        const blocked = completeNeeds();
        blocked[prerequisite].result = result;
        assert.equal(allowed(job, context({ needs: blocked })), false);
      }
    }
    for (const decision of ["true", "", "unexpected"]) {
      needs.changes.outputs.only_changed = decision;
      assert.equal(allowed(job, context({ needs })), false);
    }
    assert.equal(
      allowed(
        job,
        context({
          github: { repository: "fork/radius" },
          needs: completeNeeds()
        })
      ),
      false
    );
  }
  for (const name of ["upload-ghcr", "upload-test-types"]) {
    assert.ok(cloud.jobs.tests.needs.includes(name));
    const needs = completeNeeds();
    needs[name].result = "failure";
    assert.equal(allowed(cloud.jobs.tests, context({ needs })), false);
  }
});

test("cloud authorization executes the actual fail-closed shell gate", () => {
  const gate = jobStep(cloud.jobs.authorize, "Evaluate trust and approval").run;
  for (const [event, trust, external, approval, expected] of [
    ["pull_request_target", "success", "false", "skipped", 0],
    ["pull_request_target", "success", "true", "success", 0],
    ["pull_request_target", "success", "true", "skipped", 1],
    ["pull_request_target", "success", "true", "cancelled", 1],
    ["pull_request_target", "success", "false", "failure", 1],
    ["pull_request_target", "failure", "", "skipped", 1],
    ["pull_request_target", "skipped", "", "skipped", 1],
    ["pull_request_target", "success", "", "skipped", 1],
    ["schedule", "skipped", "", "skipped", 0],
    ["merge_group", "skipped", "", "skipped", 0],
    ["workflow_dispatch", "failure", "", "skipped", 1]
  ]) {
    const result = spawnSync("bash", ["-e", "-c", gate], {
      env: {
        ...process.env,
        GITHUB_EVENT_NAME: event,
        CHECK_TRUST_RESULT: trust,
        IS_EXTERNAL: external,
        APPROVAL_GATE_RESULT: approval
      },
      encoding: "utf8"
    });
    assert.equal(
      result.status,
      expected,
      `${event}/${trust}/${external}/${approval}`
    );
  }
});

test("actual docs-only decision does not import candidate code and rejects missing results", async () => {
  const filter = jobStep(workflow("__changes.yml").jobs.changes, "Set result")
    .with.script;
  assert.doesNotMatch(filter, /import\(/);
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  for (const [event, base, decision, expected] of [
    ["pull_request", "", "true", "true"],
    ["pull_request_target", "", "false", "false"],
    ["workflow_dispatch", "", "true", "false"],
    ["schedule", "", "true", "false"],
    ["merge_group", "a".repeat(40), "true", "true"]
  ]) {
    let output;
    await new AsyncFunction("core", "context", filter)(
      {
        getInput: (name) => (name === "ONLY_CHANGED" ? decision : base),
        setOutput: (_, value) => {
          output = value;
        }
      },
      { eventName: event }
    );
    assert.equal(output, expected);
  }
  await assert.rejects(
    new AsyncFunction("core", "context", filter)(
      {
        getInput: () => "",
        setOutput: () => assert.fail("must not produce a skip decision")
      },
      { eventName: "pull_request" }
    ),
    /boolean decision/
  );
});

test("required cloud summary includes every phase, handles failures and intentional skips", () => {
  const needs = completeNeeds();
  assert.equal(testStatus(needs), "success");
  for (const name of Object.keys(needs)) {
    assert.ok(cloud.jobs["report-test-results"].needs.includes(name));
    for (const result of ["failure", "cancelled", "skipped", ""]) {
      const changed = completeNeeds();
      changed[name].result = result;
      assert.equal(
        testStatus(changed),
        result === "cancelled" ? "cancelled" : "failure"
      );
    }
  }
  needs.changes.outputs.only_changed = "true";
  for (const name of Object.keys(needs).slice(3)) {
    needs[name].result = "skipped";
  }
  assert.equal(testStatus(needs), "success");
  needs.changes.result = "failure";
  assert.equal(testStatus(needs), "failure");
  const cancelled = completeNeeds();
  cancelled.tests.result = "cancelled";
  cancelled.build.result = "failure";
  assert.equal(testStatus(cancelled), "failure");
  const invalid = completeNeeds();
  invalid.changes.outputs.only_changed = "";
  assert.equal(testStatus(invalid), "failure");
  const report = cloud.jobs["report-test-results"];
  assert.match(report.if, /always\(\)/);
  const failure = jobStep(report, "Require successful final status").run;
  assert.notEqual(
    spawnSync("bash", ["-e", "-c", failure], {
      env: { ...process.env, TEST_STATUS: "failure" }
    }).status,
    0
  );
});

test("only trusted reporters can hold the status App key", () => {
  for (const [name, job] of Object.entries(cloud.jobs)) {
    if (!JSON.stringify(job).includes("FUNCTIONAL_TEST_APP_PRIVATE_KEY")) {
      continue;
    }
    assert.ok(
      [
        "check-trust",
        "announce",
        "skip-tests",
        "report-suite-results",
        "report-test-results"
      ].includes(name),
      name
    );
    for (const checkout of job.steps.filter((step) =>
      step.uses?.startsWith("actions/checkout@")
    )) {
      assert.equal(checkout.with.repository, repository);
      assert.match(checkout.with.ref, /TRUSTED_SHA|refs\/heads\/main/);
    }
  }
  const suite = cloud.jobs["report-suite-results"];
  assert.ok(suite.needs.includes("tests"));
  assert.equal(
    jobStep(suite, "Publish suite result").with.commit,
    "${{ needs.setup.outputs.CHECKOUT_REF }}"
  );
  assert.equal(
    jobStep(suite, "Publish suite result").with.comment_mode,
    "failures"
  );
});

test("maintenance jobs reject canonical arbitrary refs while preserving safe fork testing", () => {
  const jobs = [
    workflow("long-running-azure.yaml").jobs.tests,
    workflow("purge-azure-test-resources.yaml").jobs.purge_bicep_types,
    workflow("repo-radius-state-e2e.yaml").jobs["state-rehydration"],
    workflow("repo-radius-state-e2e.yaml").jobs["cleanup-state-version"],
    workflow("devcontainer-feature-release.yaml").jobs.deploy
  ];
  for (const job of jobs) {
    assert.equal(allowed(job, context()), true);
    for (const github of [
      { ref: "refs/heads/feature", event_name: "workflow_dispatch" },
      { ref: "refs/tags/v0.60.0", event_name: "workflow_dispatch" },
      { ref_protected: false, event_name: "workflow_dispatch" }
    ]) {
      assert.equal(allowed(job, context({ github })), false);
    }
  }
  for (const name of ["state-rehydration", "cleanup-state-version"]) {
    assert.equal(
      allowed(
        workflow("repo-radius-state-e2e.yaml").jobs[name],
        context({
          github: {
            repository: "fork/radius",
            ref: "refs/heads/feature",
            event_name: "workflow_dispatch",
            ref_protected: false
          }
        })
      ),
      true
    );
  }
});

test("noncloud, snapshot, and Windows validation remain without registry credentials", () => {
  for (const file of [
    "functional-test-noncloud.yaml",
    "goreleaser-snapshot.yaml"
  ]) {
    for (const job of Object.values(workflow(file).jobs)) {
      assert.notEqual(job.permissions?.packages, "write");
    }
  }
  const windows = workflow("unit-tests.yaml").jobs["windows-cli-tests"];
  assert.deepEqual(windows.permissions, { contents: "read" });
  assert.equal(windows.strategy.matrix.include.length, 2);
  const docker = readFileSync("build/docker.mk", "utf8");
  for (const image of [
    "ucpd",
    "applications-rp",
    "dynamic-rp",
    "controller",
    "testrp",
    "magpiego",
    "bicep",
    "pre-upgrade"
  ]) {
    assert.ok(docker.includes(`${image}:`));
  }
  const tests = JSON.stringify(cloud.jobs.tests);
  for (const component of [
    "applications-rp",
    "dynamic-rp",
    "controller",
    "ucpd",
    "bicep"
  ]) {
    assert.ok(tests.includes(`\${CONTAINER_REGISTRY}/${component}`));
  }
  assert.match(tests, /TEST_BICEP_TYPES_REGISTRY/);
  assert.match(tests, /ociEnabled/);
});

test("trusted setup resolves exact PR, manual and merge-queue source identities", async () => {
  const baseContext = {
    eventName: "schedule",
    ref: "refs/heads/main",
    sha: "a".repeat(40),
    serverUrl: "https://github.com",
    runId: 101,
    runNumber: 3,
    payload: { repository: { full_name: repository } }
  };
  const cases = [
    ["schedule", {}, "a".repeat(40), repository],
    [
      "workflow_dispatch",
      { inputs: { branch: "candidate" } },
      "b".repeat(40),
      repository
    ],
    [
      "pull_request_target",
      {
        pull_request: {
          number: 12,
          base: {
            ref: "main",
            sha: "a".repeat(40),
            repo: { full_name: repository }
          },
          head: { sha: "b".repeat(40), repo: { full_name: "fork/radius" } }
        }
      },
      "b".repeat(40),
      "fork/radius"
    ],
    [
      "merge_group",
      {
        merge_group: { base_ref: "refs/heads/main", head_sha: "c".repeat(40) }
      },
      "c".repeat(40),
      repository
    ]
  ];
  for (const [eventName, payload, expectedSHA, expectedRepo] of cases) {
    const outputs = {};
    const current = {
      ...baseContext,
      eventName,
      payload: { ...baseContext.payload, ...payload }
    };
    if (eventName === "merge_group") {
      current.ref = "refs/heads/gh-readonly-queue/main/pr-12";
      current.sha = expectedSHA;
    }
    await setup({
      github: {
        rest: {
          repos: {
            getBranch: async ({ branch }) => ({
              data: {
                protected: true,
                commit: {
                  sha: branch === "candidate" ? "b".repeat(40) : "a".repeat(40)
                }
              }
            })
          }
        }
      },
      context: current,
      core: {
        setOutput: (name, value) => {
          outputs[name] = value;
        }
      }
    });
    assert.equal(outputs.CHECKOUT_REF, expectedSHA);
    assert.equal(outputs.CHECKOUT_REPO, expectedRepo);
    assert.equal(outputs.TRUSTED_SHA, "a".repeat(40));
    assert.match(outputs.REL_VERSION, /^pr-func[0-9a-f]{10}$/);
    assert.equal(outputs.RAD_CLI_ARTIFACT_NAME, "rad_cli_linux_amd64");
  }
  for (const changed of [
    { ref: "refs/heads/candidate", eventName: "workflow_dispatch" },
    { payload: { repository: { full_name: "fork/radius" } } },
    { eventName: "pull_request" }
  ]) {
    await assert.rejects(
      setup({
        github: {
          rest: {
            repos: {
              getBranch: async () => ({
                data: { protected: true, commit: { sha: "a".repeat(40) } }
              })
            }
          }
        },
        context: { ...baseContext, ...changed },
        core: { setOutput: () => assert.fail("untrusted source") }
      })
    );
  }
});

test("validation Build Check fails when docs-only detection fails or has no decision", () => {
  const check = jobStep(
    workflow("build-validation.yaml").jobs["build-check"],
    "Require successful change detection"
  );
  for (const [result, decision, expected] of [
    ["success", "true", 0],
    ["success", "false", 0],
    ["failure", "", 1],
    ["cancelled", "", 1],
    ["success", "", 1]
  ]) {
    assert.equal(
      spawnSync("bash", ["-e", "-c", check.run], {
        env: { ...process.env, CHANGES_RESULT: result, ONLY_CHANGED: decision }
      }).status,
      expected
    );
  }
});
