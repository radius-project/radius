// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import collect, {
  authenticateAwsGeneration,
  awsCaptureJob,
  awsWorkflow,
  evidencePath,
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
    operation: "persist"
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
    head_sha: awsSha,
    head_branch: plan.version,
    path: awsWorkflow,
    event: "push",
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
    workflow_run: { id: 10, head_sha: awsSha, head_branch: plan.version }
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
          assert.equal(tree[0].path, evidencePath);
          state.pending = JSON.parse(tree[0].content);
          return { data: { sha: "f".repeat(40) } };
        },
        createCommit: async ({ parents }) => {
          assert.deepEqual(parents, [sourceSha]);
          return { data: { sha: "d".repeat(40) } };
        },
        createRef: async ({ ref }) => {
          assert.equal(
            ref,
            "refs/heads/automation/bicep-release-state-0.62.0-rc.1"
          );
          state.refWrites++;
          state.evidence = state.pending;
          if (state.failCreate) throw new Error("uncertain create response");
        }
      },
      repos: {
        getCommit: async ({ ref }) =>
          ref.startsWith("refs/tags/") ?
            { data: { sha: awsSha } }
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

test("proposed A1 authenticates distinct publication and original capture attempts", async () => {
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
      f.run.head_branch = "main";
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
