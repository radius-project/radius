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
import { ScopeType } from "../src/bicep.js";
import { armResourceKindToScopeType } from "../src/scopes.js";

describe("armResourceKindToScopeType", () => {
  it("maps Radius/UCP resource kinds to ScopeType.None (matching the AutoRest output)", () => {
    expect(armResourceKindToScopeType("Tracked")).toBe(ScopeType.None);
    expect(armResourceKindToScopeType("Proxy")).toBe(ScopeType.None);
    expect(armResourceKindToScopeType("Custom")).toBe(ScopeType.None);
  });

  it("maps Extension resources to ScopeType.Extension", () => {
    expect(armResourceKindToScopeType("Extension")).toBe(ScopeType.Extension);
  });
});
