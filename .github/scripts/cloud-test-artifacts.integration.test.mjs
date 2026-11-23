// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

import assert from "node:assert/strict";
import { createHash, randomUUID } from "node:crypto";
import { once } from "node:events";
import {
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile
} from "node:fs/promises";
import { createServer as createTLSServer } from "node:https";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { test } from "node:test";
import {
  capture,
  destinations,
  images,
  publish,
  sourceIdentity,
  tool
} from "./cloud-test-artifacts.mjs";

test(
  "native publisher bounds manifest visibility reads without repeating uploads",
  { timeout: 30_000 },
  async (t) => {
    const directory = await mkdtemp(join(tmpdir(), "cloud-publish-"));
    t.after(() => rm(directory, { recursive: true, force: true }));
    const hash = (bytes) =>
      `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
    const blob = { digest: hash("{}"), size: 2 };
    const roots = [],
      manifests = new Map();
    for (const name of [
      ...images.map((name) => `images-${name}`),
      "recipes-visibility",
      "types-radius"
    ]) {
      const kind = name.split("-")[0];
      const prefix = `application/vnd.ms.bicep.${kind === "types" ? "provider" : "module"}.`;
      const raw = JSON.stringify({
        schemaVersion: 2,
        mediaType: "application/vnd.oci.image.manifest.v1+json",
        ...(kind === "types" ? { artifactType: `${prefix}artifact` } : {}),
        config: {
          ...blob,
          mediaType:
            kind === "images" ?
              "application/vnd.oci.image.config.v1+json"
            : `${prefix}config.v1+json`
        },
        layers: [
          {
            ...blob,
            mediaType:
              kind === "images" ?
                "application/vnd.oci.image.layer.v1.tar+gzip"
              : `${prefix}layer.v1${kind === "types" ? ".tar+gzip" : "+json"}`
          }
        ]
      });
      const digest = hash(raw);
      roots.push({
        digest,
        size: Buffer.byteLength(raw),
        mediaType: JSON.parse(raw).mediaType,
        annotations: { "org.opencontainers.image.ref.name": name }
      });
      manifests.set(digest, raw);
    }
    const layout = join(directory, "layout");
    await mkdir(join(layout, "blobs/sha256"), { recursive: true });
    for (const [digest, raw] of [...manifests, [blob.digest, "{}"]])
      await writeFile(join(layout, "blobs/sha256", digest.slice(7)), raw);
    await writeFile(
      join(layout, "index.json"),
      JSON.stringify({ schemaVersion: 2, manifests: roots })
    );
    await writeFile(
      join(layout, "oci-layout"),
      JSON.stringify({ imageLayoutVersion: "1.0.0" })
    );
    const registry = "testregistry123.azurecr.io";
    const certificate = join(directory, "certificate.pem"),
      key = join(directory, "key.pem");
    await tool([
      "openssl",
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-days",
      "1",
      "-keyout",
      key,
      "-out",
      certificate,
      "-subj",
      "/CN=ghcr.io",
      "-addext",
      `subjectAltName=DNS:ghcr.io,DNS:${registry}`
    ]);
    const config = join(directory, "config.json");
    await writeFile(config, JSON.stringify({ auths: {} }));
    const stored = new Map(),
      reads = new Map(),
      requests = [];
    let policy;
    const server = createTLSServer(
      {
        key: await readFile(key),
        cert: await readFile(certificate)
      },
      async (request, response) => {
        const path = new URL(request.url, "https://ghcr.io").pathname;
        const match = /^\/v2\/(.+)\/manifests\/([^/]+)$/.exec(path);
        let status = 404,
          body = "";
        response.setHeader("X-Github-Request-Id", "ABCD:1234:5678:9ABC");
        response.setHeader("X-Trace-Secret", "private-fixture-value");
        if (
          request.method === "HEAD" &&
          path.endsWith(`/blobs/${blob.digest}`)
        ) {
          status = 200;
          response.setHeader("Content-Length", blob.size);
          response.setHeader("Docker-Content-Digest", blob.digest);
        } else if (match && request.method === "PUT") {
          const chunks = [];
          for await (const chunk of request) chunks.push(chunk);
          const raw = Buffer.concat(chunks).toString();
          assert.equal(manifests.get(hash(raw)), raw);
          status = policy.putStatus ?? 201;
          if (status === 201) stored.set(match[1], raw);
          response.setHeader("Docker-Content-Digest", hash(raw));
        } else if (match && request.method === "GET" && stored.has(match[1])) {
          const count = (reads.get(path) ?? 0) + 1;
          reads.set(path, count);
          const byDigest = match[2].startsWith("sha256:");
          const failureStatus =
            byDigest ?
              count === 1 ?
                policy.postCopyStatus
              : policy.digestStatus
            : policy.tagStatus;
          if (failureStatus) {
            status = failureStatus;
          } else if (
            count > (byDigest ? policy.digestMisses : policy.tagMisses)
          ) {
            status = 200;
            body =
              (
                (!byDigest && policy.wrongTag) ||
                (byDigest && count > 1 && policy.wrongDigest)
              ) ?
                "{}"
              : stored.get(match[1]);
            response.setHeader(
              "Content-Type",
              "application/vnd.oci.image.manifest.v1+json"
            );
            response.setHeader("Content-Length", Buffer.byteLength(body));
            response.setHeader("Docker-Content-Digest", hash(body));
          }
        }
        requests.push({ method: request.method, path, status });
        response.writeHead(status);
        response.end(body);
      }
    );
    t.after(() => new Promise((resolve) => server.close(resolve)));
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    const run = (command, options) => {
      const host = command.at(-1).split("/")[0];
      assert.ok(
        ["ghcr.io", registry].includes(host),
        "Only fixture hosts may be resolved"
      );
      const prefix = command[1] === "cp" ? "--to-" : "--";
      return tool(
        [
          ...command,
          `${prefix}resolve`,
          `${host}:443:127.0.0.1:${server.address().port}`,
          `${prefix}ca-file`,
          certificate,
          `${prefix}registry-config`,
          config
        ],
        {
          ...options,
          env: { ...process.env, NO_PROXY: "*", no_proxy: "*" }
        }
      );
    };
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
      REL_VERSION: "test-func0123456789-101-1",
      CONTAINER_REGISTRY: "ghcr.io/radius-project/dev",
      BICEP_RECIPE_REGISTRY: "ghcr.io/radius-project/dev",
      TEST_BICEP_TYPES_REGISTRY: registry
    };
    const first = destinations(roots, env.REL_VERSION, env)[0];
    const tagPath = `/v2/radius-project/dev/ucpd/manifests/${env.REL_VERSION}`;
    const digestPath = `/v2/radius-project/dev/ucpd/manifests/${first.digest}`;
    for (const boundary of ["none", "post-copy", "tag"]) {
      await t.test(`raw ORAS control: ${boundary}`, async () => {
        policy = {
          digestMisses: boundary === "post-copy" ? 1 : 0,
          tagMisses: boundary === "tag" ? 1 : 0
        };
        stored.clear();
        reads.clear();
        requests.length = 0;
        const copy = run([
          "oras",
          "cp",
          "--debug",
          "--from-oci-layout",
          `${layout}@${first.digest}`,
          first.reference
        ]);
        if (boundary === "post-copy") {
          await assert.rejects(copy, (error) => {
            assert.equal(error.code, 1);
            assert.ok(error.stderr.includes(`${first.digest}: not found`));
            return true;
          });
        } else {
          await copy;
        }
        assert.deepEqual(requests.slice(-2), [
          { method: "PUT", path: tagPath, status: 201 },
          {
            method: "GET",
            path: digestPath,
            status: boundary === "post-copy" ? 404 : 200
          }
        ]);
        const fetch = () => run(["oras", "manifest", "fetch", first.reference]);
        if (boundary === "tag") {
          await assert.rejects(fetch(), (error) => {
            assert.equal(error.code, 1);
            assert.ok(error.stderr.includes("not found"));
            return true;
          });
          assert.deepEqual(requests.at(-1), {
            method: "GET",
            path: tagPath,
            status: 404
          });
        }
        assert.equal(await fetch(), manifests.get(first.digest));
        assert.equal(
          await run([
            "oras",
            "manifest",
            "fetch",
            `${first.reference.slice(0, first.reference.lastIndexOf(":"))}@${first.digest}`
          ]),
          manifests.get(first.digest)
        );
        assert.equal(
          requests.filter(({ method }) => method === "PUT").length,
          1
        );
      });
    }
    for (const scenario of [
      { name: "immediate visibility", digestMisses: 0, tagMisses: 0 },
      { name: "post-copy digest visibility", digestMisses: 2, tagMisses: 1 },
      { name: "explicit tag visibility", digestMisses: 0, tagMisses: 2 },
      {
        name: "private types visibility",
        digestMisses: 2,
        tagMisses: 1,
        destination: "acr"
      },
      {
        name: "permanent digest absence",
        digestMisses: Infinity,
        tagMisses: 0,
        error: /digest-readback.*not visible after 5 attempts/
      },
      {
        name: "permanent tag absence",
        digestMisses: 0,
        tagMisses: Infinity,
        error: /tag-readback.*not visible after 5 attempts/
      },
      {
        name: "wrong tag digest",
        digestMisses: 0,
        tagMisses: 0,
        wrongTag: true,
        error: /Published digest changed/
      },
      {
        name: "wrong digest readback",
        digestMisses: 0,
        tagMisses: 0,
        wrongDigest: true,
        error: /Published digest changed|Registry digest-readback failed/
      },
      {
        name: "rejected manifest PUT",
        putStatus: 400,
        error: /Registry copy failed/
      },
      ...[401, 403].flatMap((status) =>
        ["putStatus", "postCopyStatus", "digestStatus", "tagStatus"].map(
          (phase) => ({
            name: `${phase} authorization failure ${status}`,
            digestMisses: 0,
            tagMisses: 0,
            [phase]: status,
            error: /Registry .* failed/
          })
        )
      )
    ]) {
      await t.test(scenario.name, async () => {
        policy = scenario;
        stored.clear();
        reads.clear();
        requests.length = 0;
        const receipt = {
          source: sourceIdentity(context, env),
          tag: env.REL_VERSION,
          roots,
          published: []
        };
        const receiptPath = join(directory, "receipt.json");
        await writeFile(receiptPath, JSON.stringify(receipt));
        const messages = [],
          delays = [];
        const destination = scenario.destination ?? "ghcr";
        const core = {
          getInput: (name) => ({ directory, destination })[name],
          info: (message) => messages.push(message),
          summary: {
            addCodeBlock() {
              return this;
            },
            async write() {}
          }
        };
        const work = publish(
          {
            core,
            context,
            sleep: async (delay) => {
              delays.push(delay);
            }
          },
          env,
          run
        );
        if (scenario.error) await assert.rejects(work, scenario.error);
        else await work;
        const published = JSON.parse(await readFile(receiptPath)).published;
        const expected = destinations(roots, env.REL_VERSION, env).filter(
          (entry) => (entry.kind === "types") === (destination === "acr")
        );
        assert.equal(published.length, scenario.error ? 0 : expected.length);
        const puts = requests.filter(({ method }) => method === "PUT");
        assert.equal(puts.length, scenario.error ? 1 : expected.length);
        assert.equal(new Set(puts.map(({ path }) => path)).size, puts.length);
        if (
          scenario.digestMisses === Infinity ||
          scenario.tagMisses === Infinity
        )
          assert.deepEqual(delays, [1000, 2000, 4000, 8000]);
        else if (scenario.error) assert.deepEqual(delays, []);
        else
          assert.deepEqual(
            delays,
            Array.from({ length: expected.length }, () =>
              scenario.digestMisses === 2 ? [1000, 1000]
              : scenario.tagMisses === 2 ? [1000, 2000]
              : []
            ).flat()
          );
        if (scenario.error) {
          assert.equal(
            reads.get(digestPath) ?? 0,
            scenario.putStatus ? 0
            : scenario.postCopyStatus ? 1
            : scenario.digestMisses === Infinity ? 6
            : 2
          );
          assert.equal(
            reads.get(tagPath) ?? 0,
            scenario.tagMisses === Infinity ? 5
            : scenario.wrongTag || scenario.tagStatus ? 1
            : 0
          );
        } else {
          for (const entry of expected) {
            const repository = entry.reference.slice(
              entry.reference.indexOf("/") + 1,
              entry.reference.lastIndexOf(":")
            );
            const prefix = `/v2/${repository}/manifests`;
            assert.equal(
              reads.get(`${prefix}/${entry.digest}`),
              2 + Math.max(0, scenario.digestMisses - 1)
            );
            assert.equal(
              reads.get(`${prefix}/${env.REL_VERSION}`),
              1 + scenario.tagMisses
            );
          }
        }
        assert.doesNotMatch(
          messages.join("\n"),
          /private-fixture-value|Response headers|Request URL/
        );
        assert.ok(
          messages.some((message) =>
            message.includes('"x-github-request-id":"ABCD:1234:5678:9ABC"')
          )
        );
        if (scenario.digestMisses > 0)
          assert.ok(
            messages.some((message) =>
              message.startsWith("ORAS post-copy destination read failed")
            )
          );
      });
    }
  }
);

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
      REL_VERSION: tag,
      TEST_BICEP_TYPES_REGISTRY: "testregistry123.azurecr.io"
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
