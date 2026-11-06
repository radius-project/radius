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

import type { ArmResourceKind } from "@azure-tools/typespec-azure-resource-manager";
import { ScopeType } from "./bicep.js";

/**
 * Maps an ARM resource kind to the Bicep extensibility scope.
 *
 * Radius resources are addressed through UCP rather than the Azure
 * subscription/resource-group hierarchy, so they resolve to `ScopeType.None` -
 * which is exactly what the AutoRest path emitted (`readableScopes`/
 * `writableScopes` of `0`) because the Radius routing paths never matched the
 * Azure scope patterns. This includes ARM-style extension resources: marking
 * them as Azure extensions prevents deployment at the Radius resource group.
 * Individual-resource `scope:` needs separate Bicep/deployment-engine support.
 */
export function armResourceKindToScopeType(_kind: ArmResourceKind): ScopeType {
  return ScopeType.None;
}
