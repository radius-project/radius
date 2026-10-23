import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import promoteBicepTypes, {
  promotionRequests
} from "./promote-bicep-types.mjs";
import { mandatoryChecks } from "./publish-draft-release.mjs";

const sourceSha = "a".repeat(40);
const tag = "v0.61.2";
const digest = `sha256:${"b".repeat(64)}`;
const fixture = () => ({
  schemaVersion: 1,
  tag,
  sourceSha,
  releaseType: "patch",
  channel: "0.61",
  checks: Object.fromEntries(mandatoryChecks.map((name) => [name, "verified"])),
  observed: {
    downstream: {
      ociArtifacts: ["radius", "aws"].map((target) => ({
        name: `${target}-bicep-types`,
        reference: `biceptypes.azurecr.io/${target}:0.61.2`,
        descriptor: { digest }
      }))
    }
  }
});

test("promotion requests bind both aliases to the verified immutable digests", () => {
  const requests = promotionRequests(fixture(), tag, sourceSha);
  assert.deepEqual(
    requests.map((request) => request.registry_target),
    ["radius", "aws"]
  );
  for (const request of requests) {
    assert.equal(request.digest, digest);
    assert.equal(request.version, "0.61.2");
    assert.equal(request.channel, "0.61");
    assert.equal(request.source_sha, sourceSha);
    assert.equal(
      request.release_identifier,
      `0.61.2-${sourceSha}-${request.registry_target}`
    );
  }
});

test("unverified, prerelease, conflicting and incomplete manifests cannot promote", () => {
  for (const mutate of [
    (manifest) => {
      manifest.checks.installation = "pending";
    },
    (manifest) => {
      manifest.releaseType = "rc";
    },
    (manifest) => {
      manifest.channel = "0.62";
    },
    (manifest) => {
      manifest.sourceSha = "c".repeat(40);
    },
    (manifest) => {
      manifest.observed.downstream.ociArtifacts.pop();
    },
    (manifest) => {
      manifest.observed.downstream.ociArtifacts[0].reference =
        "biceptypes.azurecr.io/radius:0.61";
    },
    (manifest) => {
      manifest.observed.downstream.ociArtifacts[0].descriptor.digest =
        "invalid";
    }
  ]) {
    const manifest = fixture();
    mutate(manifest);
    assert.throws(() => promotionRequests(manifest, tag, sourceSha));
  }
});

test("promotion reconciles successful old runs and stops immediately on failure", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "bicep-promotion-"));
  try {
    const manifestFile = path.join(directory, "manifest.json");
    await writeFile(manifestFile, JSON.stringify(fixture()));
    const inputs = {
      MANIFEST_FILE: manifestFile,
      TAG: tag,
      SOURCE_SHA: sourceSha
    };
    const outputs = {};
    const core = {
      getInput: (name) => inputs[name] ?? "",
      setOutput: (name, value) => {
        outputs[name] = value;
      }
    };
    const requests = [];
    await promoteBicepTypes({
      github: {},
      context: {},
      core,
      monitor: async ({ core }) => {
        assert.equal(core.getInput("REUSE_SUCCESSFUL"), "false");
        assert.equal(core.getInput("EVENT_TYPE"), "bicep-types.promote");
        const request = JSON.parse(core.getInput("CLIENT_PAYLOAD"));
        requests.push(request);
        core.setOutput("run_url", request.registry_target);
      }
    });
    assert.equal(requests.length, 2);
    assert.deepEqual(outputs, { radius_run_url: "radius", aws_run_url: "aws" });

    let attempts = 0;
    await assert.rejects(
      () =>
        promoteBicepTypes({
          github: {},
          context: {},
          core,
          monitor: async ({ core }) => {
            attempts++;
            core.setFailed("Publisher digest mismatch");
          }
        }),
      /Publisher digest mismatch/
    );
    assert.equal(attempts, 1);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
