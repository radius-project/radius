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
import { $lib } from "../src/lib.js";

// Phase 0 smoke tests: validate the library is wired correctly. The Phase 1
// work replaces these with golden-file tests that compile a real ARM spec
// (starting with Applications.Messaging) through the TypeSpec test host and
// diff the emitted types.json / types.md / docs against the committed baselines.
describe("@radius-project/typespec-bicep-types library", () => {
  it("declares a library name matching the package name", () => {
    expect($lib.name).toBe("@radius-project/typespec-bicep-types");
  });

  it("registers an emitter options schema", () => {
    expect($lib.emitter?.options).toBeDefined();
  });
});
