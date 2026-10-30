// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync
} from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { createServer } from "node:http";
import { once } from "node:events";
import test from "node:test";
import {
  getIDToken,
  login,
  requestIDToken,
  runAzure
} from "./azure-oidc-login.mjs";

const token = "header.payload.signature";
const env = {
  ACTIONS_ID_TOKEN_REQUEST_URL:
    "https://example.invalid/idtoken?api-version=2.0",
  ACTIONS_ID_TOKEN_REQUEST_TOKEN: "request-secret",
  AZURE_CLIENT_ID: "client",
  AZURE_TENANT_ID: "tenant",
  AZURE_SUBSCRIPTION_ID: "subscription"
};

function tokenRequest(fetch) {
  return (url, secret) => requestIDToken(url, secret, { fetch });
}

test("requests the Azure audience and authenticates without logging tokens", async () => {
  const warnings = [];
  const result = await getIDToken(env, {
    request: tokenRequest(async (url, options) => {
      assert.equal(url.searchParams.get("api-version"), "2.0");
      assert.equal(
        url.searchParams.get("audience"),
        "api://AzureADTokenExchange"
      );
      assert.equal(options.headers.Authorization, "Bearer request-secret");
      assert.equal(options.redirect, "error");
      return Response.json({ value: token });
    }),
    warn: (message) => warnings.push(message)
  });
  assert.equal(result, token);
  assert.deepEqual(warnings, []);
});

for (const failure of [
  ...[408, 429, 500, 502, 503, 504].map((status) => ({ status })),
  ...[
    "ECONNRESET",
    "ETIMEDOUT",
    "EAI_AGAIN",
    "UND_ERR_CONNECT_TIMEOUT",
    "UND_ERR_HEADERS_TIMEOUT",
    "UND_ERR_BODY_TIMEOUT",
    "UND_ERR_SOCKET"
  ].map((code) => ({ code })),
  { name: "TimeoutError" }
]) {
  test(`retries a transient OIDC failure: ${JSON.stringify(failure)}`, async () => {
    let calls = 0;
    const delays = [];
    const warnings = [];
    const result = await getIDToken(env, {
      request: tokenRequest(async () => {
        calls++;
        if (calls === 1) {
          if (failure.status) {
            return new Response("sensitive response", {
              status: failure.status
            });
          }
          throw Object.assign(new Error("request-secret"), failure);
        }
        return Response.json({ value: token });
      }),
      delay: async (ms) => delays.push(ms),
      warn: (message) => warnings.push(message)
    });
    assert.equal(result, token);
    assert.equal(calls, 2);
    assert.deepEqual(delays, [2000]);
    assert.equal(warnings.length, 1);
    assert.ok(!warnings[0].includes("request-secret"));
    assert.ok(!warnings[0].includes(token));
  });
}

test("stops after three OIDC timeouts with bounded delays", async () => {
  let calls = 0;
  const delays = [];
  await assert.rejects(
    getIDToken(env, {
      request: tokenRequest(async () => {
        calls++;
        throw new DOMException("request-secret", "TimeoutError");
      }),
      delay: async (ms) => delays.push(ms),
      warn: () => {}
    }),
    { message: "GitHub OIDC token request failed." }
  );
  assert.equal(calls, 3);
  assert.deepEqual(delays, [2000, 4000]);
});

test("bounds the token request even when the server does not respond", async () => {
  await assert.rejects(
    requestIDToken(
      new URL(env.ACTIONS_ID_TOKEN_REQUEST_URL),
      env.ACTIONS_ID_TOKEN_REQUEST_TOKEN,
      {
        timeoutMs: 5,
        fetch: async (_url, { signal }) =>
          new Promise((_resolve, reject) => {
            signal.addEventListener("abort", () => reject(signal.reason), {
              once: true
            });
            // Keep the event loop alive while AbortSignal's unreferenced timer runs.
            const timer = setTimeout(
              () => reject(new Error("timeout did not fire")),
              1000
            );
            signal.addEventListener("abort", () => clearTimeout(timer), {
              once: true
            });
          })
      }
    ),
    { message: "GitHub OIDC token request failed.", retryable: true }
  );
});

test("recovers from a real HTTP request timeout before Azure login", async (t) => {
  let calls = 0;
  const server = createServer((request, response) => {
    calls++;
    assert.equal(request.headers.authorization, "Bearer request-secret");
    if (calls > 1) {
      response.setHeader("Content-Type", "application/json");
      response.end(JSON.stringify({ value: token }));
    }
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => {
    server.closeAllConnections();
    server.close();
  });
  const result = await getIDToken(env, {
    request: (url, secret) =>
      requestIDToken(url, secret, {
        timeoutMs: 100,
        fetch: (_url, options) =>
          fetch(`http://127.0.0.1:${server.address().port}`, options)
      }),
    delay: async () => {},
    warn: () => {}
  });
  assert.equal(result, token);
  assert.equal(calls, 2);
});

for (const status of [400, 401, 403, 404]) {
  test(`does not retry permanent HTTP ${status}`, async () => {
    let calls = 0;
    await assert.rejects(
      getIDToken(env, {
        request: tokenRequest(async () => {
          calls++;
          return new Response("request-secret", { status });
        }),
        delay: async () => assert.fail("must not delay")
      }),
      { message: `GitHub OIDC token request returned HTTP ${status}.` }
    );
    assert.equal(calls, 1);
  });
}

for (const body of [
  "not json",
  "{}",
  '{"value":null}',
  '{"value":"bad\\ntoken"}'
]) {
  test(`rejects a malformed response without retry: ${body}`, async () => {
    let calls = 0;
    await assert.rejects(
      getIDToken(env, {
        request: tokenRequest(async () => {
          calls++;
          return new Response(body);
        }),
        delay: async () => assert.fail("must not delay")
      })
    );
    assert.equal(calls, 1);
  });
}

for (const code of [
  "ENOTFOUND",
  "CERT_HAS_EXPIRED",
  "UNABLE_TO_VERIFY_LEAF_SIGNATURE"
]) {
  test(`does not retry a configuration or certificate error: ${code}`, async () => {
    let calls = 0;
    await assert.rejects(
      getIDToken(env, {
        request: tokenRequest(async () => {
          calls++;
          throw new TypeError("request-secret", { cause: { code } });
        }),
        delay: async () => assert.fail("must not delay")
      }),
      { message: "GitHub OIDC token request failed.", retryable: false }
    );
    assert.equal(calls, 1);
  });
}

for (const name of Object.keys(env)) {
  test(`fails before login when ${name} is missing`, async () => {
    const missing = { ...env, [name]: "" };
    await assert.rejects(
      login(missing, {
        run: () => assert.fail("must not run Azure CLI"),
        mask: () => {}
      }),
      new RegExp(name)
    );
  });
}

test("preserves Azure CLI login, cloud selection, and subscription selection", async () => {
  const calls = [];
  const masked = [];
  await login(env, {
    getToken: async () => token,
    run: (args) => calls.push(args),
    mask: (value) => masked.push(value)
  });
  assert.deepEqual(masked, ["client", token]);
  assert.deepEqual(calls, [
    ["cloud", "set", "--name", "azurecloud"],
    [
      "login",
      "--service-principal",
      "--username",
      "client",
      "--tenant",
      "tenant",
      "--federated-token",
      token,
      "--output",
      "none"
    ],
    ["account", "set", "--subscription", "subscription"]
  ]);
});

for (const result of [
  { status: 1 },
  {
    status: null,
    error: new Error(`Command failed: az login --federated-token ${token}`)
  },
  { status: null, signal: "SIGTERM" }
]) {
  test(`does not expose command arguments on CLI failure: ${JSON.stringify(result)}`, () => {
    assert.throws(
      () =>
        runAzure(
          ["login", "--federated-token", token],
          (command, args, options) => {
            assert.equal(command, "az");
            assert.deepEqual(args, ["login", "--federated-token", token]);
            assert.deepEqual(options.stdio, ["ignore", "ignore", "inherit"]);
            return result;
          }
        ),
      {
        message: `Azure CLI login failed (status ${result.status ?? "unavailable"}).`
      }
    );
  });
}

test("accepts a successful CLI exit", () => {
  runAzure(["cloud", "set", "--name", "azurecloud"], () => ({ status: 0 }));
});

for (const failedCommand of ["cloud", "login", "account"]) {
  test(`does not retry Azure ${failedCommand} failures`, async () => {
    const calls = [];
    let tokenCalls = 0;
    await assert.rejects(
      login(env, {
        getToken: async () => {
          tokenCalls++;
          return token;
        },
        run: (args) => {
          calls.push(args[0]);
          if (args[0] === failedCommand) {
            throw new Error("Azure command failed");
          }
        },
        mask: () => {}
      }),
      /Azure command failed/
    );
    assert.equal(tokenCalls, 1);
    assert.equal(
      calls.filter((command) => command === failedCommand).length,
      1
    );
    assert.equal(calls.at(-1), failedCommand);
  });
}

test("workflow keeps OIDC permissions and always clears both Azure sessions", () => {
  const workflow = readFileSync(
    new URL("../workflows/functional-test-cloud.yaml", import.meta.url),
    "utf8"
  );
  assert.equal(
    (workflow.match(/run: node "\$\{AZURE_LOGIN_SCRIPT\}"/g) ?? []).length,
    2
  );
  assert.equal(
    (
      workflow.match(
        /name: Clear Azure session\n        if: always\(\)\n        run: az account clear/g
      ) ?? []
    ).length,
    2
  );
  assert.ok(workflow.includes("id-token: write"));
});

test("both jobs use the workflow script when the PR checkout lacks it", (t) => {
  const workflow = readFileSync(
    new URL("../workflows/functional-test-cloud.yaml", import.meta.url),
    "utf8"
  );
  const root = mkdtempSync(join(tmpdir(), "azure-login-checkout-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  assert.equal(
    existsSync(join(root, ".github", "scripts", "azure-oidc-login.mjs")),
    false
  );

  for (const jobName of ["upload-test-types", "tests"]) {
    const job = workflow.match(
      new RegExp(
        `^  ${jobName}:\\n[\\s\\S]*?(?=^  [a-z][\\w-]*:|$(?![\\s\\S]))`,
        "m"
      )
    )?.[0];
    assert.ok(job, `${jobName} job must exist`);
    const checkout = job.match(
      /      - name: Checkout workflow Azure login script\n[\s\S]*?(?=\n      - )/
    )?.[0];
    assert.ok(checkout, `${jobName} must check out the workflow script`);
    assert.match(checkout, /repository: \$\{\{ github\.repository \}\}/);
    assert.match(checkout, /ref: \$\{\{ github\.sha \}\}/);
    assert.match(
      checkout,
      /sparse-checkout: \.github\/scripts\/azure-oidc-login\.mjs/
    );
    assert.match(checkout, /sparse-checkout-cone-mode: false/);
    assert.match(checkout, /persist-credentials: false/);
    const checkoutPath = checkout.match(/path: (.+)/)?.[1];
    assert.equal(checkoutPath, ".workflow-azure-login");
    const scriptPath = job.match(
      /AZURE_LOGIN_SCRIPT: \$\{\{ github\.workspace \}\}\/(.+)/
    )?.[1];
    assert.equal(
      scriptPath,
      `${checkoutPath}/.github/scripts/azure-oidc-login.mjs`
    );
    const loginStep = job.indexOf('run: node "${AZURE_LOGIN_SCRIPT}"');
    assert.ok(loginStep > job.indexOf(checkout));
    if (jobName === "tests") {
      assert.match(job, /repository: \$\{\{ env\.CHECKOUT_REPO \}\}/);
      assert.match(job, /ref: \$\{\{ env\.CHECKOUT_REF \}\}/);
    } else {
      assert.match(job, /repository: radius-project\/radius/);
      assert.match(job, /ref: \$\{\{ needs\.setup\.outputs\.TRUSTED_SHA \}\}/);
      assert.doesNotMatch(job, /allow-unsafe-pr-checkout/);
    }

    const script = join(root, ...scriptPath.split("/"));
    mkdirSync(dirname(script), { recursive: true });
    copyFileSync(new URL("./azure-oidc-login.mjs", import.meta.url), script);
    const result = spawnSync(process.execPath, [script], {
      cwd: root,
      env: { ...process.env, AZURE_CLIENT_ID: "" },
      encoding: "utf8"
    });
    assert.equal(result.status, 1);
    assert.match(result.stderr, /AZURE_CLIENT_ID is required/);
    assert.doesNotMatch(result.stderr, /MODULE_NOT_FOUND/);
  }
});
