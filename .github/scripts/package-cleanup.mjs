// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.
// SPDX-License-Identifier: Apache-2.0

import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync, mkdirSync, readdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const REPOSITORY = "radius-project/radius";
export const PROTECTED_MESSAGE =
  "Publicly visible package versions with more than 5000 downloads cannot be deleted. Contact GitHub support for further assistance.";
const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const EVIDENCE = ".github/package-cleanup-evidence.json";
const SHA = /^[a-f0-9]{40}$/;
const DIGEST = /^sha256:[a-f0-9]{64}$/;
const PACKAGE = /^[a-z0-9]+(?:[._/-][a-z0-9]+)*$/;
const RELEASE = /^v?\d+\.\d+\.\d+(?:-rc\.?\d+)?$/;
const CHANNEL = /^(?:v?\d+\.\d+|latest|edge|stable|next|canary|nightly)$/;
const PROFILES = ["pr", "functional", "test-recipes", "reviewed-tests"];
const MAX_AGE = 6 * 60 * 60 * 1000;
const ALLOWED_WORKFLOWS = new Set([
  ".github/workflows/purge-old-images.yaml",
  ".github/workflows/package-cleanup-apply.yaml",
  ".github/workflows/__package-cleanup.yaml"
]);
const json = (path) => JSON.parse(readFileSync(path, "utf8"));
export const hash = (value) =>
  createHash("sha256")
    .update(typeof value === "string" ? value : canonical(value))
    .digest("hex");
export function canonical(value) {
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (value && typeof value === "object")
    return `{${Object.keys(value)
      .filter((k) => value[k] !== undefined)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${canonical(value[k])}`)
      .join(",")}}`;
  return JSON.stringify(value);
}
function requireThat(condition, message) {
  if (!condition) throw new Error(message);
}
function positive(value) {
  return Number.isSafeInteger(value) && value > 0;
}
function date(value) {
  requireThat(
    typeof value === "string" && Number.isFinite(Date.parse(value)),
    "Invalid timestamp"
  );
  return Date.parse(value);
}
function packageName(value) {
  requireThat(
    typeof value === "string" &&
      value.length < 256 &&
      PACKAGE.test(value) &&
      !value.includes(".."),
    "Invalid package name"
  );
  return value;
}
function list(value, name) {
  requireThat(Array.isArray(value), `Invalid ${name} list`);
  return value;
}
function save(path, value) {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`);
}
export function counts(records, field = "decision") {
  const result = {};
  for (const r of records) result[r[field]] = (result[r[field]] ?? 0) + 1;
  return result;
}
function versionPath(name) {
  return `/orgs/radius-project/packages/container/${encodeURIComponent(packageName(name))}/versions`;
}
export class Incomplete extends Error {}

// The existing curl helper owns retry delays and deadlines. Count each logical
// request here; retries have a separate fixed ceiling in the transport.
export class Client {
  constructor(transport, budget = 4000) {
    requireThat(
      positive(budget) && budget <= 20000,
      "API budget must be 1..20000"
    );
    this.transport = transport;
    this.budget = budget;
    this.calls = 0;
  }
  request(service, method, path) {
    if (++this.calls > this.budget)
      throw new Incomplete("API request budget reached");
    const response = this.transport(service, method, path);
    requireThat(
      response &&
        Number.isInteger(response.status) &&
        typeof response.body === "string",
      "Invalid HTTP response"
    );
    if (
      response.status === 429 ||
      (response.status === 403 &&
        (response.headers?.remaining === "0" || response.headers?.retryAfter))
    ) {
      throw new Incomplete(`Rate limit after bounded retries: ${path}`);
    }
    return response;
  }
  get(path, service = "github") {
    const r = this.request(service, "GET", path);
    requireThat(
      r.status === 200,
      `GET ${path}: HTTP ${r.status}: ${r.body.slice(0, 500)}`
    );
    return JSON.parse(r.body);
  }
  pages(path) {
    const all = [],
      seen = new Set();
    for (let page = 1; page <= 1000; page++) {
      const rows = list(
        this.get(
          `${path}${path.includes("?") ? "&" : "?"}per_page=100&page=${page}`
        ),
        path
      );
      requireThat(rows.length <= 100, "Oversized API page");
      for (const row of rows) {
        requireThat(
          positive(row.id) && !seen.has(row.id),
          "Missing or repeated API identity"
        );
        seen.add(row.id);
        all.push(row);
      }
      if (rows.length < 100) return all;
    }
    throw new Incomplete("Pagination limit reached");
  }
}
function transport(service, method, path) {
  const result = spawnSync(
    "bash",
    [
      resolve(ROOT, ".github/scripts/purge-old-images.sh"),
      "--request",
      service,
      method,
      path
    ],
    {
      encoding: "utf8",
      maxBuffer: 32 * 1024 * 1024,
      env: {
        ...process.env,
        RETRY_CURL_MAX_SECONDS: "60",
        RETRY_CURL_MAX_ATTEMPTS: "3"
      }
    }
  );
  if (result.stderr) process.stderr.write(result.stderr);
  requireThat(
    result.status === 0,
    `HTTP transport failed: ${result.error?.message ?? result.status}`
  );
  return JSON.parse(result.stdout);
}
export function normalizeVersion(raw) {
  requireThat(
    positive(raw.id) && DIGEST.test(raw.name),
    "Invalid version identity/digest"
  );
  const tags = list(raw.metadata?.container?.tags, "tags");
  requireThat(
    tags.every(
      (t) => typeof t === "string" && t.length > 0 && t.length <= 128
    ) && new Set(tags).size === tags.length,
    "Invalid tags"
  );
  date(raw.created_at);
  date(raw.updated_at);
  return {
    id: raw.id,
    digest: raw.name,
    tags: [...tags].sort(),
    createdAt: raw.created_at,
    updatedAt: raw.updated_at
  };
}
export function manifestEdges(body, digest) {
  requireThat(
    `sha256:${hash(body)}` === digest,
    "Registry manifest digest mismatch"
  );
  const m = JSON.parse(body);
  const recipeConfig = "application/vnd.ms.bicep.module.config.v1+json";
  const recipeLayer = "application/vnd.ms.bicep.module.layer.v1+json";
  const allowed = [
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.oci.image.manifest.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
    "application/vnd.docker.distribution.manifest.v2+json"
  ];
  requireThat(
    m.schemaVersion === 2 &&
      (allowed.includes(m.mediaType) ||
        (m.mediaType === undefined && m.config?.mediaType === recipeConfig)),
    "Unknown manifest format"
  );
  const index =
    m.mediaType?.includes("index") || m.mediaType?.includes("manifest.list");
  const children =
    index ? list(m.manifests, "manifests").map((c) => c.digest) : [];
  requireThat(
    children.every((d) => DIGEST.test(d)),
    "Invalid child digest"
  );
  if (m.artifactType || m.subject) {
    requireThat(
      !m.subject || DIGEST.test(m.subject.digest),
      "Invalid artifact subject"
    );
    return {
      children: [
        ...new Set([...children, ...(m.subject ? [m.subject.digest] : [])])
      ].sort(),
      artifact: true,
      subject: m.subject?.digest
    };
  }
  if (!index)
    requireThat(
      DIGEST.test(m.config?.digest) &&
        Array.isArray(m.layers) &&
        m.layers.every((l) => DIGEST.test(l.digest)),
      "Incomplete image manifest"
    );
  const imageConfigs = [
    "application/vnd.oci.image.config.v1+json",
    "application/vnd.docker.container.image.v1+json"
  ];
  const recipe =
    !index &&
    m.config.mediaType === recipeConfig &&
    m.layers.length === 1 &&
    m.layers[0].mediaType === recipeLayer;
  const artifact =
    !index &&
    !recipe &&
    (!imageConfigs.includes(m.config.mediaType) ||
      m.layers.some(
        (l) =>
          !/^application\/vnd\.(?:oci\.image\.layer\.|docker\.image\.rootfs\.)/.test(
            l.mediaType ?? ""
          )
      ));
  return { children: [...new Set(children)].sort(), artifact, recipe };
}

export function collect(client, checkpoint = () => {}) {
  const snapshot = {
    repository: REPOSITORY,
    packages: [],
    releases: [],
    openPRs: [],
    activeRuns: [],
    complete: false
  };
  checkpoint(snapshot);
  const repository = client.get(`/repos/${REPOSITORY}`);
  requireThat(
    repository.full_name === REPOSITORY &&
      repository.permissions?.push === true,
    "Inventory needs repository push visibility to include draft releases"
  );
  // All release entries, including drafts and prereleases, are evidence. Never
  // infer history completeness from a supported-version list or an asset name.
  snapshot.releases = client.pages(`/repos/${REPOSITORY}/releases`);
  snapshot.openPRs = client.pages(`/repos/${REPOSITORY}/pulls?state=open`);
  requireThat(
    snapshot.releases.every(
      (r) =>
        typeof r.tag_name === "string" &&
        r.tag_name.length > 0 &&
        typeof r.updated_at === "string" &&
        Number.isFinite(Date.parse(r.updated_at))
    ) && snapshot.openPRs.every((pr) => positive(pr.number)),
    "Invalid release or pull request metadata"
  );
  const runs = client.get(`/repos/${REPOSITORY}/actions/runs?per_page=100`);
  requireThat(
    positive(runs.total_count) || runs.total_count === 0,
    "Invalid workflow run count"
  );
  snapshot.activeRuns = list(runs.workflow_runs, "workflow runs").filter(
    (r) => r.status !== "completed"
  );
  // Old queued runs can be outside the first page. Query each active status
  // separately and hold if GitHub's 1000-result search limit is reached.
  for (const status of [
    "queued",
    "in_progress",
    "waiting",
    "pending",
    "requested"
  ]) {
    for (let page = 1; ; page++) {
      const r = client.get(
        `/repos/${REPOSITORY}/actions/runs?status=${status}&per_page=100&page=${page}`
      );
      requireThat(
        Number.isSafeInteger(r.total_count) && r.total_count < 1000,
        "Incomplete active workflow search"
      );
      const rows = list(r.workflow_runs, "active workflows");
      snapshot.activeRuns.push(...rows);
      if (rows.length < 100) break;
      requireThat(page < 10, "Incomplete active workflow search");
    }
  }
  snapshot.activeRuns = [
    ...new Map(snapshot.activeRuns.map((r) => [r.id, r])).values()
  ];
  checkpoint(snapshot);
  for (const summary of client.pages(
    "/orgs/radius-project/packages?package_type=container"
  )) {
    packageName(summary.name);
    const p = client.get(
      `/orgs/radius-project/packages/container/${encodeURIComponent(summary.name)}`
    );
    requireThat(
      p.name === summary.name &&
        p.id === summary.id &&
        p.package_type === "container",
      "Package identity changed"
    );
    if (p.repository?.full_name && p.repository.full_name !== REPOSITORY)
      continue;
    const linked =
      p.repository?.full_name === REPOSITORY &&
      p.repository?.owner?.login === "radius-project";
    const entry = {
      id: p.id,
      name: p.name,
      repository: p.repository?.full_name ?? null,
      linked,
      versions: []
    };
    snapshot.packages.push(entry);
    if (!linked) {
      entry.hold = "Unknown or unlinked repository";
      checkpoint(snapshot);
      continue;
    }
    for (const raw of client.pages(versionPath(p.name))) {
      const v = normalizeVersion(raw);
      entry.versions.push(v);
      const r = client.request(
        "registry",
        "GET",
        `/v2/radius-project/${p.name}/manifests/${v.digest}`
      );
      if (r.status !== 200) {
        v.graphError = `Manifest unavailable: HTTP ${r.status}`;
      } else {
        try {
          Object.assign(v, manifestEdges(r.body, v.digest));
        } catch (error) {
          v.graphError = error.message;
        }
      }
      checkpoint(snapshot);
    }
    checkpoint(snapshot);
  }
  snapshot.complete = true;
  checkpoint(snapshot);
  return snapshot;
}

function evidenceState(snapshot, evidence, now) {
  requireThat(
    evidence.schema === 1 && evidence.repository === REPOSITORY,
    "Wrong evidence repository/schema"
  );
  const roots = new Set(list(evidence.protectedDigests, "protected digests"));
  requireThat(
    [...roots].every((d) => DIGEST.test(d)),
    "Invalid evidence digest"
  );
  const releases = list(evidence.releases, "release evidence");
  const releaseIds = new Set();
  for (const r of releases) {
    requireThat(
      positive(r.id) && typeof r.tag === "string" && !releaseIds.has(r.id),
      "Invalid release evidence identity"
    );
    releaseIds.add(r.id);
    requireThat(
      typeof r.source === "string" &&
        r.source.startsWith(`https://github.com/${REPOSITORY}/`),
      "Release evidence needs a repository source"
    );
    requireThat(
      Array.isArray(r.digests) &&
        r.digests.length > 0 &&
        r.digests.every((d) => DIGEST.test(d)),
      "Release evidence needs verified roots"
    );
    r.digests.forEach((d) => roots.add(d));
  }
  const covered =
    snapshot.releases.length > 0 &&
    snapshot.releases.every((r) =>
      releases.some(
        (e) =>
          e.id === r.id && e.tag === r.tag_name && e.updatedAt === r.updated_at
      )
    );
  const fresh =
    date(evidence.expiresAt) > now &&
    date(evidence.expiresAt) <= now + 7 * 86400000;
  const reviewed =
    typeof evidence.review === "string" &&
    /^https:\/\/github\.com\/radius-project\/radius\/pull\/[1-9]\d*$/.test(
      evidence.review
    );
  const activeTags = new Set(list(evidence.activeTags, "active tags"));
  requireThat(
    [...activeTags].every((t) => typeof t === "string" && t.length > 0),
    "Invalid active tag"
  );
  const recipePackages = list(
    evidence.testRecipePackages,
    "test recipe packages"
  ).map(packageName);
  requireThat(
    recipePackages.every((p) =>
      /^(?:dev\/|radius\/dev\/|test\/testrecipes\/test-bicep-recipes\/)/.test(p)
    ),
    "Test recipe package must be in a test namespace"
  );
  return {
    roots,
    activeTags,
    recipePackages,
    complete:
      covered &&
      fresh &&
      reviewed &&
      evidence.historicalReviewComplete === true &&
      evidence.activeReferenceReviewComplete === true
  };
}
function selectedTag(tag, profile, name, state) {
  const pr =
    /^pr-[1-9]\d*(?:-[a-f0-9]{7,40})?$/.test(tag) ||
    /^0\.42\.42-pr-[1-9]\d*$/.test(tag);
  // These forms come from functional-test-cloud.yaml and cli_test.go.
  const functional =
    /^pr-func[a-f0-9]{10}$/.test(tag) ||
    /^test-func[a-f0-9]{10}-[1-9]\d*-[1-9]\d*$/.test(tag);
  const recipe =
    state.recipePackages.includes(name) &&
    (pr || functional || /^test-[1-9]\d{9}-(?:0|[1-9]\d{0,2})$/.test(tag));
  return (
    (["pr", "reviewed-tests"].includes(profile) && pr) ||
    (["functional", "reviewed-tests"].includes(profile) && functional) ||
    (["test-recipes", "reviewed-tests"].includes(profile) && recipe)
  );
}
export function classify(
  snapshot,
  evidence,
  { profile = "pr", retentionDays = 7, now = Date.now() } = {}
) {
  requireThat(
    snapshot.repository === REPOSITORY && snapshot.complete === true,
    "Incomplete inventory"
  );
  requireThat(PROFILES.includes(profile), "Unknown cleanup profile");
  requireThat(
    positive(retentionDays) && retentionDays >= 7 && retentionDays <= 3650,
    "Retention must be 7..3650 days"
  );
  const state = evidenceState(snapshot, evidence, now);
  const records = [];
  const allGraphsComplete = snapshot.packages
    .filter((p) => p.linked)
    .every((p) => {
      const digests = new Set(p.versions.map((v) => v.digest));
      return p.versions.every(
        (v) =>
          !v.graphError &&
          Array.isArray(v.children) &&
          v.children.every((d) => digests.has(d)) &&
          (!v.subject || digests.has(v.subject))
      );
    });
  const releaseTags = new Set(
    snapshot.releases.flatMap((r) => [
      r.tag_name,
      r.tag_name?.replace(/^v/, "")
    ])
  );
  const globalProtected = new Set(state.roots);
  for (const p of snapshot.packages)
    for (const v of p.versions) {
      if (
        v.artifact ||
        v.tags.some(
          (t) =>
            RELEASE.test(t) ||
            CHANNEL.test(t) ||
            releaseTags.has(t) ||
            /(?:^sha256-|\.sig$|\.att$|\.sbom$)/.test(t)
        ) ||
        /^(?:rad\/|helm-chart\/|devcontainer-features\/)/.test(p.name)
      )
        globalProtected.add(v.digest);
    }
  let rootsChanged;
  do {
    rootsChanged = false;
    for (const p of snapshot.packages)
      for (const v of p.versions) {
        if (!globalProtected.has(v.digest)) continue;
        for (const d of v.children ?? []) {
          if (!globalProtected.has(d)) {
            globalProtected.add(d);
            rootsChanged = true;
          }
        }
      }
  } while (rootsChanged);
  for (const p of snapshot.packages) {
    if (!p.linked) {
      records.push({
        package: p.name,
        packageId: p.id,
        decision: "hold",
        reasons: [p.hold ?? "Repository link not proven"]
      });
      continue;
    }
    const byDigest = new Map(p.versions.map((v) => [v.digest, v]));
    requireThat(
      byDigest.size === p.versions.length,
      "Duplicate version digest"
    );
    const graphComplete = p.versions.every(
      (v) =>
        !v.graphError &&
        Array.isArray(v.children) &&
        v.children.every((d) => byDigest.has(d)) &&
        (!v.subject || byDigest.has(v.subject))
    );
    for (const v of p.versions) {
      const r = {
        package: p.name,
        packageId: p.id,
        ...v,
        decision: "hold",
        reasons: []
      };
      if (globalProtected.has(v.digest)) {
        r.decision = "keep";
        r.reasons.push("Release, alias, artifact, or released-content copy");
      } else if (
        !/^(?:dev|radius\/dev)\//.test(p.name) &&
        !(
          state.recipePackages.includes(p.name) &&
          ["test-recipes", "reviewed-tests"].includes(profile)
        )
      ) {
        r.reasons.push("Namespace not approved for test cleanup");
      } else if (v.recipe && !state.recipePackages.includes(p.name)) {
        r.reasons.push(
          "Bicep recipe package needs explicit reviewed test-path evidence"
        );
      } else if (!state.complete) {
        r.reasons.push(
          "Historical release or active-reference evidence incomplete/expired"
        );
      } else if (!graphComplete || !allGraphsComplete) {
        r.reasons.push(
          "Incomplete dependency graph; cross-namespace release copies cannot be excluded"
        );
      } else if (
        date(v.updatedAt) > now - retentionDays * 86400000 ||
        date(v.createdAt) > now - retentionDays * 86400000
      ) {
        r.decision = "keep";
        r.reasons.push("Within retention window");
      } else if (
        v.tags.some(
          (t) =>
            state.activeTags.has(t) ||
            snapshot.openPRs.some(
              (pr) =>
                t === `pr-${pr.number}` ||
                t.startsWith(`pr-${pr.number}-`) ||
                t === `0.42.42-pr-${pr.number}`
            )
        )
      ) {
        r.decision = "keep";
        r.reasons.push("Active PR or test reference");
      } else if (
        snapshot.activeRuns.some((run) => !ALLOWED_WORKFLOWS.has(run.path))
      ) {
        r.reasons.push("Active workflow may use or publish test content");
      } else if (v.tags.length === 0) {
        r.reasons.push("Untagged does not prove unused");
      } else if (!v.tags.every((t) => selectedTag(t, profile, p.name, state))) {
        r.reasons.push("Unrecognized or mixed tags");
      } else {
        r.decision = "delete-candidate";
        r.reasons.push("Expired reviewed test tags");
      }
      records.push(r);
    }
  }
  // A retained parent retains all descendants. A released digest in any linked
  // namespace also protects its copies and their platform manifests.
  let changed;
  do {
    changed = false;
    for (const parent of records.filter(
      (r) => r.id && r.decision !== "delete-candidate"
    )) {
      for (const d of parent.children ?? []) {
        for (const child of records.filter(
          (r) => r.package === parent.package && r.digest === d
        )) {
          if (
            child.decision === "delete-candidate" ||
            (parent.decision === "keep" && child.decision !== "keep")
          ) {
            child.decision = parent.decision === "keep" ? "keep" : "hold";
            child.reasons = ["Referenced by a retained parent"];
            changed = true;
          }
        }
      }
    }
  } while (changed);
  return records;
}
export function deletionOrder(records) {
  const pending = records.filter((r) => r.decision === "delete-candidate");
  const order = [];
  while (pending.length) {
    const next = pending.find(
      (r) =>
        !pending.some(
          (p) =>
            p.package === r.package && (p.children ?? []).includes(r.digest)
        )
    );
    requireThat(next, "Dependency cycle");
    order.push(next);
    pending.splice(pending.indexOf(next), 1);
  }
  return order;
}
export function makePlan(snapshot, evidence, identity, options) {
  requireThat(
    SHA.test(identity.sha) &&
      identity.ref === "refs/heads/main" &&
      /^[a-f0-9]{64}$/.test(identity.policy) &&
      positive(identity.runId) &&
      positive(identity.runAttempt),
    "Invalid source identity"
  );
  const records = classify(snapshot, evidence, options);
  deletionOrder(records);
  return {
    schema: 1,
    repository: REPOSITORY,
    source: identity,
    createdAt: new Date(options.now).toISOString(),
    profile: options.profile,
    retentionDays: options.retentionDays,
    evidenceHash: hash(evidence),
    evidence,
    snapshotHash: hash(snapshot),
    snapshot,
    records
  };
}
export function validatePlan(
  plan,
  approvedHash,
  identity,
  evidence,
  options,
  now
) {
  requireThat(
    /^[a-f0-9]{64}$/.test(approvedHash) && hash(plan) === approvedHash,
    "Plan hash mismatch"
  );
  requireThat(
    plan.schema === 1 && plan.repository === REPOSITORY,
    "Wrong plan repository/schema"
  );
  requireThat(
    canonical(plan.source) === canonical(identity),
    "Plan source/policy/run changed"
  );
  requireThat(
    now >= date(plan.createdAt) && now - date(plan.createdAt) <= MAX_AGE,
    "Plan expired or from the future"
  );
  requireThat(
    plan.evidenceHash === hash(evidence) &&
      canonical(plan.evidence) === canonical(evidence),
    "Protection policy changed"
  );
  requireThat(plan.snapshotHash === hash(plan.snapshot), "Inventory changed");
  const regenerated = makePlan(plan.snapshot, evidence, identity, {
    profile: plan.profile,
    retentionDays: plan.retentionDays,
    now: date(plan.createdAt)
  });
  requireThat(
    canonical(regenerated) === canonical(plan),
    "Plan policy/result mismatch"
  );
  requireThat(
    options.profile === plan.profile &&
      options.retentionDays === plan.retentionDays,
    "Apply inputs changed"
  );
  requireThat(
    evidenceState(plan.snapshot, evidence, now).complete,
    "Release evidence incomplete"
  );
}

export function checkEnvironment(environment, branchPolicies) {
  requireThat(
    environment?.name === "package-cleanup",
    "Missing cleanup environment"
  );
  requireThat(
    environment.can_admins_bypass === false,
    "Cleanup environment must prevent admin bypass"
  );
  requireThat(
    environment.protection_rules?.some(
      (r) =>
        r.type === "required_reviewers" &&
        r.prevent_self_review === true &&
        Array.isArray(r.reviewers) &&
        r.reviewers.length > 0
    ),
    "Required independent cleanup reviewers missing"
  );
  requireThat(
    environment.deployment_branch_policy?.custom_branch_policies === true &&
      environment.deployment_branch_policy?.protected_branches === false,
    "Cleanup environment needs an explicit main-only branch rule"
  );
  const policies = list(
    branchPolicies?.branch_policies,
    "environment branch policies"
  );
  requireThat(
    policies.length === 1 &&
      policies[0].name === "main" &&
      policies[0].type === "branch",
    "Cleanup environment must allow only main"
  );
}
export function verifyGate(client, identity, freeze, now) {
  checkEnvironment(
    client.get(`/repos/${REPOSITORY}/environments/package-cleanup`),
    client.get(
      `/repos/${REPOSITORY}/environments/package-cleanup/deployment-branch-policies?per_page=100`
    )
  );
  const workflows = client.get(
    `/repos/${REPOSITORY}/actions/workflows?per_page=100`
  );
  requireThat(
    workflows.total_count <= 100 &&
      workflows.workflows?.length === workflows.total_count,
    "Incomplete workflow gate"
  );
  requireThat(
    workflows.workflows.every(
      (w) => ALLOWED_WORKFLOWS.has(w.path) || w.state === "disabled_manually"
    ),
    "Other repository workflows must be disabled manually before apply"
  );
  const branch = client.get(`/repos/${REPOSITORY}/branches/main`);
  requireThat(
    branch.commit?.sha === identity.sha,
    "Main moved; generate a new plan"
  );
  requireThat(
    freeze?.repository === REPOSITORY &&
      freeze.sha === identity.sha &&
      freeze.externalPublishersStopped === true &&
      date(freeze.expiresAt) > now &&
      date(freeze.expiresAt) <= now + MAX_AGE,
    "Missing or invalid publication-freeze lease"
  );
  for (const status of [
    "queued",
    "in_progress",
    "waiting",
    "pending",
    "requested"
  ]) {
    const r = client.get(
      `/repos/${REPOSITORY}/actions/runs?status=${status}&per_page=100`
    );
    requireThat(
      r.total_count <= 100 && r.workflow_runs?.length === r.total_count,
      "Incomplete freeze run gate"
    );
    requireThat(
      r.workflow_runs.every(
        (run) => run.id === Number(process.env.GITHUB_RUN_ID)
      ),
      "Other workflow runs must finish or be cancelled before apply"
    );
  }
}
export function verifyPlanRun(client, runId, sha) {
  requireThat(positive(runId) && SHA.test(sha), "Invalid approved run/source");
  const run = client.get(`/repos/${REPOSITORY}/actions/runs/${runId}`);
  requireThat(
    run.id === runId &&
      run.repository?.full_name === REPOSITORY &&
      run.head_repository?.full_name === REPOSITORY &&
      run.head_branch === "main" &&
      run.head_sha === sha &&
      run.path === ".github/workflows/purge-old-images.yaml" &&
      ["schedule", "workflow_dispatch"].includes(run.event) &&
      run.status === "completed" &&
      run.conclusion === "success" &&
      positive(run.run_attempt),
    "Untrusted plan run"
  );
  return run;
}
export function applyPlan(
  client,
  plan,
  fresh,
  { maxDeletes = 25, gate, checkpoint = () => {} } = {}
) {
  requireThat(
    positive(maxDeletes) && maxDeletes <= 100,
    "Delete cap must be 1..100"
  );
  requireThat(typeof gate === "function", "Apply requires a publication gate");
  const report = {
    status: "partial",
    planHash: hash(plan),
    results: plan.records.map((r) => ({
      package: r.package,
      id: r.id,
      digest: r.digest,
      status: r.decision === "delete-candidate" ? "unfinished" : r.decision,
      reasons: r.reasons
    }))
  };
  const resultFor = (r) =>
    report.results.find((x) => x.package === r.package && x.id === r.id);
  const blocked = new Set();
  let attempted = 0;
  const persist = () => {
    report.counts = counts(report.results, "status");
    checkpoint(report);
  };
  persist();
  try {
    for (const r of deletionOrder(plan.records)) {
      const result = resultFor(r);
      const key = `${r.package}@${r.digest}`;
      if (blocked.has(key)) {
        result.reasons = ["Ancestor retained after refused/changed deletion"];
        persist();
        continue;
      }
      if (attempted >= maxDeletes) throw new Incomplete("Delete cap reached");
      gate();
      fresh.releases = client.pages(`/repos/${REPOSITORY}/releases`);
      fresh.openPRs = client.pages(`/repos/${REPOSITORY}/pulls?state=open`);
      const current = classify(fresh, plan.evidence, {
        profile: plan.profile,
        retentionDays: plan.retentionDays,
        now: Date.now()
      });
      const live = current.find(
        (v) => v.package === r.package && v.id === r.id
      );
      if (
        !live &&
        fresh.packages.some(
          (p) => p.name === r.package && p.linked && p.id === r.packageId
        )
      ) {
        result.status = "already-absent";
        result.reasons = ["Absent from fresh complete inventory"];
        retainDescendants(r, plan.records, blocked);
        persist();
        continue;
      }
      if (
        !live ||
        live.decision !== "delete-candidate" ||
        canonical({ ...live, reasons: [], decision: "" }) !==
          canonical({ ...r, reasons: [], decision: "" })
      ) {
        result.reasons = [
          "Fresh inventory or protection differs; needs a new plan"
        ];
        retainDescendants(r, plan.records, blocked);
        persist();
        continue;
      }
      // A whole fresh snapshot above proves reachability. Check this exact
      // version again immediately before DELETE, under the publication freeze.
      const response = client.request(
        "github",
        "GET",
        `${versionPath(r.package)}/${r.id}`
      );
      if (response.status === 404) {
        result.status = "already-absent";
        result.reasons = ["Version absent on revalidation"];
        retainDescendants(r, plan.records, blocked);
        persist();
        continue;
      }
      requireThat(
        response.status === 200,
        `Version revalidation HTTP ${response.status}`
      );
      const raw = normalizeVersion(JSON.parse(response.body));
      const expected = {
        id: r.id,
        digest: r.digest,
        tags: r.tags,
        createdAt: r.createdAt,
        updatedAt: r.updatedAt
      };
      if (canonical(raw) !== canonical(expected)) {
        result.reasons = ["Version tags/digest/timestamps changed"];
        retainDescendants(r, plan.records, blocked);
        persist();
        continue;
      }
      attempted++;
      const deleted = client.request(
        "github",
        "DELETE",
        `${versionPath(r.package)}/${r.id}`
      );
      if (deleted.status === 204) {
        result.status = "deleted";
        result.reasons = ["DELETE returned 204"];
      } else if (deleted.status === 404) {
        result.status = "already-absent";
        result.reasons = ["DELETE returned 404, not counted as deleted"];
        retainDescendants(r, plan.records, blocked);
      } else if (
        deleted.status === 400 &&
        JSON.parse(deleted.body)?.message === PROTECTED_MESSAGE
      ) {
        result.status = "protected";
        result.reasons = [PROTECTED_MESSAGE];
        console.error(
          `::warning::${r.package} version ${r.id}: ${PROTECTED_MESSAGE}`
        );
        retainDescendants(r, plan.records, blocked);
      } else {
        throw new Error(
          `DELETE ${r.package}/${r.id}: HTTP ${deleted.status}: ${deleted.body.slice(0, 500)}`
        );
      }
      persist();
    }
    report.status =
      (
        report.results.some((r) =>
          ["unfinished", "protected"].includes(r.status)
        )
      ) ?
        "partial"
      : "complete";
  } catch (error) {
    report.error = error.message;
    report.status = error instanceof Incomplete ? "partial" : "failed";
  }
  persist();
  return report;
}
function retainDescendants(parent, records, blocked) {
  for (const d of parent.children ?? []) {
    const key = `${parent.package}@${d}`;
    if (blocked.has(key)) continue;
    blocked.add(key);
    const child = records.find(
      (r) => r.package === parent.package && r.digest === d
    );
    if (child) retainDescendants(child, records, blocked);
  }
}
function policyIdentity() {
  const files = [
    ".github/scripts/package-cleanup.mjs",
    ".github/scripts/purge-old-images.sh",
    EVIDENCE,
    ".github/scripts/release-version.sh",
    ".github/scripts/release-oci-artifacts.sh",
    "build/scripts/retry-curl.sh",
    ".github/release-parity/targets.json",
    ".github/workflows/__package-cleanup.yaml",
    ".github/workflows/purge-old-images.yaml",
    ".github/workflows/package-cleanup-apply.yaml",
    ...readdirSync(resolve(ROOT, ".github/release-parity/baselines"))
      .sort()
      .map((f) => `.github/release-parity/baselines/${f}`)
  ];
  return hash(
    files.map((f) => [f, hash(readFileSync(resolve(ROOT, f), "utf8"))])
  );
}
function loadEvidence() {
  const evidence = json(resolve(ROOT, EVIDENCE));
  const sources = [];
  const roots = new Set(list(evidence.protectedDigests, "protected digests"));
  for (const name of readdirSync(
    resolve(ROOT, ".github/release-parity/baselines")
  ).sort()) {
    const path = `.github/release-parity/baselines/${name}`;
    const body = readFileSync(resolve(ROOT, path), "utf8");
    JSON.parse(body);
    for (const match of body.matchAll(/"sha256:[a-f0-9]{64}"/g))
      roots.add(JSON.parse(match[0]));
    sources.push({ path, sha256: hash(body) });
  }
  return {
    ...evidence,
    protectedDigests: [...roots].sort(),
    baselineSources: sources
  };
}
export function main(env = process.env) {
  requireThat(
    env.GITHUB_REPOSITORY === REPOSITORY &&
      env.GITHUB_REF === "refs/heads/main" &&
      ["schedule", "workflow_dispatch"].includes(env.GITHUB_EVENT_NAME) &&
      SHA.test(env.GITHUB_SHA),
    "Cleanup requires the trusted main workflow"
  );
  const mode = env.CLEANUP_MODE ?? "plan";
  requireThat(
    ["plan", "preflight", "apply"].includes(mode),
    "Unknown cleanup mode"
  );
  requireThat(
    typeof env.GH_TOKEN === "string" && env.GH_TOKEN.length > 0,
    "Cleanup credential is empty"
  );
  const options = {
    profile: env.CLEANUP_PROFILE ?? "pr",
    retentionDays: Number(env.RETENTION_DAYS ?? 7),
    now: Date.now()
  };
  requireThat(PROFILES.includes(options.profile), "Unknown cleanup profile");
  requireThat(
    positive(options.retentionDays) &&
      options.retentionDays >= 7 &&
      options.retentionDays <= 3650,
    "Retention must be 7..3650 days"
  );
  if (mode === "apply")
    requireThat(
      positive(Number(env.CLEANUP_MAX_DELETES ?? 25)) &&
        Number(env.CLEANUP_MAX_DELETES ?? 25) <= 100,
      "Delete cap must be 1..100"
    );
  const evidence = loadEvidence();
  const client = new Client(transport, Number(env.CLEANUP_API_BUDGET ?? 4000));
  const runId = Number(mode === "plan" ? env.GITHUB_RUN_ID : env.PLAN_RUN_ID);
  const run =
    mode !== "plan" ? verifyPlanRun(client, runId, env.GITHUB_SHA) : null;
  const identity = {
    sha: env.GITHUB_SHA,
    ref: env.GITHUB_REF,
    policy: policyIdentity(),
    runId,
    runAttempt: run?.run_attempt ?? Number(env.GITHUB_RUN_ATTEMPT)
  };
  const out = resolve(ROOT, "cleanup-output");
  mkdirSync(out, { recursive: true });
  if (mode === "preflight") {
    checkEnvironment(
      client.get(`/repos/${REPOSITORY}/environments/package-cleanup`),
      client.get(
        `/repos/${REPOSITORY}/environments/package-cleanup/deployment-branch-policies?per_page=100`
      )
    );
    requireThat(
      /^[a-f0-9]{64}$/.test(env.PLAN_SHA256 ?? ""),
      "Approved plan hash required"
    );
    writeFileSync(env.GITHUB_OUTPUT, `attempt=${run.run_attempt}\n`);
    return;
  }
  let plan;
  if (mode === "apply") {
    requireThat(env.CLEANUP_ENABLE_APPLY === "true", "Apply is not activated");
    plan = json(resolve(ROOT, "approved-plan/plan.json"));
    validatePlan(
      plan,
      env.PLAN_SHA256,
      identity,
      evidence,
      options,
      Date.now()
    );
    save(resolve(out, "approved-plan.json"), plan);
    const results = plan.records.map((r) => ({
      package: r.package,
      id: r.id,
      digest: r.digest,
      status: r.decision === "delete-candidate" ? "unfinished" : r.decision,
      reasons: r.reasons
    }));
    save(resolve(out, "report.json"), {
      status: "partial",
      planHash: hash(plan),
      counts: counts(results, "status"),
      results
    });
    verifyGate(
      client,
      identity,
      JSON.parse(env.CLEANUP_PUBLICATION_FREEZE ?? "null"),
      Date.now()
    );
  }
  const snapshot = collect(client, (s) =>
    save(resolve(out, "inventory.json"), s)
  );
  if (mode === "plan") {
    plan = makePlan(snapshot, evidence, identity, options);
    save(resolve(out, "plan.json"), plan);
    writeFileSync(resolve(out, "plan.sha256"), `${hash(plan)}\n`);
    save(resolve(out, "report.json"), {
      status: "plan-only",
      counts: counts(plan.records),
      records: plan.records
    });
    console.log(`Plan SHA256 (canonical JSON): ${hash(plan)}`);
  } else {
    const report = applyPlan(client, plan, snapshot, {
      maxDeletes: Number(env.CLEANUP_MAX_DELETES ?? 25),
      gate: () =>
        verifyGate(
          client,
          identity,
          JSON.parse(env.CLEANUP_PUBLICATION_FREEZE ?? "null"),
          Date.now()
        ),
      checkpoint: (r) => save(resolve(out, "report.json"), r)
    });
    console.log(`Cleanup status: ${report.status}`);
    if (report.status !== "complete") process.exitCode = 1;
  }
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  try {
    main();
  } catch (error) {
    const out = resolve(ROOT, "cleanup-output");
    mkdirSync(out, { recursive: true });
    save(resolve(out, "error.json"), {
      status: error instanceof Incomplete ? "partial" : "failed",
      error: error.message
    });
    console.error(error.message);
    process.exitCode = 1;
  }
}
