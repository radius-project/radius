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

import { BicepType, ResourceType, TypeBaseKind } from "../bicep.js";
import { writeTableMarkdown } from "./markdown-table.js";

/**
 * A single generated reference document: the target file name (without a
 * directory, e.g. `containers.md`) and its markdown content.
 */
export interface ResourceDoc {
  filename: string;
  content: string;
}

/**
 * Builds the per-resource reference docs (`docs/<resource>.md`) for a namespace.
 *
 * For every {@link ResourceType} it derives the file name from the resource
 * name (`Namespace/type@version` -> `type.md`, lowercased) and renders the
 * markdown table with the shared {@link writeTableMarkdown}. This is the single
 * source of truth for the `docs/*.md` layout so the TypeSpec emitter and the
 * contrib (Go manifest) path produce identical output.
 *
 * `describe` supplies the optional resource description rendered above the
 * property tables. The TypeSpec emitter resolves it from `@doc`; the contrib
 * path has no equivalent metadata in `types.json` and omits it.
 */
export function buildResourceDocs(
  resourceTypes: ResourceType[],
  types: BicepType[],
  describe?: (resourceType: ResourceType) => string | undefined
): ResourceDoc[] {
  return resourceTypes.map((resourceType) => ({
    filename: `${resourceDocFilename(resourceType)}.md`,
    content: writeTableMarkdown([resourceType], types, describe?.(resourceType))
  }));
}

/**
 * Derives the doc file stem from a resource name. Bicep resource names are
 * `Namespace/type@version` (and, for nested types, `Namespace/parent/child@version`).
 * Everything after the namespace and before `@` is used, joined with `-`, so
 * nested types get a distinct, collision-free name.
 */
function resourceDocFilename(resourceType: ResourceType): string {
  const [typePath] = resourceType.name.split("@");
  const segments = typePath.split("/");
  if (segments.length < 2) {
    throw new Error(
      `Unexpected resource type name '${resourceType.name}'; expected 'Namespace/type@version'`
    );
  }
  return segments.slice(1).join("-").toLowerCase();
}

/** Returns the {@link ResourceType}s from a flat list of Bicep types. */
export function filterResourceTypes(types: BicepType[]): ResourceType[] {
  return types.filter(
    (type): type is ResourceType => type.type === TypeBaseKind.ResourceType
  );
}
