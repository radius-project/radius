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

import { readFile, readdir, writeFile } from "node:fs/promises";
import { join, relative, resolve } from "node:path";
import {
  buildIndex,
  readTypesJson,
  writeIndexJson,
  writeIndexMarkdown,
  type TypeFile,
  type TypeSettings
} from "./bicep.js";

/**
 * Builds the unified Bicep types index (`index.json` + `index.md`) by walking
 * `baseDir` for every `types.json`, merging them via the upstream `bicep-types`
 * `buildIndex`, and writing the index files back to `baseDir`.
 *
 * This is the AutoRest-free index step, relocated from the legacy `generator`
 * package so that package can be removed. It is run after both the TypeSpec
 * emitter and the Go manifest converter have written their per-namespace
 * `types.json` into the generated tree, so the index spans both paths.
 */
export async function buildTypeIndex(
  baseDir: string,
  version: string,
  log: (message: string) => void = (message) => console.log(message)
): Promise<void> {
  // A single recursive `readdir` reports every entry with its file/directory
  // type, so no per-entry `stat` is needed. `buildIndex` sorts the merged
  // resources by key, so traversal order does not affect the output.
  const entries = await readdir(baseDir, {
    recursive: true,
    withFileTypes: true
  });
  const typesPaths = entries
    .filter((entry) => entry.isFile() && entry.name === "types.json")
    .map((entry) => join(entry.parentPath, entry.name));

  const typeFiles: TypeFile[] = await Promise.all(
    typesPaths.map(async (typePath) => ({
      // Normalize to forward slashes so the index is identical across platforms.
      relativePath: relative(baseDir, typePath).replaceAll("\\", "/"),
      types: readTypesJson(await readFile(typePath, { encoding: "utf8" }))
    }))
  );

  const index = buildIndex(typeFiles, log, {
    name: "Radius",
    version,
    isSingleton: false
  } as TypeSettings);

  await writeFile(resolve(baseDir, "index.json"), writeIndexJson(index));
  await writeFile(resolve(baseDir, "index.md"), writeIndexMarkdown(index));

  log(`Built index from ${typeFiles.length} types.json file(s) in ${baseDir}`);
}
