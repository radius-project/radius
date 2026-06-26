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

import {
  createObjectProperty,
  ObjectTypeProperty,
  ObjectTypePropertyFlags,
  TypeFactory,
  TypeReference,
} from "./bicep.js";

/**
 * Builds the standardized resource envelope (`id`, `name`, `type`,
 * `apiVersion`) shared by every Bicep resource type. Ported verbatim from the
 * AutoRest extension's `getStandardizedResourceProperties` so the property
 * order, flags, and descriptions match the existing golden files exactly:
 *
 * - `id`        - ReadOnly | DeployTimeConstant (flags 10)
 * - `name`      - Required | DeployTimeConstant | Identifier (flags 25)
 * - `type`      - ReadOnly | DeployTimeConstant (flags 10)
 * - `apiVersion`- ReadOnly | DeployTimeConstant (flags 10)
 *
 * @param resourceName - The type reference for the resource name property
 *   (a plain string today; Phase 2 ports the full name-parameter parsing).
 */
export function getStandardizedResourceProperties(
  factory: TypeFactory,
  fullyQualifiedType: string,
  apiVersion: string,
  resourceName: TypeReference,
): Record<string, ObjectTypeProperty> {
  const typeLiteral = factory.addStringLiteralType(fullyQualifiedType);

  return {
    id: createObjectProperty(
      factory.addStringType(),
      ObjectTypePropertyFlags.ReadOnly |
        ObjectTypePropertyFlags.DeployTimeConstant,
      "The resource id",
    ),
    name: createObjectProperty(
      resourceName,
      ObjectTypePropertyFlags.Required |
        ObjectTypePropertyFlags.DeployTimeConstant |
        ObjectTypePropertyFlags.Identifier,
      "The resource name",
    ),
    type: createObjectProperty(
      typeLiteral,
      ObjectTypePropertyFlags.ReadOnly |
        ObjectTypePropertyFlags.DeployTimeConstant,
      "The resource type",
    ),
    apiVersion: createObjectProperty(
      factory.addStringLiteralType(apiVersion),
      ObjectTypePropertyFlags.ReadOnly |
        ObjectTypePropertyFlags.DeployTimeConstant,
      "The resource api version",
    ),
  };
}
