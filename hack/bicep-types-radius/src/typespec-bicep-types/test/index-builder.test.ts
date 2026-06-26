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

import { describe, expect, it } from "vitest";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { buildTypeIndex } from "../src/index-builder.js";

describe("buildTypeIndex", () => {
  it("walks types.json files and writes a unified index.json + index.md", async () => {
    const baseDir = await mkdtemp(join(tmpdir(), "bicep-index-"));
    try {
      const namespaceDir = join(baseDir, "test.ns", "v1");
      await mkdir(namespaceDir, { recursive: true });

      const types = [
        { $type: "ObjectType", name: "Test.Ns/foo", properties: {} },
        {
          $type: "ResourceType",
          name: "Test.Ns/foo@v1",
          body: { $ref: "#/0" },
          readableScopes: 0,
          writableScopes: 0
        }
      ];
      await writeFile(join(namespaceDir, "types.json"), JSON.stringify(types));

      await buildTypeIndex(baseDir, "1.0.0", () => {});

      const index = JSON.parse(
        await readFile(join(baseDir, "index.json"), "utf8")
      );
      // Resources are keyed by the full `Type@version` with a cross-file $ref
      // embedding the forward-slash relative path and the type index.
      expect(index.resources["Test.Ns/foo@v1"]).toBeDefined();
      expect(index.resources["Test.Ns/foo@v1"].$ref).toBe(
        "test.ns/v1/types.json#/1"
      );

      const indexMarkdown = await readFile(join(baseDir, "index.md"), "utf8");
      expect(indexMarkdown).toContain("test.ns/foo");
    } finally {
      await rm(baseDir, { recursive: true, force: true });
    }
  });
});
