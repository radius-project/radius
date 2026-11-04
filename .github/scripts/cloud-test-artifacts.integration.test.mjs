// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { mkdtemp, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { test } from "node:test";
import { capture, images, tool } from "./cloud-test-artifacts.mjs";

test(
  "native ORAS captures TLS-local registry data without plaintext or insecure TLS",
  { timeout: 180_000 },
  async (t) => {
    const tools = JSON.parse(
      await tool(["yq", "-o=json", ".", "build/tools.yaml"])
    );
    const version = tools.tools
      .find((item) => item.name === "oras")
      .version.replace(/^v/, "");
    const actual = await tool(["oras", "version"]);
    assert.equal(
      actual.match(/^Version:\s+(\S+)/m)?.[1],
      version,
      "Use the ORAS version pinned in build/tools.yaml"
    );
    const directory = await mkdtemp(join(tmpdir(), "cloud-native-tls-"));
    let id;
    t.after(async () => {
      try {
        if (id) await tool(["docker", "rm", "-f", "-v", id]);
      } finally {
        await rm(directory, { recursive: true, force: true });
      }
    });
    const action = JSON.parse(
      await tool([
        "yq",
        "-o=json",
        ".",
        ".github/actions/create-local-registry/action.yaml"
      ])
    );
    const certificates = action.runs.steps.find(
      (item) => item.name === "Create certificates for local registry"
    );
    await tool(["bash", "-e", "-c", certificates.run], {
      env: {
        ...process.env,
        INPUT_REGISTRY_NAME: "radius-registry",
        INPUT_REGISTRY_SERVER: "localhost",
        STEPS_CREATE_TEMP_CERT_DIR_OUTPUTS_TEMP_CERT_DIR: directory
      }
    });
    const ca = join(directory, "certs/localhost/client.crt");
    const name = `cloud-native-tls-${randomUUID()}`;
    id = (
      await tool([
        "docker",
        "create",
        "--name",
        name,
        "-p",
        "127.0.0.1:5000:5000",
        "-v",
        `${join(directory, "certs/localhost")}:/certs:ro`,
        "-e",
        "REGISTRY_HTTP_TLS_CERTIFICATE=/certs/client.crt",
        "-e",
        "REGISTRY_HTTP_TLS_KEY=/certs/client.key",
        "registry:2"
      ])
    ).trim();
    await tool(["docker", "start", id]);
    // Only retry readiness. A port collision or TLS failure must not be hidden.
    await tool([
      "curl",
      "--fail",
      "--silent",
      "--show-error",
      "--cacert",
      ca,
      "--retry",
      "10",
      "--retry-connrefused",
      "--retry-delay",
      "1",
      "--max-time",
      "2",
      "https://localhost:5000/v2/"
    ]);

    const tag = "test-func0123456789-101-1";
    const recipes = (
      await readdir("test/testrecipes/test-bicep-recipes", { recursive: true })
    )
      .filter(
        (file) => file.endsWith(".bicep") && !basename(file).startsWith("_")
      )
      .map((file) => basename(file, ".bicep"))
      .sort();
    const paths = [
      ...images.map((name) => `images/${name}`),
      "test/radius",
      ...recipes.map((name) => `test/testrecipes/test-bicep-recipes/${name}`)
    ];
    const bytes = Buffer.from("native TLS capture payload\n");
    await writeFile(join(directory, "payload.bin"), bytes);
    const seed = `localhost:5000/${paths[0]}:${tag}`;
    await tool(
      [
        "oras",
        "push",
        "--plain-http=false",
        "--ca-file",
        ca,
        seed,
        "payload.bin"
      ],
      { cwd: directory }
    );
    const rawManifest = await tool([
      "oras",
      "manifest",
      "fetch",
      "--plain-http=false",
      "--ca-file",
      ca,
      seed
    ]);
    const digest = `sha256:${createHash("sha256").update(rawManifest).digest("hex")}`;
    const manifest = JSON.parse(rawManifest);
    for (const path of paths.slice(1)) {
      await tool([
        "oras",
        "cp",
        "--from-plain-http=false",
        "--to-plain-http=false",
        "--from-ca-file",
        ca,
        "--to-ca-file",
        ca,
        seed,
        `localhost:5000/${path}:${tag}`
      ]);
    }
    const context = {
      repo: { owner: "radius-project", repo: "radius" },
      eventName: "merge_group",
      sha: "a".repeat(40),
      runId: 101
    };
    const env = {
      CHECKOUT_REPO: "radius-project/radius",
      CHECKOUT_REF: context.sha,
      UNIQUE_ID: "func0123456789",
      GITHUB_RUN_ATTEMPT: "1",
      REL_VERSION: tag
    };
    const args = { context, core: { getInput: () => directory } };
    const execute = async (command) => {
      // Supply only fixture CA trust; transport selection comes from capture itself.
      return tool(
        command[0] === "oras" ? [...command, "--from-ca-file", ca] : command
      );
    };
    await assert.rejects(
      capture(args, env, (command) =>
        execute(command.filter((arg) => arg !== "--from-plain-http=false"))
      ),
      /http:\/\/localhost:5000|HTTP request to an HTTPS server/
    );
    await capture(args, env, execute);
    const archive = join(directory, "cloud-test-inputs-101-1.tar");
    const index = JSON.parse(
      await tool(["tar", "-xOf", archive, "index.json"])
    );
    assert.equal(index.manifests.length, paths.length);
    for (const root of index.manifests) assert.equal(root.digest, digest);
    assert.equal(
      await tool(["tar", "-xOf", archive, `blobs/sha256/${digest.slice(7)}`]),
      rawManifest
    );
    const blob = await tool([
      "tar",
      "-xOf",
      archive,
      `blobs/sha256/${manifest.layers[0].digest.slice(7)}`
    ]);
    assert.equal(blob, bytes.toString());
    assert.equal(
      `sha256:${createHash("sha256").update(blob).digest("hex")}`,
      manifest.layers[0].digest
    );
    t.diagnostic(
      `ORAS ${version}: plaintext reproduction rejected; ${paths.length} TLS roots exported with matching manifest and payload bytes`
    );
  }
);
