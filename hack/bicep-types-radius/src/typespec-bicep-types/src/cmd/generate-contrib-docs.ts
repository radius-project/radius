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
// Standalone CLI that generates per-resource reference docs (`docs/<resource>.md`)
// for a contrib namespace produced by the Go `manifest-to-bicep generate`
// command. That command emits only `types.json`; this reuses the shared
// `writeTableMarkdown()` markdown so contrib namespaces (Radius.Compute,
// Radius.Data, Radius.Security, ...) get the same reference docs as the
// TypeSpec-generated core namespaces, without reimplementing the renderer in Go.
//
// Usage:
//   node dist/src/cmd/generate-contrib-docs.js \
//     --types-json <namespace>/<apiVersion>/types.json \
//     --out-dir    <namespace>/<apiVersion>/docs

import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { readTypesJson } from "../bicep.js";
import {
  buildResourceDocs,
  filterResourceTypes
} from "../writers/resource-docs.js";

function getArg(name: string): string | undefined {
  const flagIndex = process.argv.indexOf(`--${name}`);
  const value = flagIndex >= 0 ? process.argv[flagIndex + 1] : undefined;
  return value && !value.startsWith("--") ? value : undefined;
}

const typesJsonArg = getArg("types-json");
const outDirArg = getArg("out-dir");

if (!typesJsonArg || !outDirArg) {
  console.error(
    "Usage: generate-contrib-docs --types-json <path> --out-dir <docsDir>"
  );
  process.exit(1);
}

const typesJsonPath = resolve(process.cwd(), typesJsonArg);
const outDir = resolve(process.cwd(), outDirArg);

const types = readTypesJson(await readFile(typesJsonPath, { encoding: "utf8" }));
const resourceTypes = filterResourceTypes(types);

if (resourceTypes.length === 0) {
  console.error(
    `No resource types found in ${typesJsonArg}; refusing to generate empty docs. ` +
      "This usually means the manifest-to-bicep conversion produced no resources."
  );
  process.exit(1);
}

const docs = buildResourceDocs(resourceTypes, types);

// Regenerate from scratch so docs for renamed/removed resources do not linger
// and get republished by the docs-repo consumer.
await mkdir(outDir, { recursive: true });
const stale = (await readdir(outDir)).filter((name) => name.endsWith(".md"));
await Promise.all(stale.map((name) => rm(resolve(outDir, name))));

await Promise.all(
  docs.map((doc) => writeFile(resolve(outDir, doc.filename), doc.content))
);

console.log(
  `Generated ${docs.length} reference doc(s) from ${typesJsonArg} into ${outDirArg}`
);
