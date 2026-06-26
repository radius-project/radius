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
 * Azure scope patterns. Extension resources keep `Extension` scope.
 *
 * Phase 2 refines this using each resource's operation paths for any
 * non-Radius/Azure-style scopes; this mapping covers the Radius namespaces that
 * currently flow through the AutoRest path.
 */
export function armResourceKindToScopeType(kind: ArmResourceKind): ScopeType {
  switch (kind) {
    case "Extension":
      return ScopeType.Extension;
    default:
      return ScopeType.None;
  }
}
