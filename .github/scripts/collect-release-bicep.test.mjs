// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import test from "node:test";
import { tool } from "./bicep-types.mjs";
import collect, {
  authenticateAwsGeneration,
  authenticateAwsSnapshot,
  awsCaptureJob,
  awsWorkflow,
  evidencePath,
  requestPath,
  requestAwsPublication,
  persistAwsEvidence,
  readAwsEvidence,
  selectAwsPublication,
  validateAwsEvidence
} from "./collect-release-bicep.mjs";
import {
  bicepExtensionTargets,
  plannedBicepExtensions
} from "./release-bicep-extensions.mjs";

const sourceSha = "a".repeat(40);
const awsSha = "b".repeat(40);
const controllerSha = "c".repeat(40);
const digest = `sha256:${"c".repeat(64)}`;
const plan = {
  schemaVersion: 2,
  version: "v0.62.0-rc.1",
  releaseType: "rc",
  expectedOutputs: {
    repository: "radius-project/radius",
    bicepExtensionsContract: "ghcr-v1",
    ociArtifacts: bicepExtensionTargets
  },
  siblingRepositories: [
    {
      name: "bicep-types-aws",
      repository: "radius-project/bicep-types-aws",
      sourceCommit: awsSha
    }
  ]
};

test("legacy plans make no AWS calls or state writes; changed snapshot bytes cannot persist", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "aws-evidence-test-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const planFile = join(directory, "plan.json");
  const legacy = structuredClone(plan);
  delete legacy.expectedOutputs.bicepExtensionsContract;
  legacy.expectedOutputs.ociArtifacts = legacy.expectedOutputs.ociArtifacts.map(
    (entry) => ({
      ...entry,
      repository: `biceptypes.azurecr.io/${entry.name === "aws-bicep-types" ? "aws" : "radius"}`
    })
  );
  const inputs = {
    directory,
    plan_file: planFile,
    source_sha: sourceSha,
    operation: "snapshot"
  };
  const outputs = {};
  const core = {
    getInput: (name) => inputs[name],
    setOutput: (name, value) => {
      outputs[name] = value;
    }
  };
  await writeFile(planFile, JSON.stringify(legacy));
  await collect({ github: undefined, core });
  assert.equal(outputs.ghcr, "false");
  const f = fixture();
  await writeFile(planFile, JSON.stringify(plan));
  await writeFile(
    join(directory, "aws-evidence.json"),
    JSON.stringify(f.evidence)
  );
  await mkdir(join(directory, "snapshot"));
  await writeFile(join(directory, "snapshot/capture.tar"), "changed");
  await assert.rejects(
    collect({ github: f.github, core }),
    /snapshot digest differs/
  );
  assert.equal(f.state.refWrites, 0);
});
function fixture() {
  const record = {
    ...plannedBicepExtensions(plan, sourceSha)[0],
    digest,
    generation: {
      workflow: awsWorkflow,
      runId: 10,
      runAttempt: 1,
      artifactId: 20,
      artifactDigest: digest
    }
  };
  const evidence = {
    schemaVersion: 1,
    version: plan.version,
    releaseSourceCommit: sourceSha,
    artifact: record,
    publishing: {
      workflow: awsWorkflow,
      runId: 11,
      runAttempt: 2,
      artifactId: 21,
      artifactDigest: digest
    }
  };
  const run = {
    id: 10,
    run_attempt: 1,
    head_sha: controllerSha,
    head_branch: "main",
    display_title: `Publish AWS ${plan.version} at ${awsSha}`,
    path: awsWorkflow,
    event: "repository_dispatch",
    repository: { full_name: record.source.repository },
    head_repository: { full_name: record.source.repository },
    status: "completed",
    conclusion: "success"
  };
  const job = {
    name: awsCaptureJob,
    status: "completed",
    conclusion: "success",
    started_at: "2026-10-01T10:00:00Z",
    completed_at: "2026-10-01T10:02:00Z"
  };
  const artifact = {
    id: 20,
    name: "bicep-types-aws-10.tar",
    digest,
    expired: false,
    size_in_bytes: 100,
    created_at: "2026-10-01T10:01:00Z",
    workflow_run: { id: 10, head_sha: controllerSha, head_branch: "main" }
  };
  const state = {
    evidence: undefined,
    parent: sourceSha,
    files: [{ filename: evidencePath, status: "added" }],
    refWrites: 0,
    failCreate: false
  };
  const notFound = () => {
    throw Object.assign(new Error("not found"), { status: 404 });
  };
  const github = {
    paginate: async (method) => method(),
    rest: {
      actions: {
        getWorkflowRunAttempt: async ({ run_id, attempt_number }) => {
          assert.equal(run_id, 10);
          assert.equal(attempt_number, 1);
          return { data: run };
        },
        getArtifact: async () => ({ data: artifact }),
        listJobsForWorkflowRunAttempt: async () => [job],
        listWorkflowRuns: async () => [{ ...run, id: 11, run_attempt: 2 }],
        listWorkflowRunArtifacts: async () => [
          {
            ...artifact,
            id: 21,
            name: "aws-bicep-extension-11-2",
            workflow_run: { ...artifact.workflow_run, id: 11 }
          }
        ]
      },
      git: {
        getRef: async () =>
          state.evidence ?
            { data: { object: { sha: "d".repeat(40) } } }
          : notFound(),
        getCommit: async () => ({ data: { tree: { sha: "e".repeat(40) } } }),
        createTree: async ({ tree }) => {
          assert.equal(tree.length, 1);
          assert.ok([evidencePath, requestPath].includes(tree[0].path));
          state.files = [{ filename: tree[0].path, status: "added" }];
          state.pending = JSON.parse(tree[0].content);
          return { data: { sha: "f".repeat(40) } };
        },
        createCommit: async ({ parents }) => {
          assert.deepEqual(parents, [sourceSha]);
          return { data: { sha: "d".repeat(40) } };
        },
        createRef: async ({ ref }) => {
          assert.ok(
            [
              "refs/heads/automation/bicep-release-state-0.62.0-rc.1",
              "refs/heads/automation/aws-bicep-request-0.62.0-rc.1"
            ].includes(ref)
          );
          state.refWrites++;
          if (state.evidence)
            throw Object.assign(new Error("exists"), { status: 422 });
          state.evidence = state.pending;
          if (state.failCreate) throw new Error("uncertain create response");
        }
      },
      repos: {
        compareCommits: async ({ base, head }) => {
          assert.equal(head, controllerSha);
          return {
            data: {
              status:
                [awsSha, controllerSha].includes(base) ? "ahead" : "diverged"
            }
          };
        },
        getCommit: async ({ ref }) =>
          ref.startsWith("refs/tags/") ? { data: { sha: awsSha } }
          : ref === "refs/heads/main" ? { data: { sha: controllerSha } }
          : {
              data: { parents: [{ sha: state.parent }], files: state.files }
            },
        getContent: async () => ({
          data: {
            type: "file",
            encoding: "base64",
            content: Buffer.from(JSON.stringify(state.evidence)).toString(
              "base64"
            )
          }
        })
      }
    }
  };
  return { github, record, evidence, run, artifact, job, state };
}

test("trusted-main A1 authenticates distinct publication and original capture attempts", async () => {
  const f = fixture();
  assert.deepEqual(
    await selectAwsPublication(f.github, plan, sourceSha),
    f.evidence.publishing
  );
  await authenticateAwsGeneration(f.github, plan, sourceSha, f.record);
  validateAwsEvidence(plan, sourceSha, f.evidence);
});

for (const [name, mutate] of [
  [
    "source",
    (f) => {
      f.run.head_sha = sourceSha;
    }
  ],
  [
    "fork",
    (f) => {
      f.run.head_repository.full_name = "fork/bicep-types-aws";
    }
  ],
  [
    "tag",
    (f) => {
      f.run.head_branch = plan.version;
    }
  ],
  [
    "workflow",
    (f) => {
      f.run.path = ".github/workflows/other.yaml";
    }
  ],
  [
    "event",
    (f) => {
      f.run.event = "pull_request";
    }
  ],
  [
    "attempt",
    (f) => {
      f.run.run_attempt = 2;
    }
  ],
  [
    "expired",
    (f) => {
      f.artifact.expired = true;
    }
  ],
  [
    "digest",
    (f) => {
      f.artifact.digest = `sha256:${"d".repeat(64)}`;
    }
  ],
  [
    "ownership",
    (f) => {
      f.artifact.workflow_run.id = 12;
    }
  ],
  [
    "capture failure",
    (f) => {
      f.job.conclusion = "failure";
    }
  ],
  [
    "capture timestamp",
    (f) => {
      f.artifact.created_at = "2026-10-01T09:00:00Z";
    }
  ]
]) {
  test(`rejects invalid AWS generation ${name}`, async () => {
    const f = fixture();
    mutate(f);
    await assert.rejects(
      authenticateAwsGeneration(f.github, plan, sourceSha, f.record)
    );
  });
}

test("missing, failed, ambiguous and unauthorized AWS publication never succeeds", async () => {
  for (const type of ["missing", "failed", "ambiguous", "unauthorized"]) {
    const f = fixture();
    f.github.rest.actions.listWorkflowRuns = async () => {
      if (type === "unauthorized")
        throw Object.assign(new Error("denied"), { status: 403 });
      if (type === "missing") return [];
      if (type === "failed") return [{ ...f.run, conclusion: "failure" }];
      return [f.run, { ...f.run, id: 12 }];
    };
    let time = 0;
    await assert.rejects(
      selectAwsPublication(f.github, plan, sourceSha, {
        now: () => time,
        sleep: async () => {
          time += 600_000;
        }
      })
    );
  }
});

test("dedicated state creates once, reads back and never changes DE state", async () => {
  const f = fixture();
  assert.equal(await readAwsEvidence(f.github, plan, sourceSha), undefined);
  await persistAwsEvidence(f.github, plan, sourceSha, f.evidence);
  await persistAwsEvidence(f.github, plan, sourceSha, f.evidence);
  assert.equal(f.state.refWrites, 1);
  assert.deepEqual(
    await readAwsEvidence(f.github, plan, sourceSha),
    f.evidence
  );
  const changed = structuredClone(f.evidence);
  changed.artifact.digest = `sha256:${"e".repeat(64)}`;
  await assert.rejects(
    persistAwsEvidence(f.github, plan, sourceSha, changed),
    /conflicts/
  );
});

test("uncertain state creation recovers only identical committed content", async () => {
  const f = fixture();
  f.state.failCreate = true;
  await persistAwsEvidence(f.github, plan, sourceSha, f.evidence);
  assert.equal(f.state.refWrites, 1);
});

test("state rejects wrong parent, altered files, source and new unknown selector", async () => {
  for (const mutate of [
    (f) => {
      f.state.parent = awsSha;
    },
    (f) => {
      f.state.files.push({ filename: "other", status: "added" });
    },
    (f) => {
      f.state.files[0].status = "modified";
    },
    (f) => {
      f.evidence.artifact.source.commit = sourceSha;
    }
  ]) {
    const f = fixture();
    f.state.evidence = f.evidence;
    mutate(f);
    await assert.rejects(readAwsEvidence(f.github, plan, sourceSha));
  }
  const f = fixture();
  await assert.rejects(
    readAwsEvidence(
      f.github,
      {
        ...plan,
        expectedOutputs: {
          ...plan.expectedOutputs,
          bicepExtensionsContract: "unknown"
        }
      },
      sourceSha
    ),
    /Unknown/
  );
});

test("publishing and original generation authenticate independent main controllers", async () => {
  const f = fixture();
  const newer = "d".repeat(40);
  f.github.rest.actions.listWorkflowRuns = async () => [
    {
      ...f.run,
      id: 11,
      run_attempt: 2,
      head_sha: newer
    }
  ];
  const artifacts = f.github.rest.actions.listWorkflowRunArtifacts;
  f.github.rest.actions.listWorkflowRunArtifacts = async () =>
    (await artifacts()).map((artifact) => ({
      ...artifact,
      workflow_run: { ...artifact.workflow_run, head_sha: newer }
    }));
  const observed = [];
  f.github.rest.repos.compareCommits = async ({ base }) => {
    observed.push(base);
    return { data: { status: "ahead" } };
  };
  await selectAwsPublication(f.github, plan, sourceSha);
  await authenticateAwsGeneration(f.github, plan, sourceSha, f.record);
  assert.ok(observed.includes(newer));
  assert.ok(observed.includes(controllerSha));
  assert.ok(observed.includes(awsSha));
});

test("run title never authorizes a wrong publishing origin or planned tag", async () => {
  for (const mutate of [
    (f) => {
      f.run.event = "push";
    },
    (f) => {
      f.run.head_branch = plan.version;
    },
    (f) => {
      f.run.head_sha = "d".repeat(40);
    },
    (f) => {
      f.run.path = ".github/workflows/other.yaml";
    },
    (f) => {
      f.run.repository.full_name = "fork/bicep-types-aws";
    },
    (f) => {
      f.github.rest.repos.getCommit = async () => ({
        data: { sha: sourceSha }
      });
    },
    (f) => {
      f.github.rest.repos.compareCommits = async () => {
        throw Object.assign(new Error("denied"), { status: 403 });
      };
    }
  ]) {
    const f = fixture();
    mutate(f);
    let dispatched = false;
    await assert.rejects(
      selectAwsPublication(f.github, plan, sourceSha, {
        dispatch: async () => {
          dispatched = true;
        }
      })
    );
    assert.equal(dispatched, false);
  }
});

test("raw snapshot binds frozen source and original controller only after hashing", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "aws-controller-source-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const f = fixture();
  const metadata = {
    source: {
      repository: f.record.source.repository,
      ref: `refs/tags/${plan.version}`,
      commit: awsSha,
      workflow: awsWorkflow,
      runId: 10,
      generationAttempt: 1,
      controller: {
        repository: f.record.source.repository,
        ref: "refs/heads/main",
        commit: controllerSha
      }
    },
    manifestDigest: digest
  };
  const file = join(directory, "snapshot.tar");
  await writeFile(join(directory, "source.json"), JSON.stringify(metadata));
  await tool(["tar", "-cf", file, "-C", directory, "source.json"]);
  const { readFile } = await import("node:fs/promises");
  const bytes = await readFile(file);
  f.record.generation.artifactDigest = `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
  f.artifact.digest = f.record.generation.artifactDigest;
  await authenticateAwsSnapshot(f.github, plan, sourceSha, f.record, file);
  for (const mutate of [
    (m) => {
      m.source.commit = sourceSha;
    },
    (m) => {
      m.source.ref = "refs/tags/v9.0.0";
    },
    (m) => {
      m.source.repository = "fork/bicep-types-aws";
    },
    (m) => {
      m.source.workflow = ".github/workflows/other.yaml";
    },
    (m) => {
      m.source.runId = 11;
    },
    (m) => {
      m.source.generationAttempt = 2;
    },
    (m) => {
      m.source.controller.commit = awsSha;
    },
    (m) => {
      m.source.controller.ref = `refs/tags/${plan.version}`;
    },
    (m) => {
      m.source.controller.repository = "fork/bicep-types-aws";
    },
    (m) => {
      delete m.source.controller;
    },
    (m) => {
      m.manifestDigest = "bad";
    }
  ]) {
    const changed = structuredClone(metadata);
    mutate(changed);
    await assert.rejects(
      authenticateAwsSnapshot(
        f.github,
        plan,
        sourceSha,
        f.record,
        file,
        async () => JSON.stringify(changed)
      )
    );
  }
  await writeFile(file, "corrupt");
  await assert.rejects(
    authenticateAwsSnapshot(
      f.github,
      plan,
      sourceSha,
      f.record,
      file,
      async () => {
        assert.fail("must not parse before hash validation");
      }
    ),
    /snapshot digest differs/
  );
});

function dispatcher() {
  const calls = [];
  return {
    calls,
    rest: {
      repos: {
        createDispatchEvent: async (args) => {
          calls.push(args);
        }
      }
    }
  };
}

test("request winner dispatches once; existing and concurrent markers only discover", async () => {
  const f = fixture();
  const client = dispatcher();
  await Promise.all([
    requestAwsPublication(f.github, client, plan, sourceSha),
    requestAwsPublication(f.github, client, plan, sourceSha)
  ]);
  await requestAwsPublication(f.github, client, plan, sourceSha);
  assert.equal(client.calls.length, 1);
  const { request, ...payload } = client.calls[0];
  assert.equal(request.retries, 0);
  assert.deepEqual(payload, {
    owner: "radius-project",
    repo: "bicep-types-aws",
    event_type: "aws-bicep-release",
    client_payload: { version: plan.version, sourceCommit: awsSha }
  });
  assert.equal(f.state.files[0].filename, requestPath);
  assert.equal(f.state.evidence.releaseSourceCommit, sourceSha);
});

test("lost POST and crash before POST retain the marker and never repost on resume", async () => {
  for (const beforePost of [false, true]) {
    const f = fixture();
    const client = dispatcher();
    let posts = 0;
    client.rest.repos.createDispatchEvent = async () => {
      posts++;
      throw new Error("accepted but response lost");
    };
    if (beforePost) f.state.failCreate = true;
    if (beforePost)
      await requestAwsPublication(f.github, client, plan, sourceSha);
    else
      await assert.rejects(
        requestAwsPublication(f.github, client, plan, sourceSha),
        /response lost/
      );
    assert.ok(f.state.evidence);
    await requestAwsPublication(f.github, client, plan, sourceSha);
    assert.equal(posts, beforePost ? 0 : 1);
  }
});

test("request marker rejects conflicting identity/history and all read/auth failures", async () => {
  for (const mutate of [
    (f) => {
      f.state.evidence.clientPayload.sourceCommit = sourceSha;
    },
    (f) => {
      f.state.evidence.clientPayload.version = "v9.0.0";
    },
    (f) => {
      f.state.evidence.repository = "fork/bicep-types-aws";
    },
    (f) => {
      f.state.parent = awsSha;
    },
    (f) => {
      f.state.files[0].filename = evidencePath;
    },
    (f) => {
      f.state.files[0].status = "modified";
    },
    (f) => {
      f.github.rest.git.getRef = async () => {
        throw Object.assign(new Error("denied"), { status: 403 });
      };
    }
  ]) {
    const f = fixture();
    const client = dispatcher();
    await requestAwsPublication(f.github, client, plan, sourceSha);
    mutate(f);
    await assert.rejects(
      requestAwsPublication(f.github, client, plan, sourceSha)
    );
    assert.equal(client.calls.length, 1);
  }
  for (const status of [401, 403]) {
    const f = fixture();
    const client = dispatcher();
    f.github.rest.git.createRef = async () => {
      throw Object.assign(new Error("denied"), { status });
    };
    await assert.rejects(
      requestAwsPublication(f.github, client, plan, sourceSha)
    );
    assert.equal(client.calls.length, 0);
  }
});

test("marker readback must succeed before POST", async () => {
  const f = fixture();
  const client = dispatcher();
  const create = f.github.rest.git.createRef;
  f.github.rest.git.createRef = async (args) => {
    await create(args);
    f.state.parent = awsSha;
  };
  await assert.rejects(
    requestAwsPublication(f.github, client, plan, sourceSha)
  );
  assert.equal(client.calls.length, 0);
});

test("bounded discovery dispatches only after an empty authenticated search", async () => {
  for (const kind of [
    "absent",
    "active",
    "success",
    "failed",
    "unauthorized"
  ]) {
    const f = fixture();
    let time = 0;
    let calls = 0;
    const original = f.github.rest.actions.listWorkflowRuns;
    f.github.rest.actions.listWorkflowRuns = async () => {
      if (kind === "unauthorized")
        throw Object.assign(new Error("denied"), { status: 403 });
      if (kind === "absent") return [];
      const runs = await original();
      if (kind === "active") runs[0].status = "in_progress";
      if (kind === "failed") runs[0].conclusion = "failure";
      return runs;
    };
    const promise = selectAwsPublication(f.github, plan, sourceSha, {
      dispatch: async () => {
        calls++;
      },
      now: () => time,
      sleep: async () => {
        time += 600_000;
      }
    });

    if (kind === "success") await promise;
    else await assert.rejects(promise);
    assert.equal(calls, kind === "absent" ? 1 : 0);
  }
});

test("incomplete or over-budget discovery cannot authorize a dispatch", async () => {
  for (const paginate of [
    async () => {
      throw new Error("later history page failed");
    },
    async (method, options, map) => {
      map({ data: [] });
      return [];
    }
  ]) {
    const f = fixture();
    let time = 0;
    let dispatched = false;
    f.github.paginate = async (...args) => {
      time = 600_000;
      return paginate(...args);
    };
    await assert.rejects(
      selectAwsPublication(f.github, plan, sourceSha, {
        now: () => time,
        dispatch: async () => {
          dispatched = true;
        }
      })
    );
    assert.equal(dispatched, false);
  }
});
