import { DefaultArtifactClient } from "@actions/artifact";
import { chmodSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";

export interface ArtifactClient {
  deleteArtifact(name: string): Promise<{ id: number }>;
  uploadArtifact(
    name: string,
    files: string[],
    rootDirectory: string,
    options?: { retentionDays?: number }
  ): Promise<{ id?: number; size?: number }>;
}

export interface UploadRequest {
  artifactName: string;
  progressFile: string;
  retentionDays: number;
  replaceExistingSlot: boolean;
}

export function prepareArtifactRuntimeEnvironment(
  env: NodeJS.ProcessEnv = process.env
): string {
  const runtimeToken = env.ACTIONS_RUNTIME_TOKEN;
  const resultsUrl = env.ACTIONS_RESULTS_URL;
  const runnerTemp = env.RUNNER_TEMP;
  if (!runtimeToken) {
    throw new Error("Unable to get the ACTIONS_RUNTIME_TOKEN env variable");
  }
  if (!resultsUrl) {
    throw new Error("Unable to get the ACTIONS_RESULTS_URL env variable");
  }
  if (!runnerTemp) {
    throw new Error("Unable to get the RUNNER_TEMP env variable");
  }

  const progressDirectory = join(runnerTemp, "radius-deploy-progress");
  mkdirSync(progressDirectory, { recursive: true, mode: 0o700 });
  chmodSync(progressDirectory, 0o700);
  const runtimeFile = join(progressDirectory, "artifact-runtime.json");
  writeFileSync(
    runtimeFile,
    `${JSON.stringify({ runtimeToken, resultsUrl })}\n`,
    { mode: 0o600 }
  );
  chmodSync(runtimeFile, 0o600);
  return runtimeFile;
}

export async function uploadProgressArtifact(
  client: ArtifactClient,
  request: UploadRequest
): Promise<{ artifactId: number; size: number }> {
  if (request.replaceExistingSlot) {
    try {
      await client.deleteArtifact(request.artifactName);
    } catch {
      // The first use of a ring slot has nothing to delete. Upload remains the
      // authoritative operation and will surface real conflicts or API errors.
    }
  }

  const result = await client.uploadArtifact(
    request.artifactName,
    [request.progressFile],
    dirname(request.progressFile),
    { retentionDays: request.retentionDays }
  );
  if (result.id === undefined) {
    throw new Error("artifact upload completed without an artifact ID");
  }
  return { artifactId: result.id, size: result.size ?? 0 };
}

function parseRequest(args: string[]): UploadRequest {
  if (args.length !== 4) {
    throw new Error(
      "usage: upload <artifact-name> <progress-file> <retention-days> <replace-existing-slot>"
    );
  }

  const retentionDays = Number.parseInt(args[2], 10);
  if (!Number.isInteger(retentionDays) || retentionDays < 1) {
    throw new Error("retention-days must be a positive integer");
  }
  if (args[3] !== "true" && args[3] !== "false") {
    throw new Error("replace-existing-slot must be true or false");
  }

  return {
    artifactName: args[0],
    progressFile: args[1],
    retentionDays,
    replaceExistingSlot: args[3] === "true"
  };
}

function redactRuntimeCredentials(
  message: string,
  env: NodeJS.ProcessEnv
): string {
  let redacted = message;
  for (const credential of [
    env.ACTIONS_RUNTIME_TOKEN,
    env.ACTIONS_RESULTS_URL
  ]) {
    if (credential) {
      redacted = redacted.replaceAll(credential, "[REDACTED]");
    }
  }
  return redacted;
}

export async function run(
  args: string[],
  client: ArtifactClient = new DefaultArtifactClient(),
  env: NodeJS.ProcessEnv = process.env
): Promise<number> {
  try {
    if (args.length === 0) {
      const runtimeFile = prepareArtifactRuntimeEnvironment(env);
      process.stdout.write(`${JSON.stringify({ ok: true, runtimeFile })}\n`);
      return 0;
    }
    const request = parseRequest(args);
    const result = await uploadProgressArtifact(client, request);
    process.stdout.write(`${JSON.stringify({ ok: true, ...result })}\n`);
    return 0;
  } catch (error) {
    const rawMessage = error instanceof Error ? error.message : String(error);
    const message = redactRuntimeCredentials(rawMessage, env);
    process.stdout.write(`${JSON.stringify({ ok: false, error: message })}\n`);
    return 1;
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.exitCode = await run(process.argv.slice(2));
}
