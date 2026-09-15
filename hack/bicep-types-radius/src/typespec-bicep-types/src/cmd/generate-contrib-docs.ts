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
//     --types-json    <namespace>/<apiVersion>/types.json \
//     --out-dir       <namespace>/<apiVersion>/docs \
//     [--descriptions <path to { "<Namespace>/<typeName>": "<description>" } JSON>]

import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { readTypesJson, writeMarkdown } from "../bicep.js";
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
const descriptionsArg = getArg("descriptions");

if (!typesJsonArg || !outDirArg) {
  console.error(
    "Usage: generate-contrib-docs --types-json <path> --out-dir <docsDir>"
  );
  process.exit(1);
}

const typesJsonPath = resolve(process.cwd(), typesJsonArg);
const outDir = resolve(process.cwd(), outDirArg);

// The stale-doc cleanup below removes every *.md in the output directory, so
// require the output to be the `docs/` directory alongside the supplied
// types.json. A looser check (for example, only requiring the directory to be
// named `docs`) would allow an unrelated docs directory to be wiped.
const expectedOutDir = resolve(dirname(typesJsonPath), "docs");
if (outDir !== expectedOutDir) {
  console.error(
    `--out-dir must be the 'docs' directory next to --types-json; got '${outDirArg}' ` +
      `(resolved to '${outDir}'), expected '${expectedOutDir}'.`
  );
  process.exit(1);
}

const types = readTypesJson(
  await readFile(typesJsonPath, { encoding: "utf8" })
);
const resourceTypes = filterResourceTypes(types);

if (resourceTypes.length === 0) {
  console.error(
    `No resource types found in ${typesJsonArg}; refusing to generate empty docs. ` +
      "This usually means the manifest-to-bicep conversion produced no resources."
  );
  process.exit(1);
}

// Resource-level descriptions are authored in the source manifests but cannot be
// carried in types.json: the bicep-types schema exposes `description` only on
// individual object properties, not on ResourceType or ObjectType. The generation
// script extracts them with yq and passes them here, keyed by `<Namespace>/<typeName>`
// without the API version.
const descriptions = new Map<string, string>();
if (descriptionsArg) {
  const parsed: unknown = JSON.parse(
    await readFile(resolve(process.cwd(), descriptionsArg), {
      encoding: "utf8"
    })
  );
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    console.error(
      `--descriptions must contain a JSON object mapping resource type names to descriptions; got ${descriptionsArg}.`
    );
    process.exit(1);
  }
  for (const [name, description] of Object.entries(parsed)) {
    // A non-string value means the manifest authored something unexpected. The Go
    // converter ignores the resource-level description entirely, so nothing upstream
    // catches it; fail here rather than silently publishing a doc without it.
    if (typeof description !== "string") {
      console.error(
        `Description for '${name}' in ${descriptionsArg} must be a string; got ${typeof description}.`
      );
      process.exit(1);
    }
    descriptions.set(name, description);
  }
}

const docs = buildResourceDocs(resourceTypes, types, (resourceType) =>
  descriptions.get(resourceType.name.split("@")[0])
);

// Regenerate from scratch so docs for renamed/removed resources do not linger
// and get republished by the docs-repo consumer.
await mkdir(outDir, { recursive: true });
const stale = (await readdir(outDir)).filter((name) => name.endsWith(".md"));
await Promise.all(stale.map((name) => rm(resolve(outDir, name))));

await Promise.all(
  docs.map((doc) => writeFile(resolve(outDir, doc.filename), doc.content))
);

// The generated index.md links every resource to `types.md#resource-...`, and the
// TypeSpec path writes that file alongside types.json. Emit it here too so the
// contrib namespaces have the same layout and those links resolve.
const [namespace, apiVersion] = splitResourceTypeName(resourceTypes[0].name);
await writeFile(
  resolve(dirname(typesJsonPath), "types.md"),
  writeMarkdown(types, `${namespace} @ ${apiVersion}`)
);

console.log(
  `Generated ${docs.length} reference doc(s) from ${typesJsonArg} into ${outDirArg}`
);

/** Splits `Namespace/type@apiVersion` into its namespace and API version. */
function splitResourceTypeName(name: string): [string, string] {
  const [typePath, apiVersion = ""] = name.split("@");
  return [typePath.split("/")[0], apiVersion];
}
