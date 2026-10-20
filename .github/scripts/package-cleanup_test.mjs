// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.
// SPDX-License-Identifier: Apache-2.0

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  Client,
  Incomplete,
  REPOSITORY,
  PROTECTED_MESSAGE,
  hash,
  canonical,
  normalizeVersion,
  manifestEdges,
  collect,
  classify,
  deletionOrder,
  makePlan,
  validatePlan,
  applyPlan,
  checkEnvironment,
  verifyGate,
  verifyPlanRun,
  main
} from "./package-cleanup.mjs";

const now = Date.now();
const old = new Date(now - 30 * 86400000).toISOString();
const expires = new Date(now + 3600000).toISOString();
const d = (n) => `sha256:${String(n).padStart(64, "0")}`;
const release = { id: 1, tag_name: "v0.1.0-rc1", updated_at: old };
const identity = {
  sha: "a".repeat(40),
  ref: "refs/heads/main",
  policy: "b".repeat(64),
  runId: 123,
  runAttempt: 1
};
function evidence() {
  return {
    schema: 1,
    repository: REPOSITORY,
    historicalReviewComplete: true,
    activeReferenceReviewComplete: true,
    expiresAt: expires,
    review: `https://github.com/${REPOSITORY}/pull/1`,
    releases: [
      {
        id: 1,
        tag: release.tag_name,
        updatedAt: old,
        source: `https://github.com/${REPOSITORY}/releases/tag/v0.1.0-rc1`,
        digests: [d(99)]
      }
    ],
    protectedDigests: [],
    activeTags: [],
    testRecipePackages: []
  };
}
function version(id, tags = [`pr-${id}`], children = []) {
  return {
    id,
    digest: d(id),
    tags,
    createdAt: old,
    updatedAt: old,
    children,
    artifact: false
  };
}
function snapshot(versions = [version(1)]) {
  return {
    repository: REPOSITORY,
    complete: true,
    releases: [release],
    openPRs: [],
    activeRuns: [],
    packages: [
      {
        id: 1,
        name: "dev/example",
        repository: REPOSITORY,
        linked: true,
        versions
      }
    ]
  };
}
const options = { profile: "pr", retentionDays: 7, now };
const planFor = (s) => makePlan(s, evidence(), identity, options);
const response = (body, status = 200, headers = {}) => ({
  status,
  headers,
  body: typeof body === "string" ? body : JSON.stringify(body)
});
function raw(v) {
  return {
    id: v.id,
    name: v.digest,
    created_at: v.createdAt,
    updated_at: v.updatedAt,
    metadata: { container: { tags: v.tags } }
  };
}
function apply(s, codes = [], opts = {}) {
  const calls = [],
    persisted = [];
  const p = planFor(s);
  const c = new Client((service, method, path) => {
    calls.push({ service, method, path });
    if (path.includes("/releases?")) return response([release]);
    if (path.includes("/pulls?")) return response([]);
    if (method === "DELETE") return codes.shift() ?? response("", 204);
    const v = s.packages
      .flatMap((p) => p.versions)
      .find((v) => path.endsWith(`/${v.id}`));
    return response(raw(v));
  });
  const report = applyPlan(c, p, opts.fresh ?? s, {
    gate: () => {},
    checkpoint: (r) => persisted.push(structuredClone(r)),
    ...opts
  });
  return { report, calls, persisted };
}
test("canonical plan hash survives JSON serialization and property reordering", () => {
  assert.equal(
    canonical({ b: 2, a: 1, missing: undefined }),
    canonical({ a: 1, b: 2 })
  );
  const p = planFor(snapshot());
  validatePlan(
    JSON.parse(JSON.stringify(p)),
    hash(p),
    identity,
    evidence(),
    options,
    now
  );
});
test("scope: repository-linked only; unknown and legacy are held", () => {
  const s = snapshot();
  s.packages.push({ id: 2, name: "dev/unlinked", linked: false, versions: [] });
  s.packages.push({
    id: 3,
    name: "radius/controller",
    linked: true,
    versions: [version(2)]
  });
  const rows = classify(s, evidence(), options);
  assert.deepEqual(
    rows.map((r) => r.decision),
    ["delete-candidate", "hold", "hold"]
  );
});
test("protection wins over mixed tags, historical versions, both RC spellings, aliases and artifact tags", () => {
  const tags = [
    "0.1.0",
    "v0.1.0-rc1",
    "0.1.0-rc.1",
    "latest",
    "0.1",
    "edge",
    `sha256-${"a".repeat(64)}.sig`,
    "something-else"
  ];
  const rows = classify(
    snapshot(tags.map((t, i) => version(i + 1, ["pr-1", t]))),
    evidence(),
    options
  );
  assert.ok(rows.slice(0, -1).every((r) => r.decision === "keep"));
  assert.equal(rows.at(-1).decision, "hold");
});
test("every official release, including non-SemVer development entries, needs evidence", () => {
  const s = snapshot();
  s.releases.push({ id: 2, tag_name: "development-build", updated_at: old });
  assert.equal(classify(s, evidence(), options)[0].decision, "hold");
  s.packages[0].versions[0].tags.push("development-build");
  assert.equal(classify(s, evidence(), options)[0].decision, "keep");
});
test("unknown history, expired evidence, new releases and active-reference gaps hold", () => {
  for (const change of [
    (e) => {
      e.historicalReviewComplete = false;
    },
    (e) => {
      e.activeReferenceReviewComplete = false;
    },
    (e) => {
      e.expiresAt = old;
    },
    (e) => {
      e.releases = [];
    },
    (e) => {
      e.review = "";
    },
    (e) => {
      e.releases[0].updatedAt = expires;
    }
  ]) {
    const e = evidence();
    change(e);
    assert.equal(classify(snapshot(), e, options)[0].decision, "hold");
  }
  assert.throws(() =>
    classify(snapshot(), { ...evidence(), protectedDigests: ["bad"] }, options)
  );
});
test("active PR, active test tags, active runs and retention win", () => {
  const s = snapshot([version(12), version(13), version(14)]);
  s.openPRs = [{ number: 12 }];
  const e = evidence();
  e.activeTags = ["pr-13"];
  s.packages[0].versions[2].updatedAt = new Date(now).toISOString();
  assert.ok(classify(s, e, options).every((r) => r.decision === "keep"));
  const busy = snapshot();
  busy.activeRuns = [{ path: ".github/workflows/build-release.yaml" }];
  assert.equal(classify(busy, evidence(), options)[0].decision, "hold");
});
test("functional and explicitly reviewed old recipe profiles do not open arbitrary namespaces", () => {
  const s = snapshot([
    version(1, ["pr-func0123456789"]),
    version(2, ["test-func0123456789-123-1"])
  ]);
  const e = evidence();
  e.testRecipePackages = ["dev/example"];
  assert.deepEqual(
    classify(s, e, { ...options, profile: "functional" }).map(
      (r) => r.decision
    ),
    ["delete-candidate", "delete-candidate"]
  );
  assert.deepEqual(
    classify(s, e, { ...options, profile: "test-recipes" }).map(
      (r) => r.decision
    ),
    ["delete-candidate", "delete-candidate"]
  );
  assert.throws(() =>
    classify(s, { ...e, testRecipePackages: ["helm-chart/radius"] }, options)
  );
});
test("runtime functional tags require complete evidence and retain all safety guards", () => {
  const tag = "test-func0123456789-123-1";
  const s = snapshot([version(1, [tag])]);
  s.packages[0].name = "dev/ucpd";
  for (const profile of ["functional", "reviewed-tests"]) {
    const opts = { ...options, profile };
    assert.equal(classify(s, evidence(), opts)[0].decision, "delete-candidate");
    for (const flag of [
      "historicalReviewComplete",
      "activeReferenceReviewComplete"
    ]) {
      assert.equal(
        classify(s, { ...evidence(), [flag]: false }, opts)[0].decision,
        "hold"
      );
    }
    const mixed = structuredClone(s);
    mixed.packages[0].versions[0].tags.push("0.60.0-rc.1");
    assert.equal(classify(mixed, evidence(), opts)[0].decision, "keep");
    assert.equal(
      classify(s, { ...evidence(), activeTags: [tag] }, opts)[0].decision,
      "keep"
    );
    const recent = structuredClone(s);
    recent.packages[0].versions[0].updatedAt = new Date(now).toISOString();
    assert.equal(classify(recent, evidence(), opts)[0].decision, "keep");
    const incomplete = structuredClone(s);
    incomplete.packages[0].versions[0].children = [d(999)];
    assert.equal(classify(incomplete, evidence(), opts)[0].decision, "hold");
    const outside = structuredClone(s);
    outside.packages[0].name = "ucpd";
    assert.equal(classify(outside, evidence(), opts)[0].decision, "hold");
    const recipe = structuredClone(s);
    recipe.packages[0].versions[0].recipe = true;
    assert.equal(classify(recipe, evidence(), opts)[0].decision, "hold");
  }
  assert.equal(classify(s, evidence(), options)[0].decision, "hold");
});
test("known Bicep recipes require reviewed paths; release aliases and unrecognized tags still win", () => {
  const body = JSON.stringify({
    schemaVersion: 2,
    config: {
      mediaType: "application/vnd.ms.bicep.module.config.v1+json",
      digest: d(8)
    },
    layers: [
      {
        mediaType: "application/vnd.ms.bicep.module.layer.v1+json",
        digest: d(9)
      }
    ]
  });
  const graph = manifestEdges(body, `sha256:${hash(body)}`);
  assert.equal(graph.recipe, true);
  assert.equal(graph.artifact, false);
  const name = "test/testrecipes/test-bicep-recipes/redis";
  const s = snapshot([
    { ...version(1, ["test-func0123456789-123-1"]), ...graph },
    { ...version(2, ["latest"]), ...graph },
    { ...version(3, ["unknown"]), ...graph }
  ]);
  s.packages[0].name = name;
  const e = evidence();
  assert.equal(
    classify(s, e, { ...options, profile: "test-recipes" })[0].decision,
    "hold"
  );
  e.testRecipePackages = [name];
  assert.deepEqual(
    classify(s, e, { ...options, profile: "test-recipes" }).map(
      (r) => r.decision
    ),
    ["delete-candidate", "keep", "hold"]
  );
  assert.equal(classify(s, e, options)[0].decision, "hold");
});
test("untagged versions are never assumed unused; missing manifests and dependencies hold package", () => {
  assert.equal(
    classify(snapshot([version(1, [])]), evidence(), options)[0].decision,
    "hold"
  );
  for (const v of [
    { ...version(1), graphError: "HTTP 404" },
    version(1, ["pr-1"], [d(999)])
  ]) {
    assert.equal(
      classify(snapshot([v]), evidence(), options)[0].decision,
      "hold"
    );
  }
});
test("retained graph roots retain shared children; released children protect copies across namespaces", () => {
  const s = snapshot([
    version(1, ["pr-1"], [d(3)]),
    version(2, ["latest"], [d(3)]),
    version(3)
  ]);
  s.packages.push({
    ...s.packages[0],
    id: 2,
    name: "radius/dev/copy",
    versions: [version(3)]
  });
  const rows = classify(s, evidence(), options);
  assert.deepEqual(
    rows.map((r) => r.decision),
    ["delete-candidate", "keep", "keep", "keep"]
  );
  const unknown = snapshot([version(1, ["unknown"], [d(2)]), version(2)]);
  assert.ok(
    classify(unknown, evidence(), options).every((r) => r.decision === "hold")
  );
});
test("parent indexes are deleted before children; cycles are fatal", () => {
  const rows = classify(
    snapshot([version(2), version(1, ["pr-1"], [d(2)])]),
    evidence(),
    options
  );
  assert.deepEqual(
    deletionOrder(rows).map((r) => r.id),
    [1, 2]
  );
  const cyclic = classify(
    snapshot([version(1, ["pr-1"], [d(2)]), version(2, ["pr-2"], [d(1)])]),
    evidence(),
    options
  );
  assert.throws(() => deletionOrder(cyclic), /cycle/);
});
test("successful apply deletes parent before unused tagged children and reports complete", () => {
  const { report, calls } = apply(
    snapshot([version(2), version(1, ["pr-1"], [d(2)])])
  );
  assert.equal(report.status, "complete");
  assert.equal(report.counts.deleted, 2);
  assert.deepEqual(
    calls
      .filter((c) => c.method === "DELETE")
      .map((c) => c.path.split("/").at(-1)),
    ["1", "2"]
  );
});
test("a graph gap in another namespace blocks possible released-content copies", () => {
  const s = snapshot();
  s.packages.push({
    id: 2,
    name: "controller",
    linked: true,
    versions: [
      { ...version(99, ["0.1.0"]), graphError: "Metadata unavailable" }
    ]
  });
  assert.equal(classify(s, evidence(), options)[0].decision, "hold");
});
test("exact download protection warns and continues, but retains all descendants", () => {
  const s = snapshot([version(1, ["pr-1"], [d(2)]), version(2), version(3)]);
  const { report, calls } = apply(s, [
    response({ message: PROTECTED_MESSAGE }, 400),
    response("", 204)
  ]);
  assert.equal(report.status, "partial");
  assert.deepEqual(
    report.results.map((r) => r.status),
    ["protected", "unfinished", "deleted"]
  );
  assert.deepEqual(
    calls
      .filter((c) => c.method === "DELETE")
      .map((c) => c.path.split("/").at(-1)),
    ["1", "3"]
  );
});
test("near-match download refusals, malformed JSON, authorization, conflicts and unknown errors are fatal", () => {
  for (const r of [
    response({ message: `${PROTECTED_MESSAGE} ` }, 400),
    response("invalid", 400),
    response({ message: PROTECTED_MESSAGE }, 403),
    response("", 409),
    response("", 422),
    response([{ message: PROTECTED_MESSAGE }], 400),
    response({ message: null }, 400)
  ]) {
    const { report, calls } = apply(snapshot([version(1), version(2)]), [r]);
    assert.equal(report.status, "failed");
    assert.equal(calls.filter((c) => c.method === "DELETE").length, 1);
    assert.ok(report.results.every((r) => r.status === "unfinished"));
  }
});
test("404 is absent, not deleted, and retains descendants", () => {
  const { report } = apply(
    snapshot([version(1, ["pr-1"], [d(2)]), version(2)]),
    [response("", 404)]
  );
  assert.deepEqual(
    report.results.map((r) => r.status),
    ["already-absent", "unfinished"]
  );
  assert.equal(report.status, "partial");
});
test("caps, primary and secondary rate limits save partial results and unfinished work", () => {
  const s = snapshot([version(1), version(2)]);
  for (const headers of [{ remaining: "0" }, { retryAfter: "300" }]) {
    const { report, persisted } = apply(s, [
      response("", 204),
      response("", 403, headers)
    ]);
    assert.equal(report.status, "partial");
    assert.deepEqual(
      report.results.map((r) => r.status),
      ["deleted", "unfinished"]
    );
    assert.ok(persisted.length >= 3);
  }
  assert.equal(apply(s, [], { maxDeletes: 1 }).report.status, "partial");
  const client = new Client(() => response([]), 1);
  client.get("/repos/radius-project/radius/releases");
  assert.throws(
    () => client.get("/repos/radius-project/radius/releases"),
    Incomplete
  );
});
test("apply rechecks exact version, blocks changed tags/digest/timestamp and fresh protection", () => {
  for (const field of ["tags", "digest", "updatedAt"]) {
    const s = snapshot(),
      p = planFor(s);
    const changed = {
      ...s.packages[0].versions[0],
      [field]:
        field === "tags" ? ["latest"]
        : field === "digest" ? d(3)
        : expires
    };
    const calls = [];
    const client = new Client((service, method, path) => {
      calls.push(method);
      if (path.includes("/releases?")) return response([release]);
      if (path.includes("/pulls?")) return response([]);
      return response(raw(changed));
    });
    const report = applyPlan(client, p, s, { gate() {} });
    assert.equal(report.status, "partial");
    assert.ok(!calls.includes("DELETE"));
  }
  const s = snapshot(),
    fresh = structuredClone(s);
  fresh.packages[0].versions[0].tags.push("latest");
  const result = apply(s, [], { fresh });
  assert.equal(result.report.status, "partial");
  assert.equal(result.calls.filter((c) => c.method === "DELETE").length, 0);
});
test("freshly missing records are absent; graph children still stay unfinished", () => {
  const s = snapshot([version(1, ["pr-1"], [d(2)]), version(2)]),
    fresh = snapshot([version(2)]);
  const { report, calls } = apply(s, [], { fresh });
  assert.equal(report.results[0].status, "already-absent");
  assert.equal(report.results[1].status, "unfinished");
  assert.equal(calls.filter((c) => c.method === "DELETE").length, 0);
});
test("stale, future, tampered, wrong-source and wrong-policy plans fail validation", () => {
  const p = planFor(snapshot());
  assert.throws(() =>
    validatePlan(p, "0".repeat(64), identity, evidence(), options, now)
  );
  assert.throws(() =>
    validatePlan(p, hash(p), identity, evidence(), options, now + 7 * 3600000)
  );
  assert.throws(() =>
    validatePlan(p, hash(p), identity, evidence(), options, now - 1)
  );
  assert.throws(() =>
    validatePlan(
      p,
      hash(p),
      { ...identity, sha: "c".repeat(40) },
      evidence(),
      options,
      now
    )
  );
  assert.throws(() =>
    validatePlan(
      p,
      hash(p),
      identity,
      { ...evidence(), review: "changed" },
      options,
      now
    )
  );
  const altered = structuredClone(p);
  altered.records[0].decision = "keep";
  assert.throws(
    () =>
      validatePlan(altered, hash(altered), identity, evidence(), options, now),
    /policy\/result/
  );
  assert.throws(() =>
    validatePlan(
      p,
      hash(p),
      identity,
      evidence(),
      { ...options, retentionDays: 8 },
      now
    )
  );
});
test("malformed API records, unsupported graphs and forged manifests fail closed", () => {
  assert.throws(() => normalizeVersion({ id: 1 }));
  assert.throws(() =>
    normalizeVersion(raw({ ...version(1), tags: ["pr-1", "pr-1"] }))
  );
  assert.throws(() =>
    normalizeVersion(raw({ ...version(1), updatedAt: "not a date" }))
  );
  const manifest = JSON.stringify({
    schemaVersion: 2,
    mediaType: "application/vnd.oci.image.index.v1+json",
    manifests: [{ digest: d(1) }]
  });
  assert.deepEqual(
    manifestEdges(manifest, `sha256:${hash(manifest)}`).children,
    [d(1)]
  );
  assert.throws(() => manifestEdges(manifest, d(1)), /digest mismatch/);
  const artifact = JSON.stringify({
    schemaVersion: 2,
    mediaType: "application/vnd.oci.image.manifest.v1+json",
    subject: { digest: d(1) }
  });
  assert.deepEqual(
    manifestEdges(artifact, `sha256:${hash(artifact)}`).children,
    [d(1)]
  );
});
test("pagination collects later pages, rejects repeated identities and wrong response shapes", () => {
  let count = 0;
  const c = new Client(() =>
    response(
      ++count === 1 ?
        Array.from({ length: 100 }, (_, i) => ({ id: i + 1 }))
      : [{ id: 101 }]
    )
  );
  assert.equal(c.pages("/repos/radius-project/radius/releases").length, 101);
  assert.equal(count, 2);
  assert.throws(() =>
    new Client(() => response({ message: "no" })).pages(
      "/repos/radius-project/radius/releases"
    )
  );
  assert.throws(
    () =>
      new Client(() =>
        response(Array.from({ length: 100 }, (_, i) => ({ id: i + 1 })))
      ).pages("/repos/radius-project/radius/releases"),
    /repeated/
  );
  assert.throws(() =>
    new Client(() => ({ status: 200, body: "bad" })).get(
      "/repos/radius-project/radius/releases"
    )
  );
});
test("inventory includes only linked packages and records unknown links without sweeping other repos", () => {
  const manifest = JSON.stringify({
    schemaVersion: 2,
    mediaType: "application/vnd.oci.image.index.v1+json",
    manifests: []
  });
  const digest = `sha256:${hash(manifest)}`,
    v = { ...version(1), digest };
  const p = (id, name, repo) => ({
    id,
    name,
    package_type: "container",
    repository:
      repo ? { full_name: repo, owner: { login: "radius-project" } } : null
  });
  const packages = [
    p(1, "dev/example", REPOSITORY),
    p(2, "unknown", null),
    p(3, "other", "radius-project/other")
  ];
  const methods = [],
    checkpoints = [];
  const c = new Client((service, method, path) => {
    methods.push(method);
    if (path === `/repos/${REPOSITORY}`)
      return response({ full_name: REPOSITORY, permissions: { push: true } });
    if (service === "registry") return response(manifest);
    if (path.includes("/releases?")) return response([release]);
    if (path.includes("/pulls?")) return response([]);
    if (path.includes("/actions/runs?"))
      return response({ total_count: 0, workflow_runs: [] });
    if (path.startsWith("/orgs/radius-project/packages?"))
      return response(packages);
    if (path.includes("/versions?")) return response([raw(v)]);
    return response(
      packages.find((p) => decodeURIComponent(path).endsWith(p.name))
    );
  });
  const s = collect(c, (s) => checkpoints.push(structuredClone(s)));
  assert.deepEqual(
    s.packages.map((p) => p.name),
    ["dev/example", "unknown"]
  );
  assert.ok(s.complete);
  assert.equal(s.packages[0].versions.length, 1);
  assert.equal(s.packages[1].linked, false);
  assert.ok(methods.every((m) => m === "GET"));
  assert.equal(checkpoints[0].complete, false);
  assert.equal(checkpoints.at(-1).complete, true);
});
const environment = () => ({
  name: "package-cleanup",
  can_admins_bypass: false,
  protection_rules: [
    {
      type: "required_reviewers",
      prevent_self_review: true,
      reviewers: [{ type: "Team", id: 1 }]
    }
  ],
  deployment_branch_policy: {
    custom_branch_policies: true,
    protected_branches: false
  }
});
const branchPolicies = { branch_policies: [{ name: "main", type: "branch" }] };
test("human approval requires existing reviewers, no self-review/bypass, and exact main branch rule", () => {
  checkEnvironment(environment(), branchPolicies);
  for (const change of [
    (e) => {
      e.protection_rules = [];
    },
    (e) => {
      e.can_admins_bypass = true;
    },
    (e) => {
      e.protection_rules[0].prevent_self_review = false;
    },
    (e) => {
      e.deployment_branch_policy = null;
    }
  ]) {
    const e = environment();
    change(e);
    assert.throws(() => checkEnvironment(e, branchPolicies));
  }
  assert.throws(() =>
    checkEnvironment(environment(), {
      branch_policies: [{ name: "*", type: "branch" }]
    })
  );
});
test("publication gate refuses enabled publishers, active runs, moved main and missing external freeze", () => {
  function gate({
    enabled = false,
    busy = false,
    moved = false,
    freeze = true
  } = {}) {
    const c = new Client((service, method, path) => {
      if (path.includes("/deployment-branch-policies"))
        return response(branchPolicies);
      if (path.includes("/environments/")) return response(environment());
      if (path.includes("/actions/workflows"))
        return response({
          total_count: 1,
          workflows: [
            {
              path: ".github/workflows/build-release.yaml",
              state: enabled ? "active" : "disabled_manually"
            }
          ]
        });
      if (path.includes("/branches/main"))
        return response({
          commit: { sha: moved ? "c".repeat(40) : identity.sha }
        });
      if (path.includes("/actions/runs?"))
        return response({
          total_count: busy ? 1 : 0,
          workflow_runs: busy ? [{ id: 987 }] : []
        });
      throw new Error(`Unexpected ${path}`);
    });
    return verifyGate(
      c,
      identity,
      freeze ?
        {
          repository: REPOSITORY,
          sha: identity.sha,
          externalPublishersStopped: true,
          expiresAt: expires
        }
      : null,
      now
    );
  }
  gate();
  for (const opts of [
    { enabled: true },
    { busy: true },
    { moved: true },
    { freeze: false }
  ])
    assert.throws(() => gate(opts));
});
test("artifact source binds exact trusted workflow, main, event, source and successful attempt", () => {
  const run = {
    id: 123,
    repository: { full_name: REPOSITORY },
    head_repository: { full_name: REPOSITORY },
    head_branch: "main",
    head_sha: identity.sha,
    path: ".github/workflows/purge-old-images.yaml",
    event: "schedule",
    status: "completed",
    conclusion: "success",
    run_attempt: 1
  };
  verifyPlanRun(new Client(() => response(run)), 123, identity.sha);
  for (const change of [
    (r) => {
      r.event = "pull_request";
    },
    (r) => {
      r.head_branch = "feature";
    },
    (r) => {
      r.head_repository.full_name = "evil/fork";
    },
    (r) => {
      r.conclusion = "failure";
    },
    (r) => {
      r.path = "other.yaml";
    },
    (r) => {
      r.head_sha = "c".repeat(40);
    },
    (r) => {
      r.run_attempt = 0;
    }
  ]) {
    const r = structuredClone(run);
    change(r);
    assert.throws(() =>
      verifyPlanRun(new Client(() => response(r)), 123, identity.sha)
    );
  }
});
test("untrusted local/fork invocations fail before transport, with no deletion", () => {
  assert.throws(() => main({}), /trusted main/);
  assert.throws(
    () =>
      main({
        GITHUB_REPOSITORY: "fork/radius",
        GITHUB_REF: "refs/heads/main",
        GITHUB_EVENT_NAME: "workflow_dispatch",
        GITHUB_SHA: identity.sha
      }),
    /trusted main/
  );
});
test("workflow safety contracts preserve plan-only schedule, independent environment gate and bound artifacts", () => {
  const read = (name) =>
    readFileSync(new URL(`../workflows/${name}`, import.meta.url), "utf8");
  const wrapper = read("purge-old-images.yaml"),
    shared = read("__package-cleanup.yaml"),
    apply = read("package-cleanup-apply.yaml");
  assert.match(wrapper, /mode: plan/);
  assert.doesNotMatch(wrapper, /mode: apply|PURGE_DRY_RUN/);
  assert.match(apply, /PACKAGE_CLEANUP_ENABLE_APPLY == 'true'/);
  assert.match(shared, /name: package-cleanup/);
  assert.match(shared, /CLEANUP_MODE: preflight/);
  assert.match(shared, /needs: preflight/);
  assert.match(shared, /run-id: \$\{\{ inputs.plan_run_id \}\}/);
  assert.match(
    shared,
    /cleanup-plan-\$\{\{ needs.preflight.outputs.attempt \}\}/
  );
  assert.match(shared, /secrets.CLEANUP_PACKAGE_TOKEN/);
  assert.match(
    apply,
    /CLEANUP_PACKAGE_TOKEN: \$\{\{ secrets\.CLEANUP_PACKAGE_TOKEN \}\}/
  );
  assert.match(
    apply,
    /CLEANUP_PUBLICATION_FREEZE: \$\{\{ secrets\.CLEANUP_PUBLICATION_FREEZE \}\}/
  );
  assert.match(shared, /environment:\s+name: package-cleanup/);
  assert.match(
    shared,
    /CLEANUP_PUBLICATION_FREEZE: \$\{\{ secrets\.CLEANUP_PUBLICATION_FREEZE \}\}/
  );
  assert.match(shared, /retention-days: 30/);
  assert.match(shared, /ref: \$\{\{ github.sha \}\}/);
  assert.doesNotMatch(
    wrapper + shared + apply,
    /pull_request_target:|pull_request:|secrets: inherit/
  );
});
