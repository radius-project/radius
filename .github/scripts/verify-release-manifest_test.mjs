import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import {
  verifyReleaseManifest,
  recheckPublication
} from "./verify-release-manifest.mjs";

const targets = JSON.parse(
  await readFile(
    new URL("../release-parity/targets.json", import.meta.url),
    "utf8"
  )
);
const sourceSha = "a".repeat(40);
const parentSha = "b".repeat(40);
const digest = `sha256:${"c".repeat(64)}`;

function fixture(releaseType = "final") {
  const version =
    releaseType === "rc" ? "0.61.0-rc.1"
    : releaseType === "patch" ? "0.61.1"
    : "0.61.0";
  const channel = releaseType === "rc" ? version : "0.61";
  const plan = {
    schemaVersion: 2,
    version: `v${version}`,
    releaseType,
    channel: "0.61",
    chartVersion: version,
    source: { productCommit: parentSha },
    expectedOutputs: targets,
    siblingRepositories: targets.siblingRepositories.map((repository) => ({
      repository,
      name: repository.split("/")[1],
      sourceCommit: sourceSha
    }))
  };
  const observed = {
    release: {
      tag: plan.version,
      sourceCommit: sourceSha,
      title: `Radius ${plan.version}`,
      prerelease: releaseType === "rc",
      notes: { matchesSource: true }
    },
    cli: {
      assets: targets.cliAssets.map((asset) => ({
        ...asset,
        sha256: "c".repeat(64),
        checksum: { valid: true, declaredSha256: "c".repeat(64) },
        build: {
          goVersion: "go1.26.5",
          settings: { GOOS: asset.os, GOARCH: asset.arch },
          linkerMetadata: {
            channel,
            version: plan.version,
            release: version,
            commit: sourceSha,
            chartVersion: version,
            terraformVersion: "1.15.8"
          }
        }
      })),
      runtimeVersion: {
        version: plan.version,
        release: version,
        commit: sourceSha
      },
      sboms: targets.cliAssets.map((asset) => ({
        name: `${asset.name}.sbom.json`,
        sha256: "c".repeat(64)
      }))
    },
    images: targets.images.map((image) => ({
      name: image.name,
      reference: `${targets.imageRegistry}/${image.name}:${version}`,
      digest,
      platforms: image.requiredPlatforms.map((platform) => ({
        platform,
        config: { labels: { "org.opencontainers.image.revision": sourceSha } }
      }))
    })),
    helm: {
      metadata: { version, appVersion: version },
      descriptor: { digest },
      renderedImages: targets.images
        .filter((image) => targets.helm.expectedImages.includes(image.name))
        .map((image) => `${targets.imageRegistry}/${image.name}:${version}`)
    },
    downstream: {
      repositories: targets.siblingRepositories.map((repository) => ({
        repository,
        commit: sourceSha
      })),
      ociArtifacts: targets.ociArtifacts.map((artifact) => ({
        name: artifact.name,
        reference: `${artifact.repository}:${channel}`,
        descriptor: { digest },
        manifest: { artifactType: artifact.artifactType }
      }))
    }
  };
  return {
    plan,
    observed,
    targets,
    sourceSha,
    parentSha,
    versions: { supported: [{ version: plan.version, channel: plan.channel }] },
    goVersion: "go1.26.5",
    terraformVersion: "1.15.8",
    imageLocks: targets.images
      .filter((image) => image.radiusBuild)
      .map((image) => ({ name: image.name, digest })),
    controllerLock: {
      releaseSourceCommit: sourceSha,
      deploymentEngine: { signedTag: plan.version, digest }
    }
  };
}

test("RC, final and patch verify the same mandatory output set", () => {
  for (const releaseType of ["rc", "final", "patch"]) {
    const report = verifyReleaseManifest(fixture(releaseType));
    assert.deepEqual(report.checks, {
      plan: "verified",
      assets: "verified",
      metadata: "verified",
      images: "verified",
      helm: "verified",
      external: "verified",
      installation: "pending"
    });
    assert.equal(
      report.outputs.some((entry) => entry.status === "failed"),
      false
    );
  }
});

test("reports expected and observed mismatches without weakening other checks", () => {
  for (const [name, mutate, check] of [
    [
      "digest",
      (input) => {
        input.imageLocks[0].digest = `sha256:${"d".repeat(64)}`;
      },
      "images"
    ],
    [
      "missing asset",
      (input) => {
        input.observed.cli.assets.pop();
      },
      "assets"
    ],
    [
      "binary source",
      (input) => {
        input.observed.cli.assets[0].build.linkerMetadata.commit = parentSha;
      },
      "metadata"
    ],
    [
      "Go version",
      (input) => {
        input.observed.cli.assets[0].build.goVersion = "go1.1";
      },
      "metadata"
    ],
    [
      "missing platform",
      (input) => {
        input.observed.images[0].platforms.pop();
      },
      "images"
    ],
    [
      "chart alias",
      (input) => {
        input.observed.helm.renderedImages.pop();
      },
      "helm"
    ],
    [
      "sibling source",
      (input) => {
        input.observed.downstream.repositories[0].commit = parentSha;
      },
      "external"
    ],
    [
      "missing Bicep",
      (input) => {
        input.observed.downstream.ociArtifacts.pop();
      },
      "external"
    ],
    [
      "controller lock",
      (input) => {
        input.controllerLock.releaseSourceCommit = parentSha;
      },
      "external"
    ],
    [
      "unapproved parent",
      (input) => {
        input.parentSha = sourceSha;
      },
      "plan"
    ],
    [
      "notes",
      (input) => {
        input.observed.release.notes.matchesSource = false;
      },
      "assets"
    ],
    [
      "supported version",
      (input) => {
        input.versions.supported = [];
      },
      "plan"
    ],
    [
      "duplicate version",
      (input) => {
        input.versions.supported.push(input.versions.supported[0]);
      },
      "plan"
    ],
    [
      "missing SBOM",
      (input) => {
        input.observed.cli.sboms.pop();
      },
      "assets"
    ]
  ]) {
    const input = fixture();
    mutate(input);
    const report = verifyReleaseManifest(input);
    assert.equal(report.checks[check], "failed", name);
    const failure = report.outputs.find((entry) => entry.status === "failed");
    assert.ok(failure, name);
    assert.notDeepEqual(failure.expected, failure.observed, name);
    assert.equal(report.checks.installation, "pending");
  }
});

test("stable chart gates reject channel tags while preserving non-chart channel contracts", () => {
  for (const releaseType of ["final", "patch"]) {
    const input = fixture(releaseType);
    for (const reference of input.observed.helm.renderedImages) {
      const changed = structuredClone(input);
      changed.observed.helm.renderedImages =
        changed.observed.helm.renderedImages.map((image) =>
          image === reference ? image.replace(/:[^:]+$/, ":0.61") : image
        );
      const report = verifyReleaseManifest(changed);
      assert.equal(report.checks.helm, "failed", reference);
      assert.equal(report.checks.metadata, "verified");
      assert.equal(report.checks.external, "verified");
    }
  }
});

test("approval waits cannot replace verified digests or bypass installation", () => {
  const approved = verifyReleaseManifest(fixture());
  assert.throws(
    () => recheckPublication(verifyReleaseManifest(fixture()), approved),
    /manifest/
  );
  approved.checks.installation = "verified";
  const report = verifyReleaseManifest(fixture());
  recheckPublication(report, approved);
  assert.equal(report.checks.installation, "verified");
  const changed = fixture();
  changed.observed.helm.descriptor.digest = `sha256:${"d".repeat(64)}`;
  const changedReport = verifyReleaseManifest(changed);
  assert.throws(
    () => recheckPublication(changedReport, approved),
    /changed after installation/
  );
  const changedSbom = fixture();
  changedSbom.observed.cli.sboms[0].sha256 = "d".repeat(64);
  assert.throws(
    () => recheckPublication(verifyReleaseManifest(changedSbom), approved),
    /changed after installation/
  );
});

test("the manifest command verifies files and rejects a changed approved snapshot", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "release-manifest-"));
  try {
    const input = fixture();
    await mkdir(path.join(directory, "assets"));
    const document = JSON.stringify({
      spdxVersion: "SPDX-2.3",
      SPDXID: "SPDXRef-DOCUMENT",
      dataLicense: "CC0-1.0",
      documentNamespace: "https://example.test/sbom",
      creationInfo: {
        created: "2026-09-09T00:00:00Z",
        creators: ["Tool: syft-1.51.0"]
      },
      packages: [{ name: "radius" }],
      relationships: []
    });
    for (const sbom of input.observed.cli.sboms) {
      sbom.sha256 = createHash("sha256").update(document).digest("hex");
      await writeFile(path.join(directory, "assets", sbom.name), document);
    }
    const files = {
      "plan.json": input.plan,
      "targets.json": input.targets,
      "versions.json": input.versions,
      "observed.json": input.observed,
      "release-image-digests.json": input.imageLocks,
      "controller-lock.json": input.controllerLock,
      "context.json": {
        sourceSha,
        parentSha,
        goVersion: input.goVersion,
        terraformVersion: input.terraformVersion
      }
    };
    for (const [name, data] of Object.entries(files))
      await writeFile(path.join(directory, name), JSON.stringify(data));
    const script = fileURLToPath(
      new URL("./verify-release-manifest.mjs", import.meta.url)
    );
    const execute = (mode) =>
      spawnSync(process.execPath, [script, directory, mode], {
        encoding: "utf8",
        env: {
          ...process.env,
          RELEASE_VERIFY_APPROVED_MANIFEST: path.join(
            directory,
            "approved.json"
          )
        }
      });
    let result = execute("verify");
    assert.equal(result.status, 0, result.stderr);
    const report = JSON.parse(
      await readFile(path.join(directory, "release-manifest.json"), "utf8")
    );
    assert.equal(report.checks.installation, "pending");
    report.checks.installation = "verified";
    await writeFile(
      path.join(directory, "approved.json"),
      JSON.stringify(report)
    );
    result = execute("recheck");
    assert.equal(result.status, 0, result.stderr);
    input.observed.helm.descriptor.digest = `sha256:${"d".repeat(64)}`;
    await writeFile(
      path.join(directory, "observed.json"),
      JSON.stringify(input.observed)
    );
    result = execute("recheck");
    assert.equal(result.status, 1);
    assert.match(result.stderr, /changed after installation/);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
