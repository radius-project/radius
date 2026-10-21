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
    protectedClosureComplete: true,
    expiresAt: expires,
    review: `https://github.com/${REPOSITORY}/pull/1`,
    releases: [
      {
        id: 1,
        tag: release.tag_name,
        updatedAt: old,
        closureComplete: true,
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
    const custom = opts.respond?.({ service, method, path });
    if (custom) return custom;
    if (path.includes("/releases?")) return response([release]);
    if (path.includes("/pulls?")) return response([]);
    if (method === "DELETE") return codes.shift() ?? response("", 204);
    const v = s.packages
      .flatMap((p) => p.versions)
      .find((v) => path.endsWith(`/${v.id}`));
    return response(raw(v));
  }, opts.budget ?? 4000);
  const report = applyPlan(c, p, opts.fresh ?? s, {
    gate: () => Date.parse(expires),
    checkpoint: (r) => persisted.push(structuredClone(r)),
    ...opts
  });
  return { report, calls, persisted };
}
function diagnose404(
  s,
  {
    revalidation = false,
    initial = response({ message: "Not Found" }, 404, {
      requestId: "ABCD:404"
    }),
    followup = response({ message: "Not Found" }, 404),
    packageResponse = response({
      id: 1,
      name: "dev/example",
      package_type: "container",
      repository: { full_name: REPOSITORY, owner: { login: "radius-project" } }
    }),
    pages = [response([])],
    budget = 4000
  } = {}
) {
  let reads = 0;
  return apply(s, revalidation ? [] : [initial], {
    budget,
    respond({ method, path }) {
      if (method !== "GET") return;
      if (path.endsWith("/versions/1")) {
        reads++;
        if (revalidation && reads === 1) return initial;
        if (reads > 1) return followup;
      }
      if (path.endsWith("/container/dev%2Fexample")) return packageResponse;
      if (path.includes("/versions?")) {
        return pages[
          Number(
            new URL(`https://api.github.com${path}`).searchParams.get("page")
          ) - 1
        ];
      }
    }
  });
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
test("legacy Cosign tags protect their subjects, copies and platform descendants with exact digest parsing", () => {
  for (const suffix of ["sig", "att", "sbom"]) {
    const s = snapshot([
      version(1, ["pr-1"], [d(2)]),
      version(2),
      version(3, [`sha256-${d(1).slice(7)}.${suffix}`])
    ]);
    s.packages.push({
      ...s.packages[0],
      id: 2,
      name: "dev/copy",
      versions: [
        { ...version(10, ["pr-10"], [d(2)]), digest: d(1) },
        { ...version(11), digest: d(2) }
      ]
    });
    const rows = classify(s, evidence(), options);
    assert.ok(rows.every((r) => r.decision === "keep"));
  }
  for (const tag of [
    `sha256-${d(1).slice(7)}.sig.extra`,
    `sha256-${"a".repeat(63)}.sig`,
    `prefix-sha256-${d(1).slice(7)}.att`,
    `sha256-${d(1).slice(7)}.SIG`,
    `sha256-${"G".repeat(64)}.sbom`
  ]) {
    const rows = classify(
      snapshot([version(1), version(2, [tag])]),
      evidence(),
      options
    );
    assert.equal(rows[0].decision, "delete-candidate");
    assert.notEqual(rows[1].decision, "delete-candidate");
  }
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
test("external release closure needs explicit review and protects copied platform manifests", () => {
  const s = snapshot([version(2), version(3)]);
  const e = evidence();
  e.releases[0].digests = [d(90)];
  delete e.releases[0].closureComplete;
  assert.ok(classify(s, e, options).every((r) => r.decision === "hold"));
  // The external index cannot be traversed by the linked-only collector.
  // Its reviewer must supply its complete closure, including child d(2).
  e.releases[0].closureComplete = true;
  e.releases[0].digests.push(d(2));
  assert.deepEqual(
    classify(s, e, options).map((r) => r.decision),
    ["keep", "delete-candidate"]
  );
  e.protectedClosureComplete = false;
  assert.equal(classify(s, e, options)[1].decision, "hold");
});
test("known release edges and referrers mechanically check the reviewed closure", () => {
  const s = snapshot([
    version(1),
    version(90, ["0.1.0"], [d(2)]),
    version(2),
    { ...version(4, []), artifact: true, subject: d(90), children: [d(90)] }
  ]);
  const e = evidence();
  e.releases[0].digests = [d(90)];
  assert.equal(classify(s, e, options)[0].decision, "hold");
  e.releases[0].digests.push(d(2));
  assert.equal(classify(s, e, options)[0].decision, "hold");
  e.releases[0].digests.push(d(4));
  assert.equal(classify(s, e, options)[0].decision, "delete-candidate");
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
test("DELETE and revalidation 404 followed by GET 200 save evidence and stop all deletes", () => {
  for (const revalidation of [false, true]) {
    const { report, calls, persisted } = diagnose404(
      snapshot([version(1, ["pr-1"], [d(2)]), version(2), version(3)]),
      {
        revalidation,
        followup: response(raw(version(1)), 200, { requestId: "ABCD:200" })
      }
    );
    assert.equal(report.status, "failed");
    assert.match(
      report.error,
      /404 contradicts.*GET HTTP 200.*cause unconfirmed/
    );
    assert.ok(report.results.every((r) => r.status === "unfinished"));
    assert.equal(
      calls.filter((c) => c.method === "DELETE").length,
      revalidation ? 0 : 1
    );
    assert.deepEqual(
      report.results[0].requests.map((r) => r.status),
      revalidation ? [404, 200] : [200, 404, 200]
    );
    const failure = report.results[0].requests.find((r) => r.status === 404);
    assert.equal(failure.body, '{"message":"Not Found"}');
    assert.equal(failure.requestId, "ABCD:404");
    assert.equal(
      failure.path,
      "/orgs/radius-project/packages/container/dev%2Fexample/versions/1"
    );
    assert.equal(failure.method, revalidation ? "GET" : "DELETE");
    assert.ok(
      persisted.some((p) => p.results[0].requests?.at(-1).status === 404)
    );
  }
});
test("404 needs successful same-credential package and fully paginated version reads to establish absence", () => {
  for (const revalidation of [false, true]) {
    const { report, calls } = diagnose404(
      snapshot([version(1, ["pr-1"], [d(2)]), version(2), version(3)]),
      {
        revalidation,
        pages: [
          response(
            Array.from({ length: 100 }, (_, i) => raw(version(i + 1000)))
          ),
          response([])
        ]
      }
    );
    assert.deepEqual(
      report.results.map((r) => r.status),
      ["already-absent", "unfinished", "deleted"]
    );
    assert.equal(report.status, "partial");
    assert.match(report.results[0].reasons[0], /corroborated.*same-credential/);
    assert.equal(report.results[0].absenceCheck.versionCount, 100);
    assert.equal(calls.filter((c) => c.path.includes("/versions?")).length, 2);
    assert.ok(calls.every((c) => c.service === "github"));
    assert.ok(
      report.results[0].requests.some(
        (r) => r.path.endsWith("page=2") && r.status === 200
      )
    );
    assert.equal(
      calls.filter((c) => c.method === "DELETE").length,
      revalidation ? 1 : 2
    );
  }
});
test("404 with inaccessible package/list or a still-listed version fails closed without inferring permissions", () => {
  for (const revalidation of [false, true]) {
    for (const diagnostic of [
      { packageResponse: response({ message: "Not Found" }, 404) },
      { packageResponse: response({ message: "Forbidden" }, 403) },
      { packageResponse: response({ message: "Bad credentials" }, 401) },
      { packageResponse: response({ id: 2, name: "wrong" }) },
      { pages: [response({ message: "Not Found" }, 404)] },
      { pages: [response([raw(version(1))])] },
      {
        pages: [
          response(
            Array.from({ length: 100 }, (_, i) => raw(version(i + 1000)))
          ),
          response([raw(version(1))])
        ]
      },
      { followup: response({ message: "Forbidden" }, 403) }
    ]) {
      const { report, calls } = diagnose404(
        snapshot([version(1), version(2)]),
        { revalidation, ...diagnostic }
      );
      assert.equal(report.status, "failed");
      assert.equal(report.results[0].status, "unfinished");
      assert.equal(report.counts["already-absent"], undefined);
      assert.equal(
        calls.filter((c) => c.method === "DELETE").length,
        revalidation ? 0 : 1
      );
      assert.match(report.results[0].reasons[0], /absence unconfirmed/);
      assert.ok(report.results[0].requests.length >= 2);
    }
  }
});
test("404 malformed diagnostic bodies and version records are saved and never prove absence", () => {
  for (const diagnostic of [
    { packageResponse: response("not-json") },
    { packageResponse: response("null") },
    { pages: [response("not-json")] },
    { pages: [response({ message: "not a list" })] },
    { pages: [response([{ id: 2 }])] },
    { followup: response("not-json", 200) }
  ]) {
    const { report } = diagnose404(
      snapshot([version(1), version(2)]),
      diagnostic
    );
    assert.equal(report.status, "failed");
    assert.equal(report.results[0].status, "unfinished");
    assert.ok(
      report.results[0].requests.some(
        (r) =>
          r.body === "not-json" ||
          r.body === "null" ||
          r.body.includes("not a list") ||
          r.body === '[{"id":2}]'
      )
    );
  }
});
test("404 evidence bounds/redacts bodies, captures request IDs, and excludes credential headers", () => {
  const previous = process.env.GH_TOKEN;
  const secret = "offline-secret-credential";
  process.env.GH_TOKEN = secret;
  try {
    const { report, persisted } = diagnose404(snapshot(), {
      initial: response(
        `${secret} ghp_offlineFixtureToken Bearer other-fixture-token ${"x".repeat(5000)}`,
        404,
        {
          requestId: "ABCD:404",
          authorization: `Bearer ${secret}`
        }
      )
    });
    assert.equal(report.status, "complete");
    assert.equal(report.counts.deleted, undefined);
    const captured = report.results[0].requests.find((r) => r.status === 404);
    assert.equal(captured.body.length, 4096);
    assert.equal(captured.bodyTruncated, true);
    assert.equal(captured.requestId, "ABCD:404");
    assert.ok(Number.isFinite(Date.parse(captured.at)));
    assert.equal(captured.headers, undefined);
    for (const value of [
      secret,
      "ghp_offlineFixtureToken",
      "other-fixture-token"
    ]) {
      assert.ok(!JSON.stringify(persisted).includes(value));
    }
    const malformed = diagnose404(snapshot(), {
      packageResponse: response(secret, 200, {
        requestId: `ABCD:404\r\nAuthorization: Bearer ${secret}`
      })
    });
    assert.equal(malformed.report.status, "failed");
    assert.ok(!JSON.stringify(malformed.persisted).includes(secret));
    assert.match(malformed.report.error, /^Invalid JSON response: \/orgs\//);
    assert.ok(!malformed.report.error.includes(secret.slice(0, 10)));
    assert.equal(
      malformed.report.results[0].requests.at(-1).requestId,
      undefined
    );
  } finally {
    if (previous === undefined) delete process.env.GH_TOKEN;
    else process.env.GH_TOKEN = previous;
  }
});
test("404 diagnostic rate and budget limits stay partial with saved response evidence", () => {
  for (const revalidation of [false, true]) {
    for (const diagnostic of [
      {
        followup: response({ message: "Rate limited" }, 429, {
          requestId: "ABCD:429"
        })
      },
      {
        packageResponse: response({ message: "Rate limited" }, 403, {
          remaining: "0"
        })
      },
      {
        pages: [
          response({ message: "Secondary limit" }, 403, { retryAfter: "300" })
        ]
      },
      { budget: revalidation ? 3 : 4 }
    ]) {
      const { report, calls, persisted } = diagnose404(
        snapshot([version(1), version(2)]),
        { revalidation, ...diagnostic }
      );
      assert.equal(report.status, "partial");
      assert.equal(report.results[0].status, "unfinished");
      assert.match(report.error, /Rate limit|API request budget/);
      assert.equal(
        calls.filter((c) => c.method === "DELETE").length,
        revalidation ? 0 : 1
      );
      assert.ok(
        persisted.at(-1).results[0].requests.some((r) => r.status === 404)
      );
      if (!diagnostic.budget)
        assert.ok(
          report.results[0].requests.some(
            (r) => r.status === 429 || r.status === 403
          )
        );
    }
  }
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
  for (const budget of [0, -1, 1.5, 20001]) {
    assert.throws(() => new Client(() => response([]), budget), /API budget/);
  }
  assert.equal(new Client(() => response([]), 20000).budget, 20000);
});
test("a freeze expiring during revalidation stops before DELETE and preserves unfinished evidence", () => {
  for (const advanceAt of ["/releases?", "/pulls?", "/versions/1"]) {
    let time = now;
    const deadline = now + 100;
    const { report, calls, persisted } = apply(snapshot(), [], {
      clock: () => time,
      gate: () => deadline,
      respond({ path }) {
        if (path.includes(advanceAt)) time = deadline;
      }
    });
    assert.equal(report.status, "partial");
    assert.match(report.error, /freeze lease expired/);
    assert.equal(calls.filter((c) => c.method === "DELETE").length, 0);
    assert.equal(persisted.at(-1).results[0].status, "unfinished");
  }
});
test("plan expiry or future time is enforced after revalidation and on later DELETE candidates", () => {
  for (const future of [false, true]) {
    for (const later of [false, true]) {
      for (const stage of ["version", "gate"]) {
        let time = now;
        let gates = 0;
        const s = snapshot([version(1), version(2)]),
          p = planFor(s),
          calls = [],
          saved = [];
        p.createdAt = new Date(now - 6 * 3600000 + 100).toISOString();
        validatePlan(p, hash(p), identity, evidence(), options, now);
        const client = new Client((service, method, path) => {
          calls.push(method);
          if (path.includes("/releases?")) return response([release]);
          if (path.includes("/pulls?")) return response([]);
          if (method === "DELETE") return response("", 204);
          if (stage === "version" && path.endsWith(later ? "/2" : "/1")) {
            time = future ? Date.parse(p.createdAt) - 1 : now + 101;
          }
          return response(
            raw(s.packages[0].versions.find((v) => path.endsWith(`/${v.id}`)))
          );
        });
        const report = applyPlan(client, p, s, {
          clock: () => time,
          gate: () => {
            gates++;
            if (stage === "gate" && gates === (later ? 2 : 1)) {
              time = future ? Date.parse(p.createdAt) - 1 : now + 101;
            }
            return now + 3600000;
          },
          checkpoint: (r) => saved.push(structuredClone(r))
        });
        assert.equal(report.status, "partial");
        assert.match(report.error, /Plan expired or from the future/);
        assert.equal(calls.filter((m) => m === "DELETE").length, later ? 1 : 0);
        assert.equal(report.counts.deleted ?? 0, later ? 1 : 0);
        assert.equal(saved.at(-1).results.at(-1).status, "unfinished");
      }
    }
  }
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
    const report = applyPlan(client, p, s, { gate: () => Date.parse(expires) });
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
    visibility: "public",
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
  const privateData = {
    body: "CANARY_DRAFT_BODY",
    name: "CANARY_TITLE",
    assets: [{ name: "CANARY_ASSET", url: "CANARY_ASSET_URL" }],
    author: { login: "CANARY_AUTHOR" }
  };
  const running = {
    id: 77,
    path: ".github/workflows/purge-old-images.yaml",
    status: "in_progress",
    head_commit: { message: "CANARY_COMMIT" },
    actor: privateData.author
  };
  const c = new Client((service, method, path) => {
    methods.push(method);
    if (path === `/repos/${REPOSITORY}`)
      return response({ full_name: REPOSITORY, permissions: { push: true } });
    if (service === "registry") return response(manifest);
    if (path.includes("/releases?"))
      return response([{ ...release, ...privateData, draft: true }]);
    if (path.includes("/pulls?"))
      return response([{ id: 555, number: 555, ...privateData }]);
    if (path.includes("/actions/runs?"))
      return response({ total_count: 1, workflow_runs: [running] });
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
  assert.deepEqual(s.releases, [release]);
  assert.deepEqual(s.openPRs, [{ number: 555 }]);
  assert.deepEqual(s.activeRuns, [{ id: 77, path: running.path }]);
  const plan = planFor(s);
  assert.ok(!JSON.stringify({ checkpoints, plan }).includes("CANARY"));
  for (let attempt = 0; attempt < 2; attempt++) {
    const partial = [];
    assert.throws(
      () =>
        collect(new Client(c.transport, 1), (s) =>
          partial.push(structuredClone(s))
        ),
      Incomplete
    );
    assert.equal(partial.at(-1).complete, false);
    assert.throws(
      () => classify(partial.at(-1), evidence(), options),
      /Incomplete inventory/
    );
  }
  assert.equal(collect(new Client(c.transport, 20000)).complete, true);
  let time = now;
  const nearlyExpired = makePlan(s, evidence(), identity, {
    ...options,
    now: now - 6 * 3600000 + 100
  });
  validatePlan(
    nearlyExpired,
    hash(nearlyExpired),
    identity,
    evidence(),
    options,
    time
  );
  const ticking = new Client((...args) => {
    time += 100;
    return c.transport(...args);
  });
  const fresh = collect(ticking);
  const saved = [];
  const expired = applyPlan(ticking, nearlyExpired, fresh, {
    clock: () => time,
    gate: () => Date.parse(expires),
    checkpoint: (r) => saved.push(structuredClone(r))
  });
  assert.equal(expired.status, "partial");
  assert.match(expired.error, /Plan expired/);
  assert.equal(saved.at(-1).counts.unfinished, 1);
  assert.ok(methods.every((m) => m === "GET"));
});
test("non-public and unknown package visibility never reach public checkpoints, plans or errors", () => {
  for (const visibility of [
    "private",
    "internal",
    undefined,
    "changed",
    "unavailable",
    "catalog-error"
  ]) {
    const checkpoints = [],
      calls = [];
    const summary = {
      id: 1,
      name: "CANARY_PACKAGE",
      package_type: "container",
      visibility:
        ["changed", "unavailable"].includes(visibility) ? "public" : visibility,
      metadata: { tags: ["CANARY_PRIVATE_TAG"] }
    };
    const client = new Client((service, method, path) => {
      calls.push(path);
      if (path === `/repos/${REPOSITORY}`)
        return response({ full_name: REPOSITORY, permissions: { push: true } });
      if (path.includes("/releases?"))
        return response([
          {
            ...release,
            draft: true,
            body: "CANARY_BODY",
            assets: ["CANARY_ASSET"]
          }
        ]);
      if (path.includes("/pulls?")) return response([]);
      if (path.includes("/actions/runs?"))
        return response({ total_count: 0, workflow_runs: [] });
      if (path.startsWith("/orgs/radius-project/packages?"))
        return visibility === "catalog-error" ?
            response("CANARY_CATALOG_BODY", 403)
          : response([summary]);
      if (visibility === "unavailable")
        return response("CANARY_RESPONSE_BODY", 403);
      return response({ ...summary, visibility: "private" });
    });
    let plan, error;
    try {
      const s = collect(client, (s) => checkpoints.push(structuredClone(s)));
      plan = planFor(s);
    } catch (e) {
      error = {
        error: e.message,
        status: e instanceof Incomplete ? "partial" : "failed"
      };
    }
    assert.equal(plan, undefined);
    assert.ok(error);
    assert.equal(checkpoints.at(-1).complete, false);
    assert.ok(!JSON.stringify({ checkpoints, plan, error }).includes("CANARY"));
    if (!["changed", "unavailable", "catalog-error"].includes(visibility)) {
      assert.ok(calls.every((path) => !path.includes("/container/")));
      assert.equal(checkpoints.at(-1).withheldPackages, 1);
    }
    assert.throws(() => planFor(checkpoints.at(-1)), /Incomplete inventory/);
  }
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
    freeze = true,
    clock = () => now,
    advance = () => {}
  } = {}) {
    const c = new Client((service, method, path) => {
      advance(path);
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
      clock
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
  for (const stage of ["/environments/", "status=requested"]) {
    let time = now;
    const { report, calls, persisted } = apply(snapshot(), [], {
      clock: () => time,
      gate: () =>
        gate({
          clock: () => time,
          advance(path) {
            if (path.includes(stage)) time = Date.parse(expires);
          }
        })
    });
    assert.equal(report.status, "partial");
    assert.match(report.error, /freeze lease expired/);
    assert.equal(calls.filter((c) => c.method === "DELETE").length, 0);
    assert.equal(persisted.at(-1).counts.unfinished, 1);
  }
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
  for (const caller of [wrapper, apply]) {
    assert.match(caller, /api_budget: \$\{\{ inputs.api_budget \}\}/);
    assert.match(
      caller,
      /api_budget:\s+description: [^\r\n]+\s+type: number\s+default: 4000/
    );
  }
  assert.match(shared, /api_budget:\s+type: number\s+default: 4000/);
  assert.match(shared, /CLEANUP_API_BUDGET: \$\{\{ inputs.api_budget \}\}/);
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
