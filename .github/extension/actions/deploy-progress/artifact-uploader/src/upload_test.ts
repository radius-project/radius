import assert from "node:assert/strict";
import test from "node:test";

import { type ArtifactClient, uploadProgressArtifact } from "./upload.ts";

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
    options?: { retentionDays?: number },
  ): Promise<{ id: number; size: number }> {
    this.calls.push(
      `upload:${name}:${files.join(",")}:${rootDirectory}:${options?.retentionDays}`,
    );
    return { id: 42, size: 128 };
  }
}

test("replaces only the requested slot before upload", async () => {
  const client = new FakeArtifactClient();
  const result = await uploadProgressArtifact(client, {
    artifactName: "deploy-live-123-slot-7",
    progressFile: "/tmp/deploy-progress.json",
    retentionDays: 1,
    replaceExistingSlot: true,
  });

  assert.deepEqual(client.calls, [
    "delete:deploy-live-123-slot-7",
    "upload:deploy-live-123-slot-7:/tmp/deploy-progress.json:/tmp:1",
  ]);
  assert.deepEqual(result, { artifactId: 42, size: 128 });
});

test("uploads a fresh slot without deletion", async () => {
  const client = new FakeArtifactClient();
  await uploadProgressArtifact(client, {
    artifactName: "deploy-live-123-slot-0",
    progressFile: "/tmp/deploy-progress.json",
    retentionDays: 1,
    replaceExistingSlot: false,
  });

  assert.deepEqual(client.calls, [
    "upload:deploy-live-123-slot-0:/tmp/deploy-progress.json:/tmp:1",
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
    replaceExistingSlot: true,
  });

  assert.deepEqual(client.calls, [
    "delete:deploy-live-123-slot-0",
    "upload:deploy-live-123-slot-0:/tmp/deploy-progress.json:/tmp:1",
  ]);
});
