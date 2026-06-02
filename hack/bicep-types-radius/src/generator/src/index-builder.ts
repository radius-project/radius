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
// Shared module for building the unified Bicep types index (index.json + index.md).
//
// This walks a directory tree for types.json files and produces a single
// index.json that maps resource type names to their type definitions, plus an
// index.md with human-readable documentation. It is used by:
//
//   - generate.ts: at the end of the autorest pipeline to build the initial index.
//   - rebuild-index.ts: as a standalone step after additional types.json files
//     (e.g. from the Go-based manifest-to-bicep converter) have been added.
//
// Extracting this into a shared module ensures that both callers use the same
// logic and avoids code duplication.

import path from "path";
import { readFile, writeFile } from "fs/promises";
import {
  TypeFile,
  buildIndex,
  readTypesJson,
  writeIndexJson,
  writeIndexMarkdown,
  TypeSettings,
} from "bicep-types";
import { findRecursive, ILogger, logOut } from "./utils";

/**
 * Walks baseDir recursively for types.json files, builds a unified index, and
 * writes index.json + index.md to baseDir.
 *
 * @param logger - Logger instance for diagnostic output.
 * @param baseDir - Root directory containing the generated types tree (e.g. generated/).
 * @param version - Version string reported in the index settings (e.g. "latest" or "v0.58.0").
 */
export async function buildTypeIndex(
  logger: ILogger,
  baseDir: string,
  version: string,
) {
  // Find all types.json files in the generated tree. Each file represents a
  // single namespace/apiVersion combination (e.g. radius.compute/2025-08-01-preview/types.json).
  const typesPaths = await findRecursive(baseDir, (filePath) => {
    return path.basename(filePath) === "types.json";
  });

  // Read each types.json and collect them as TypeFile entries for the index builder.
  const typeFiles: TypeFile[] = [];
  for (const typePath of typesPaths) {
    const content = await readFile(typePath, { encoding: "utf8" });
    typeFiles.push({
      relativePath: path.relative(baseDir, typePath),
      types: readTypesJson(content),
    });
  }

  // Build the unified index from all collected type files.
  const indexContent = await buildIndex(
    typeFiles,
    (log) => logOut(logger, log),
    { name: "Radius", version: version, isSingleton: false } as TypeSettings,
  );

  // Write the index files to the base directory.
  await writeFile(`${baseDir}/index.json`, writeIndexJson(indexContent));
  await writeFile(`${baseDir}/index.md`, writeIndexMarkdown(indexContent));

  logOut(
    logger,
    `Built index from ${typeFiles.length} types.json file(s) in ${baseDir}`,
  );
}
