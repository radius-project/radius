import assert from "node:assert/strict";
import { readFileSync, statSync } from "node:fs";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
  type ArtifactClient,
  prepareArtifactRuntimeEnvironment,
  run,
  uploadProgressArtifact
} from "./upload.ts";

class FakeArtifactClient implements ArtifactClient {
  readonly calls: string[] = [];

  async deleteArtifact(name: string): Promise<{ id: number }> {
    this.calls.push(`delete:${name}`);
    return { id: 41 };
  }

  async uploadArtifact(
    name: string,
    files: string[],
    rootDirectory: string,
    options?: { retentionDays?: number }
  ): Promise<{ id: number; size: number }> {
    this.calls.push(
      `upload:${name}:${files.join(",")}:${rootDirectory}:${options?.retentionDays}`
    );
    return { id: 42, size: 128 };
  }
}

test("prepares a private artifact runtime file for the deploy shell", async () => {
  const runnerTemp = await mkdtemp(join(tmpdir(), "radius-artifact-runtime-"));
  try {
    const runtimeFile = prepareArtifactRuntimeEnvironment({
      ACTIONS_RUNTIME_TOKEN: "runtime-token",
      ACTIONS_RESULTS_URL: "https://results.example.test/path",
      RUNNER_TEMP: runnerTemp
    });

    assert.deepEqual(JSON.parse(readFileSync(runtimeFile, "utf8")), {
      runtimeToken: "runtime-token",
      resultsUrl: "https://results.example.test/path"
    });
    assert.equal(statSync(runtimeFile).mode & 0o777, 0o600);
    assert.equal(
      statSync(join(runnerTemp, "radius-deploy-progress")).mode & 0o777,
      0o700
    );
  } finally {
    await rm(runnerTemp, { recursive: true, force: true });
  }
});

test("rejects missing artifact runtime credentials", () => {
  assert.throws(
    () => prepareArtifactRuntimeEnvironment({ RUNNER_TEMP: "/tmp" }),
    /ACTIONS_RUNTIME_TOKEN/
  );
});

test("replaces only the requested slot before upload", async () => {
  const client = new FakeArtifactClient();
  const result = await uploadProgressArtifact(client, {
    artifactName: "deploy-live-123-slot-7",
    progressFile: "/tmp/deploy-progress.json",
    retentionDays: 1,
    replaceExistingSlot: true
  });

  assert.deepEqual(client.calls, [
    "delete:deploy-live-123-slot-7",
    "upload:deploy-live-123-slot-7:/tmp/deploy-progress.json:/tmp:1"
  ]);
  assert.deepEqual(result, { artifactId: 42, size: 128 });
});

test("uploads a fresh slot without deletion", async () => {
  const client = new FakeArtifactClient();
  await uploadProgressArtifact(client, {
    artifactName: "deploy-live-123-slot-0",
    progressFile: "/tmp/deploy-progress.json",
    retentionDays: 1,
    replaceExistingSlot: false
  });

  assert.deepEqual(client.calls, [
    "upload:deploy-live-123-slot-0:/tmp/deploy-progress.json:/tmp:1"
  ]);
});

test("continues when a requested slot does not exist yet", async () => {
  const client = new FakeArtifactClient();
  client.deleteArtifact = async (name: string) => {
    client.calls.push(`delete:${name}`);
    throw new Error("artifact not found");
  };

  await uploadProgressArtifact(client, {
    artifactName: "deploy-live-123-slot-0",
    progressFile: "/tmp/deploy-progress.json",
    retentionDays: 1,
    replaceExistingSlot: true
  });

  assert.deepEqual(client.calls, [
    "delete:deploy-live-123-slot-0",
    "upload:deploy-live-123-slot-0:/tmp/deploy-progress.json:/tmp:1"
  ]);
});

test("redacts artifact runtime credentials from upload failures", async () => {
  const runtimeToken = "runtime-token-that-must-not-leak";
  const resultsUrl = "https://results.example.test/private";
  const client = new FakeArtifactClient();
  client.uploadArtifact = async () => {
    throw new Error(`upload failed: token=${runtimeToken} url=${resultsUrl}`);
  };

  let output = "";
  const originalWrite = process.stdout.write;
  process.stdout.write = ((chunk: string | Uint8Array) => {
    output += chunk.toString();
    return true;
  }) as typeof process.stdout.write;

  try {
    const exitCode = await run(
      ["deploy-live-123-slot-0", "/tmp/deploy-progress.json", "1", "false"],
      client,
      {
        ACTIONS_RUNTIME_TOKEN: runtimeToken,
        ACTIONS_RESULTS_URL: resultsUrl
      }
    );

    assert.equal(exitCode, 1);
    assert.deepEqual(JSON.parse(output), {
      ok: false,
      error: "upload failed: token=[REDACTED] url=[REDACTED]"
    });
    assert.doesNotMatch(output, new RegExp(runtimeToken));
    assert.doesNotMatch(output, new RegExp(resultsUrl));
  } finally {
    process.stdout.write = originalWrite;
  }
});
