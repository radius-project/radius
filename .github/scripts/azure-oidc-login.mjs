// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import { spawnSync } from "node:child_process";
import { setTimeout } from "node:timers/promises";
import { pathToFileURL } from "node:url";

const retryableStatuses = new Set([408, 429, 500, 502, 503, 504]);
const retryableCodes = new Set([
  "ECONNRESET",
  "ETIMEDOUT",
  "EAI_AGAIN",
  "UND_ERR_CONNECT_TIMEOUT",
  "UND_ERR_HEADERS_TIMEOUT",
  "UND_ERR_BODY_TIMEOUT",
  "UND_ERR_SOCKET"
]);

class TokenRequestError extends Error {
  constructor(message, retryable) {
    super(message);
    this.retryable = retryable;
  }
}

export async function requestIDToken(
  url,
  token,
  { fetch = globalThis.fetch, timeoutMs = 30000 } = {}
) {
  let response;
  try {
    response = await fetch(url, {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.timeout(timeoutMs),
      redirect: "error"
    });
    if (response.ok) {
      const body = await response.json();
      if (
        typeof body.value !== "string" ||
        !/^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(body.value)
      ) {
        throw new TokenRequestError(
          "GitHub OIDC response has no valid ID token.",
          false
        );
      }
      return body.value;
    }
    await response.body?.cancel();
    throw new TokenRequestError(
      `GitHub OIDC token request returned HTTP ${response.status}.`,
      retryableStatuses.has(response.status)
    );
  } catch (error) {
    if (error instanceof TokenRequestError) {
      throw error;
    }
    // Do not print request errors: they can contain the URL or credentials.
    const retryable =
      error.name === "TimeoutError" ||
      retryableCodes.has(error.cause?.code ?? error.code);
    throw new TokenRequestError("GitHub OIDC token request failed.", retryable);
  }
}

export async function getIDToken(
  env,
  { request = requestIDToken, delay = setTimeout, warn = console.warn } = {}
) {
  for (const name of [
    "ACTIONS_ID_TOKEN_REQUEST_URL",
    "ACTIONS_ID_TOKEN_REQUEST_TOKEN"
  ]) {
    if (!env[name]) {
      throw new Error(`${name} is required; check id-token: write permission.`);
    }
  }
  const url = new URL(env.ACTIONS_ID_TOKEN_REQUEST_URL);
  if (url.protocol !== "https:") {
    throw new Error("GitHub OIDC token URL must use HTTPS.");
  }
  url.searchParams.set("audience", "api://AzureADTokenExchange");
  for (let attempt = 1; ; attempt++) {
    try {
      return await request(url, env.ACTIONS_ID_TOKEN_REQUEST_TOKEN);
    } catch (error) {
      if (
        !(error instanceof TokenRequestError) ||
        !error.retryable ||
        attempt === 3
      ) {
        throw error;
      }
      warn(`GitHub OIDC token request failed; retrying (${attempt}/2).`);
      await delay(2000 * attempt);
    }
  }
}

export function runAzure(args, spawn = spawnSync) {
  // Do not let a child-process exception include the token in its command line.
  const result = spawn("az", args, { stdio: ["ignore", "ignore", "inherit"] });
  if (result.error || result.status !== 0) {
    throw new Error(
      `Azure CLI ${args[0]} failed (status ${result.status ?? "unavailable"}).`
    );
  }
}

export async function login(
  env,
  {
    getToken = getIDToken,
    run = runAzure,
    mask = (value) => console.log(`::add-mask::${value}`)
  } = {}
) {
  for (const name of [
    "AZURE_CLIENT_ID",
    "AZURE_TENANT_ID",
    "AZURE_SUBSCRIPTION_ID"
  ]) {
    if (!env[name]) {
      throw new Error(`${name} is required.`);
    }
  }
  mask(env.AZURE_CLIENT_ID);
  const token = await getToken(env);
  mask(token);
  run(["cloud", "set", "--name", "azurecloud"]);
  run([
    "login",
    "--service-principal",
    "--username",
    env.AZURE_CLIENT_ID,
    "--tenant",
    env.AZURE_TENANT_ID,
    "--federated-token",
    token,
    "--output",
    "none"
  ]);
  run(["account", "set", "--subscription", env.AZURE_SUBSCRIPTION_ID]);
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  login(process.env).catch((error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
}
