// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import {
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { capture, prepare, tool } from "./bicep-types.mjs";

const root = fileURLToPath(new URL("../../", import.meta.url));
const generated = join(root, "hack/bicep-types-radius/generated");
const hash = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;

test(
  "pinned Bicep capture round-trips through native ORAS preparation",
  { timeout: 180_000 },
  async (t) => {
    const tools = JSON.parse(
      await tool(["yq", "-o=json", ".", join(root, "build/tools.yaml")])
    ).tools;
    const bicepVersion = tools
      .find((item) => item.name === "bicep")
      .version.slice(1);
    const orasVersion = tools
      .find((item) => item.name === "oras")
      .version.slice(1);
    assert.equal(
      (await tool(["bicep", "--version"])).match(
        /Bicep CLI version (\S+)/
      )?.[1],
      bicepVersion,
      "Use the Bicep version pinned in build/tools.yaml"
    );
    assert.equal(
      (await tool(["oras", "version"])).match(/^Version:\s+(\S+)/m)?.[1],
      orasVersion,
      "Use the ORAS version pinned in build/tools.yaml"
    );
    const directory = await mkdtemp(join(tmpdir(), "bicep-native-roundtrip-"));
    const previousDockerConfig = process.env.DOCKER_CONFIG;
    let container;
    t.after(async () => {
      if (previousDockerConfig === undefined) delete process.env.DOCKER_CONFIG;
      else process.env.DOCKER_CONFIG = previousDockerConfig;
      try {
        if (container) await tool(["docker", "rm", "-f", "-v", container]);
      } finally {
        await rm(directory, { recursive: true, force: true });
      }
    });
    const dockerConfig = join(directory, "docker");
    await mkdir(dockerConfig);
    await writeFile(join(dockerConfig, "config.json"), '{"auths":{}}');
    const workflow = JSON.parse(
      await tool([
        "yq",
        "-o=json",
        ".",
        join(root, ".github/workflows/__publish-bicep-types.yaml")
      ])
    );
    container = (
      await tool([
        "docker",
        "create",
        "--name",
        `bicep-native-roundtrip-${randomUUID()}`,
        "-p",
        "127.0.0.1::5000",
        workflow.jobs.bundle.services.registry.image
      ])
    ).trim();
    await tool(["docker", "start", container]);
    const binding = (
      await tool(["docker", "port", container, "5000/tcp"])
    ).trim();
    assert.match(binding, /^127\.0\.0\.1:[0-9]+$/);
    const registry = `localhost:${binding.split(":")[1]}`;
    await tool([
      "curl",
      "--fail",
      "--silent",
      "--show-error",
      "--retry",
      "10",
      "--retry-connrefused",
      "--retry-delay",
      "1",
      "--max-time",
      "2",
      `http://${registry}/v2/`
    ]);
    process.env.DOCKER_CONFIG = dockerConfig;

    // Keep the same absolute generated index; only the caller's OCI setting differs.
    await writeFile(
      join(directory, "bicepconfig.json"),
      '{"experimentalFeaturesEnabled":{"ociEnabled":false}}'
    );
    await assert.rejects(
      tool(
        [
          "bicep",
          "publish-extension",
          join(generated, "index.json"),
          "--target",
          `br:${registry}/radius:without-oci`
        ],
        {
          cwd: directory,
          env: { ...process.env, BICEP_TRUSTED_REGISTRIES: "localhost" },
          timeout: 30_000
        }
      ),
      (error) => {
        assert.notEqual(
          error.killed,
          true,
          "The control must fail, not time out"
        );
        assert.match(error.stderr, /SSL|TLS|HTTPS|https:/);
        t.diagnostic(
          `OCI-disabled control: ${error.stderr.split("\n").find((line) => /SSL|TLS|HTTPS|https:/.test(line))}`
        );
        return true;
      }
    );

    const source = {
      repository: "radius-project/radius",
      ref: "refs/heads/main",
      commit: "a".repeat(40),
      workflow: ".github/workflows/build-main.yaml",
      runId: 123,
      generationAttempt: 1
    };
    const captured = join(directory, "captured");
    await mkdir(captured);
    await capture(source, captured, registry);
    const archive = join(captured, "bicep-types-radius-123.tar");
    const archiveDigest = hash(await readFile(archive));
    const metadata = JSON.parse(
      await tool(["tar", "-xOf", archive, "source.json"])
    );
    assert.deepEqual(metadata.source, source);
    const index = JSON.parse(
      await tool(["tar", "-xOf", archive, "index.json"])
    );
    assert.equal(index.manifests.length, 1);
    const originalBytes = await tool(
      ["oras", "manifest", "fetch", "--oci-layout", `${archive}:bundle`],
      { encoding: "buffer" }
    );
    assert.equal(hash(originalBytes), metadata.manifestDigest);
    assert.equal(index.manifests[0].digest, metadata.manifestDigest);
    assert.equal(index.manifests[0].size, originalBytes.length);
    assert.deepEqual(
      originalBytes,
      await tool(
        [
          "oras",
          "manifest",
          "fetch",
          "--plain-http",
          `${registry}/radius:bundle`
        ],
        { encoding: "buffer" }
      )
    );
    const original = JSON.parse(originalBytes);
    const provider = "application/vnd.ms.bicep.provider.";
    assert.equal(original.schemaVersion, 2);
    assert.equal(
      original.mediaType,
      "application/vnd.oci.image.manifest.v1+json"
    );
    assert.equal(original.artifactType, `${provider}artifact`);
    assert.equal(original.annotations["bicep.serialization.format"], "v1");
    assert.equal(original.config.mediaType, `${provider}config.v1+json`);
    assert.equal(original.layers.length, 1);
    assert.equal(original.layers[0].mediaType, `${provider}layer.v1.tar+gzip`);
    const blobs = new Map();
    for (const descriptor of [original.config, ...original.layers]) {
      const path = `blobs/sha256/${descriptor.digest.slice(7)}`;
      const bytes = await tool(["tar", "-xOf", archive, path], {
        encoding: "buffer"
      });
      assert.equal(bytes.length, descriptor.size);
      assert.equal(hash(bytes), descriptor.digest);
      blobs.set(path, bytes);
    }
    assert.deepEqual(
      blobs.get(`blobs/sha256/${original.config.digest.slice(7)}`),
      Buffer.from("{}")
    );
    const typesArchive = join(directory, "types.tgz");
    await writeFile(
      typesArchive,
      blobs.get(`blobs/sha256/${original.layers[0].digest.slice(7)}`)
    );
    const typeFiles = (await readdir(generated, { recursive: true }))
      .filter((file) => file.endsWith("/types.json"))
      .sort();
    assert.ok(typeFiles.length > 0);
    for (const file of ["index.json", ...typeFiles]) {
      assert.deepEqual(
        JSON.parse(await tool(["tar", "-xOf", typesArchive, file])),
        JSON.parse(await readFile(join(generated, file), "utf8")),
        `Native Bicep packaging must retain ${file}`
      );
    }

    let finalDigest;
    for (const attempt of [1, 2]) {
      const output = join(directory, `prepared-${attempt}`);
      await mkdir(output);
      const receipt = await prepare(
        archive,
        { ...source, generationAttempt: attempt },
        output
      );
      assert.deepEqual(receipt.source, source);
      assert.equal(receipt.publicationAttempt, attempt);
      const stamped = await readFile(join(output, "manifest.json"));
      assert.equal(hash(stamped), receipt.manifestDigest);
      assert.deepEqual(JSON.parse(stamped), {
        ...original,
        annotations: {
          ...original.annotations,
          "org.opencontainers.image.source":
            "https://github.com/radius-project/radius",
          "org.opencontainers.image.revision": source.commit
        }
      });
      assert.deepEqual(
        await tool(
          [
            "oras",
            "manifest",
            "fetch",
            "--oci-layout",
            `${output}/layout@${receipt.manifestDigest}`
          ],
          { encoding: "buffer" }
        ),
        stamped
      );
      for (const [path, bytes] of blobs) {
        assert.deepEqual(await readFile(join(output, "layout", path)), bytes);
      }
      if (finalDigest) assert.equal(receipt.manifestDigest, finalDigest);
      finalDigest = receipt.manifestDigest;
    }
    assert.equal(hash(await readFile(archive)), archiveDigest);
    t.diagnostic(
      `Bicep ${bicepVersion}, ORAS ${orasVersion}: OCI-disabled control rejected; ` +
        `actual capture/prepare retained index + ${typeFiles.length} type documents, config/layer bytes, ` +
        `and generation attempt 1 across publication attempts 1/2; native ${metadata.manifestDigest}, stamped ${finalDigest}`
    );
  }
);
