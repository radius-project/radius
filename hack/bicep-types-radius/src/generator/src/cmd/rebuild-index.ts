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
// Standalone CLI entry point for rebuilding the unified Bicep types index.
//
// This is a thin wrapper around the shared buildTypeIndex() function from
// index-builder.ts. It exists as a separate entry point so the Makefile can
// invoke it independently after the Go-based manifest-to-bicep converter has
// added contrib types.json files to the generated/ tree.
//
// Usage:
//   pnpm run rebuild-index --release-version <version>

import path from "path";
import yargs from "yargs/yargs";
import { hideBin } from "yargs/helpers";
import { buildTypeIndex } from "../index-builder";
import { defaultLogger, executeSynchronous } from "../utils";

const rootDir = `${__dirname}/../../../../`;
const defaultOutDir = path.resolve(`${rootDir}/generated`);

const argsConfig = yargs(hideBin(process.argv))
  .strict()
  .option("out-dir", {
    type: "string",
    default: defaultOutDir,
    desc: "Output path containing previously generated types.json files",
  })
  .option("release-version", {
    type: "string",
    default: "latest",
    desc: "The version reported in the index settings",
  });

executeSynchronous(async () => {
  const args = await argsConfig.parseAsync();
  const outputBaseDir = path.resolve(args["out-dir"]);
  const version = args["release-version"];

  await buildTypeIndex(defaultLogger, outputBaseDir, version);
});
