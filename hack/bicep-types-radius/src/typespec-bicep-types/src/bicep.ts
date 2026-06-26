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

// Single import seam for the upstream `bicep-types` library (the same serializer
// the AutoRest path uses). Centralizing it here keeps the CommonJS/ESM interop in
// one place and gives the rest of the emitter a stable surface to import from.

export {
  TypeFactory,
  TypeReference,
  ScopeType,
  ObjectTypePropertyFlags,
  TypeBaseKind,
  writeTypesJson,
  readTypesJson,
  writeMarkdown,
  buildIndex,
  writeIndexJson,
  writeIndexMarkdown
} from "bicep-types";

export type {
  BicepType,
  ArrayType,
  DiscriminatedObjectType,
  FunctionParameter,
  ObjectType,
  ObjectTypeProperty,
  ResourceType,
  ResourceTypeFunction,
  StringLiteralType,
  TypeFile,
  TypeIndex,
  TypeSettings,
  UnionType
} from "bicep-types";

import type {
  ObjectTypeProperty,
  ObjectTypePropertyFlags,
  TypeReference
} from "bicep-types";

/**
 * Builds an {@link ObjectTypeProperty}. The upstream library does not export a
 * helper for this, and the AutoRest extension used a local one - this keeps the
 * port faithful and the call sites readable.
 */
export function createObjectProperty(
  type: TypeReference,
  flags: ObjectTypePropertyFlags,
  description?: string
): ObjectTypeProperty {
  return { type, flags, description };
}
