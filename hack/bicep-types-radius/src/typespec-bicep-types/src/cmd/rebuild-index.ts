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
// Standalone CLI for rebuilding the unified Bicep types index. Invoked by the
// `rebuild-bicep-types-index` Make target after the per-namespace types.json
// files (from both the TypeSpec emitter and the Go manifest converter) have been
// written to the generated tree.
//
// Usage: node dist/src/cmd/rebuild-index.js --out-dir <dir> --release-version <version>

import { resolve } from "node:path";
import { buildTypeIndex } from "../index-builder.js";

function getArg(name: string, fallback: string): string {
  const flagIndex = process.argv.indexOf(`--${name}`);
  const value = flagIndex >= 0 ? process.argv[flagIndex + 1] : undefined;
  return value && !value.startsWith("--") ? value : fallback;
}

const outDir = resolve(process.cwd(), getArg("out-dir", "generated"));
const version = getArg("release-version", "latest");

await buildTypeIndex(outDir, version);
