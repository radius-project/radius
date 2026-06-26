// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.
//
// Standalone driver that compiles the Radius TypeSpec projects with this Bicep
// emitter, producing the `generated/<base>/<namespace>/<version>/` tree that the
// deprecated AutoRest path (`autorest.bicep` + `generator`) used to produce.
//
// Why a programmatic driver instead of `tsp compile --emit`?
//   * The project `tspconfig.yaml` files emit `@azure-tools/typespec-autorest`
//     (OpenAPI for the REST contract). Driving Bicep emission separately keeps
//     the two outputs cleanly decoupled and avoids perturbing the OpenAPI step.
//   * It loads the HOST workspace's `@typespec/compiler` (the same instance the
//     emitter resolves once linked into `typespec/node_modules`), guaranteeing a
//     single compiler instance - TypeSpec rejects two.
//
// Linking model (deterministic, pnpm-version independent): the emitter's built
// `dist` + `package.json` are copied into `<typespec>/node_modules` without a
// sibling `@typespec/compiler`, so the emitter resolves the host compiler by
// walking up `node_modules`. The only runtime dependency (`bicep-types`) is
// copied alongside. The copies are removed on exit.
//
// Usage:
//   node dist/src/cmd/compile-projects.js \
//     --typespec-dir <path to typespec/> \
//     --out-dir <path to hack/bicep-types-radius/generated>

import { createRequire } from "node:module";
import { cp, mkdir, rm } from "node:fs/promises";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const EMITTER_NAME = "@radius-project/typespec-bicep-types";

/**
 * The TypeSpec projects whose Bicep types were produced by the AutoRest path,
 * mapped to the base directory under `generated/` that holds their output. The
 * emitter writes `<namespace>/<apiVersion>/` beneath that base, mirroring the
 * AutoRest output tree (`generated/applications/...`, `generated/radius/...`).
 */
const PROJECTS: ReadonlyArray<{ projectDir: string; outputBase: string }> = [
  { projectDir: "Applications.Core", outputBase: "applications" },
  { projectDir: "Applications.Dapr", outputBase: "applications" },
  { projectDir: "Applications.Datastores", outputBase: "applications" },
  { projectDir: "Applications.Messaging", outputBase: "applications" },
  { projectDir: "Radius.Core", outputBase: "radius" },
];

function getArg(name: string, fallback: string): string {
  const flagIndex = process.argv.indexOf(`--${name}`);
  const value = flagIndex >= 0 ? process.argv[flagIndex + 1] : undefined;
  return value && !value.startsWith("--") ? value : fallback;
}

const require = createRequire(import.meta.url);

// dist/src/cmd/compile-projects.js -> package root.
const packageRoot = fileURLToPath(new URL("../../..", import.meta.url));

const typespecDir = resolve(process.cwd(), getArg("typespec-dir", "typespec"));
const generatedDir = resolve(
  process.cwd(),
  getArg("out-dir", "hack/bicep-types-radius/generated"),
);
const keepLink = process.argv.includes("--keep-link");

/**
 * Copies the built emitter and its single runtime dependency into the host
 * workspace's `node_modules` so the TypeSpec compiler can resolve the emitter by
 * name while sharing the host's compiler instance. Returns the paths to remove.
 */
async function linkEmitter(): Promise<string[]> {
  const emitterDest = join(
    typespecDir,
    "node_modules",
    ...EMITTER_NAME.split("/"),
  );
  // `bicep-types` is a direct dependency, so pnpm always exposes it at the
  // package's own `node_modules/bicep-types` (its `exports` map hides
  // `package.json`, so resolve the directory directly rather than via require).
  const bicepTypesSrc = join(packageRoot, "node_modules", "bicep-types");
  const bicepTypesDest = join(typespecDir, "node_modules", "bicep-types");

  await rm(emitterDest, { recursive: true, force: true });
  await mkdir(emitterDest, { recursive: true });
  await cp(join(packageRoot, "dist"), join(emitterDest, "dist"), {
    recursive: true,
  });
  await cp(
    join(packageRoot, "package.json"),
    join(emitterDest, "package.json"),
  );

  await rm(bicepTypesDest, { recursive: true, force: true });
  await cp(bicepTypesSrc, bicepTypesDest, {
    recursive: true,
    dereference: true,
  });

  return [emitterDest, bicepTypesDest];
}

async function main(): Promise<void> {
  const links = await linkEmitter();
  try {
    // Load the host workspace's compiler so the single shared instance the
    // emitter resolves is the same one driving compilation.
    const compilerPath = require.resolve("@typespec/compiler", {
      paths: [typespecDir],
    });
    const { NodeHost, compile } = (await import(
      pathToFileURL(compilerPath).href
    )) as typeof import("@typespec/compiler");

    let hadError = false;
    for (const { projectDir, outputBase } of PROJECTS) {
      const entry = join(typespecDir, projectDir);
      const outputDir = join(generatedDir, outputBase);

      const program = await compile(NodeHost, entry, {
        outputDir,
        emit: [EMITTER_NAME],
        options: { [EMITTER_NAME]: { "emitter-output-dir": outputDir } },
      });

      const errors = program.diagnostics.filter((d) => d.severity === "error");
      if (errors.length > 0) {
        hadError = true;
        console.error(`\n${projectDir}: ${errors.length} error(s)`);
        for (const d of errors) {
          console.error(`  [${d.code}] ${d.message}`);
        }
      } else {
        console.log(`${projectDir} -> ${join(outputBase)} (ok)`);
      }
    }

    if (hadError) {
      process.exitCode = 1;
    }
  } finally {
    if (!keepLink) {
      await Promise.all(
        links.map((p) => rm(p, { recursive: true, force: true })),
      );
    }
  }
}

await main();
