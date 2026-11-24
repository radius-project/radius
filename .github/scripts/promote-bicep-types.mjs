import { readFile } from "node:fs/promises";
import monitorRemoteWorkflow from "./monitor-remote-workflow.mjs";
import { validatePublicationManifest } from "./publish-draft-release.mjs";
import { verifyBicepExtensionOutputs } from "./release-bicep-extensions.mjs";

export function promotionRequests(manifest, tag, sourceSha, plan, lock) {
  validatePublicationManifest(manifest, tag, sourceSha);
  if (plan || lock) {
    if (plan?.version !== tag)
      throw new Error("Bicep promotion plan tag mismatch");
    verifyBicepExtensionOutputs(
      plan,
      sourceSha,
      lock,
      manifest.observed?.downstream?.ociArtifacts
    );
  }
  const version = tag.replace(/^v/, "");
  if (
    !/^v\d+\.\d+\.\d+$/.test(tag) ||
    !["final", "patch"].includes(manifest.releaseType) ||
    manifest.channel !== version.split(".").slice(0, 2).join(".")
  ) {
    throw new Error("Bicep aliases require a verified stable release");
  }
  return ["radius", "aws"].map((target) => {
    const artifacts = manifest.observed?.downstream?.ociArtifacts?.filter(
      (artifact) => artifact.name === `${target}-bicep-types`
    );
    const artifact = artifacts?.[0];
    if (
      artifacts?.length !== 1 ||
      artifact.reference !==
        (plan ?
          `ghcr.io/radius-project/bicep-types-${target}:${version}`
        : `biceptypes.azurecr.io/${target}:${version}`) ||
      !/^sha256:[0-9a-f]{64}$/.test(artifact.descriptor?.digest)
    ) {
      throw new Error(`No verified immutable ${target} Bicep artifact`);
    }
    return {
      source_repository: "radius-project/radius",
      source_sha: sourceSha,
      registry_target: target,
      version,
      channel: manifest.channel,
      digest: artifact.descriptor.digest,
      release_identifier: `${version}-${sourceSha}-${target}`
    };
  });
}

export async function dispatchPromotion(
  github,
  core,
  request,
  monitor = monitorRemoteWorkflow
) {
  const inputs = {
    OWNER: "azure-octo",
    REPO: "radius-publisher",
    WORKFLOW_FILE: "promote-bicep-types.yml",
    EVENT_TYPE: "bicep-types.promote",
    RELEASE_IDENTIFIER: request.release_identifier,
    CLIENT_PAYLOAD: JSON.stringify(request),
    REUSE_SUCCESSFUL: "false",
    MAX_WAIT_SECONDS: "300",
    POLL_INTERVAL_SECONDS: "10"
  };
  await monitor({
    github,
    core: {
      ...core,
      getInput: (name) => inputs[name] ?? "",
      setFailed: (message) => {
        throw new Error(message);
      }
    }
  });
}

export default async function promoteBicepTypes({
  github,
  context,
  core,
  monitor = monitorRemoteWorkflow
}) {
  const manifest = JSON.parse(
    await readFile(core.getInput("MANIFEST_FILE", { required: true }), "utf8")
  );
  const requests = promotionRequests(
    manifest,
    core.getInput("TAG", { required: true }),
    core.getInput("SOURCE_SHA", { required: true })
  );
  for (const request of requests) {
    await dispatchPromotion(
      github,
      {
        ...core,
        setOutput: (name, value) =>
          core.setOutput(`${request.registry_target}_${name}`, value)
      },
      request,
      monitor
    );
  }
}
