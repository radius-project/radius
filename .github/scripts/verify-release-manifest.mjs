import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { isDeepStrictEqual } from "node:util";
import { pathToFileURL } from "node:url";
import path from "node:path";

import {
  mandatoryChecks,
  validatePublicationManifest
} from "./publish-draft-release.mjs";
import { verifySpdxDocument } from "./release-assets.mjs";

const digestPattern = /^sha256:[a-f0-9]{64}$/;
const sorted = (values) => [...values].sort();

export function verifyReleaseManifest({
  plan,
  targets,
  observed,
  imageLocks,
  controllerLock,
  sourceSha,
  parentSha,
  goVersion,
  terraformVersion,
  versions
}) {
  const version = plan.version?.replace(/^v/, "");
  const artifactTag = plan.releaseType === "rc" ? version : plan.channel;
  const outputs = [];
  const compare = (check, name, expected, actual) => {
    outputs.push({
      check,
      name,
      expected: expected ?? null,
      observed: actual ?? null,
      status: isDeepStrictEqual(expected, actual) ? "verified" : "failed"
    });
  };
  compare("plan", "schema", 2, plan.schemaVersion);
  compare("plan", "source SHA", true, /^[a-f0-9]{40}$/.test(sourceSha));
  compare("plan", "approved parent", parentSha, plan.source?.productCommit);
  compare("plan", "output contract", targets, plan.expectedOutputs);
  compare(
    "plan",
    "release type",
    true,
    ["rc", "final", "patch"].includes(plan.releaseType)
  );
  compare(
    "plan",
    "tag policy",
    true,
    /^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/.test(plan.version)
  );
  compare(
    "plan",
    "RC classification",
    plan.releaseType === "rc",
    plan.version?.includes("-")
  );
  compare(
    "plan",
    "channel",
    version?.split(".").slice(0, 2).join("."),
    plan.channel
  );
  compare("plan", "chart version", version, plan.chartVersion);
  compare(
    "plan",
    "supported release metadata",
    1,
    versions?.supported?.filter(
      (entry) =>
        entry.version === plan.version && entry.channel === plan.channel
    ).length
  );
  compare("plan", "tag", plan.version, observed.release?.tag);
  compare("plan", "tag source", sourceSha, observed.release?.sourceCommit);
  compare(
    "assets",
    "release title",
    `Radius ${plan.version}`,
    observed.release?.title
  );
  compare(
    "assets",
    "prerelease",
    plan.releaseType === "rc",
    observed.release?.prerelease
  );
  compare(
    "assets",
    "prepared release notes",
    true,
    observed.release?.notes?.matchesSource
  );
  compare(
    "assets",
    "CLI asset set",
    sorted(targets.cliAssets.map((asset) => asset.name)),
    sorted((observed.cli?.assets ?? []).map((asset) => asset.name))
  );
  compare(
    "assets",
    "CLI SBOM set",
    sorted(targets.cliAssets.map((asset) => `${asset.name}.sbom.json`)),
    sorted((observed.cli?.sboms ?? []).map((asset) => asset.name))
  );
  for (const sbom of observed.cli?.sboms ?? []) {
    compare(
      "assets",
      `${sbom.name} digest`,
      true,
      /^[a-f0-9]{64}$/.test(sbom.sha256)
    );
  }
  for (const target of targets.cliAssets) {
    const asset = observed.cli?.assets?.find(
      (entry) => entry.name === target.name
    );
    compare("assets", `${target.name} checksum`, true, asset?.checksum?.valid);
    compare(
      "assets",
      `${target.name} content digest`,
      true,
      /^[a-f0-9]{64}$/.test(asset?.sha256)
    );
    compare(
      "assets",
      `${target.name} declared digest`,
      asset?.sha256,
      asset?.checksum?.declaredSha256
    );
    compare(
      "metadata",
      `${target.name} Go version`,
      goVersion,
      asset?.build?.goVersion
    );
    for (const [key, value] of Object.entries({
      channel: artifactTag,
      release: version,
      version: plan.version,
      commit: sourceSha,
      chartVersion: plan.chartVersion,
      terraformVersion
    })) {
      compare(
        "metadata",
        `${target.name} ${key}`,
        value,
        asset?.build?.linkerMetadata?.[key]
      );
    }
    compare(
      "metadata",
      `${target.name} OS`,
      target.os,
      asset?.build?.settings?.GOOS
    );
    compare(
      "metadata",
      `${target.name} architecture`,
      target.arch,
      asset?.build?.settings?.GOARCH
    );
  }
  for (const [key, value] of Object.entries({
    release: version,
    version: plan.version,
    commit: sourceSha
  })) {
    compare(
      "metadata",
      `CLI runtime ${key}`,
      value,
      observed.cli?.runtimeVersion?.[key]
    );
  }
  compare(
    "images",
    "image set",
    sorted(targets.images.map((image) => image.name)),
    sorted((observed.images ?? []).map((image) => image.name))
  );
  compare(
    "images",
    "image lock set",
    sorted(
      targets.images
        .filter((image) => image.radiusBuild)
        .map((image) => image.name)
    ),
    sorted(imageLocks.map((image) => image.name))
  );
  for (const target of targets.images) {
    const image = observed.images?.find((entry) => entry.name === target.name);
    compare(
      "images",
      `${target.name} digest`,
      true,
      digestPattern.test(image?.digest)
    );
    compare(
      "images",
      `${target.name} platforms`,
      sorted(target.requiredPlatforms),
      sorted((image?.platforms ?? []).map((platform) => platform.platform))
    );
    compare(
      "images",
      `${target.name} reference`,
      `${targets.imageRegistry}/${target.name}:${version}`,
      image?.reference
    );
    if (target.radiusBuild) {
      const lock = imageLocks.find((entry) => entry.name === target.name);
      compare(
        "images",
        `${target.name} locked digest`,
        lock?.digest,
        image?.digest
      );
    }
  }
  compare(
    "helm",
    "chart version",
    plan.chartVersion,
    observed.helm?.metadata?.version
  );
  compare(
    "helm",
    "chart app version",
    version,
    observed.helm?.metadata?.appVersion
  );
  compare(
    "helm",
    "chart digest",
    true,
    digestPattern.test(observed.helm?.descriptor?.digest)
  );
  for (const target of targets.images.filter((image) =>
    targets.helm.expectedImages.includes(image.name)
  )) {
    compare(
      "helm",
      `${target.name} chart reference`,
      true,
      observed.helm?.renderedImages?.includes(
        `${targets.imageRegistry}/${target.name}:${version}`
      )
    );
  }
  compare(
    "external",
    "sibling repository set",
    sorted(targets.siblingRepositories),
    sorted(
      (observed.downstream?.repositories ?? []).map((entry) => entry.repository)
    )
  );
  for (const repository of targets.siblingRepositories) {
    const planned = plan.siblingRepositories?.find(
      (entry) => entry.repository === repository
    );
    compare(
      "external",
      `${repository} planned commit`,
      true,
      /^[a-f0-9]{40}$/.test(planned?.sourceCommit)
    );
    compare(
      "external",
      `${repository} tag commit`,
      planned?.sourceCommit,
      observed.downstream?.repositories?.find(
        (entry) => entry.repository === repository
      )?.commit
    );
  }
  const dashboard = plan.siblingRepositories?.find(
    (entry) => entry.name === "dashboard"
  );
  const dashboardImage = observed.images?.find(
    (image) => image.name === "dashboard"
  );
  for (const platform of dashboardImage?.platforms ?? []) {
    compare(
      "external",
      `dashboard ${platform.platform} source`,
      dashboard?.sourceCommit,
      platform.config?.labels?.["org.opencontainers.image.revision"]
    );
  }
  compare(
    "external",
    "Deployment Engine release binding",
    sourceSha,
    controllerLock.releaseSourceCommit
  );
  compare(
    "external",
    "Deployment Engine signed tag",
    plan.version,
    controllerLock.deploymentEngine?.signedTag
  );
  compare(
    "external",
    "Deployment Engine locked digest",
    controllerLock.deploymentEngine?.digest,
    observed.images?.find((image) => image.name === "deployment-engine")?.digest
  );
  compare(
    "external",
    "Bicep output set",
    sorted(targets.ociArtifacts.map((entry) => entry.name)),
    sorted((observed.downstream?.ociArtifacts ?? []).map((entry) => entry.name))
  );
  for (const target of targets.ociArtifacts) {
    const artifact = observed.downstream?.ociArtifacts?.find(
      (entry) => entry.name === target.name
    );
    compare(
      "external",
      `${target.name} reference`,
      `${target.repository}:${artifactTag}`,
      artifact?.reference
    );
    compare(
      "external",
      `${target.name} type`,
      target.artifactType,
      artifact?.manifest?.artifactType
    );
    compare(
      "external",
      `${target.name} digest`,
      true,
      digestPattern.test(artifact?.descriptor?.digest)
    );
  }
  const checks = Object.fromEntries(
    mandatoryChecks.map((name) => [
      name,
      name === "installation" ? "pending"
      : (
        outputs.some(
          (entry) => entry.check === name && entry.status !== "verified"
        )
      ) ?
        "failed"
      : "verified"
    ])
  );
  const stableObserved = structuredClone(observed);
  delete stableObserved.observedAt;
  delete stableObserved.release.publishedAt;
  delete stableObserved.release.draft;
  return {
    schemaVersion: 1,
    tag: plan.version,
    sourceSha,
    releaseType: plan.releaseType,
    channel: plan.channel,
    planSha256: createHash("sha256").update(JSON.stringify(plan)).digest("hex"),
    checks,
    outputs,
    observed: stableObserved
  };
}

export function recheckPublication(report, approved) {
  validatePublicationManifest(approved, report.tag, report.sourceSha);
  if (
    !isDeepStrictEqual(report.outputs, approved.outputs) ||
    !isDeepStrictEqual(report.observed, approved.observed) ||
    report.planSha256 !== approved.planSha256
  ) {
    throw new Error(
      "Release outputs changed after installation verification; publication is blocked"
    );
  }
  report.checks.installation = "verified";
  validatePublicationManifest(report, report.tag, report.sourceSha);
}

async function main(directory, mode) {
  const load = async (name) =>
    JSON.parse(await readFile(path.join(directory, name), "utf8"));
  const observed = await load("observed.json");
  for (const sbom of observed.cli.sboms ?? []) {
    const data = await readFile(path.join(directory, "assets", sbom.name));
    verifySpdxDocument(data, sbom.name);
    if (createHash("sha256").update(data).digest("hex") !== sbom.sha256)
      throw new Error(`SBOM ${sbom.name} changed during verification`);
  }
  const report = verifyReleaseManifest({
    ...(await load("context.json")),
    plan: await load("plan.json"),
    versions: await load("versions.json"),
    targets: await load("targets.json"),
    observed,
    imageLocks: await load("release-image-digests.json"),
    controllerLock: await load("controller-lock.json")
  });
  await writeFile(
    path.join(directory, "release-manifest.json"),
    `${JSON.stringify(report, null, 2)}\n`
  );
  const failures = report.outputs.filter((entry) => entry.status === "failed");
  if (failures.length) {
    throw new Error(
      `Release verification failed: ${failures.map((entry) => `${entry.name}: expected ${JSON.stringify(entry.expected)}, observed ${JSON.stringify(entry.observed)}`).join("; ")}`
    );
  }
  if (mode === "recheck") {
    const approved = JSON.parse(
      await readFile(process.env.RELEASE_VERIFY_APPROVED_MANIFEST, "utf8")
    );
    recheckPublication(report, approved);
    await writeFile(
      path.join(directory, "release-manifest.json"),
      `${JSON.stringify(approved, null, 2)}\n`
    );
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  await main(process.argv[2], process.argv[3]);
}
