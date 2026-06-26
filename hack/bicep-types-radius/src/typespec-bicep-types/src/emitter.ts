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

import { EmitContext, emitFile, resolvePath } from "@typespec/compiler";
import {
  ResourceType,
  TypeBaseKind,
  TypeFactory,
  writeMarkdown,
  writeTypesJson,
} from "./bicep.js";
import { discoverResources, DiscoveredResource } from "./resource-discovery.js";
import { buildResourceType, newTranslationCache } from "./type-translator.js";
import { writeTableMarkdown } from "./writers/markdown-table.js";
import type { BicepEmitterOptions } from "./lib.js";

/**
 * Emitter entry point invoked by `tsp compile --emit=@radius-project/typespec-bicep-types`.
 *
 * Discovers the program's ARM resources, builds Bicep extensibility types with
 * the upstream `bicep-types` factory, and writes one `types.json` + `types.md`
 * plus per-resource `docs/*.md` per `namespace/apiVersion` - matching the output
 * layout of the AutoRest path it replaces, with no AutoRest, OpenAPI, or
 * modelerfour in between.
 *
 * The translation (envelope, properties, flatten, `@visibility` flags, `list*`
 * functions, discriminated types) is validated against the real
 * `Applications.Messaging` spec. See
 * `eng/design-notes/tools/2026-06-autorest-bicep-to-typespec-emitter.md`.
 */
export async function $onEmit(
  context: EmitContext<BicepEmitterOptions>,
): Promise<void> {
  if (context.program.compilerOptions.noEmit) {
    return;
  }

  const resources = discoverResources(context.program);
  if (resources.length === 0) {
    return;
  }

  // Group by provider namespace; each namespace/apiVersion is emitted as its own
  // types.json + types.md, mirroring the AutoRest output tree.
  const byNamespace = new Map<string, DiscoveredResource[]>();
  for (const resource of resources) {
    const group = byNamespace.get(resource.namespace) ?? [];
    group.push(resource);
    byNamespace.set(resource.namespace, group);
  }

  for (const [namespace, group] of byNamespace) {
    const factory = new TypeFactory();
    // One cache per namespace so shared types are emitted once and referenced by
    // every resource that uses them (matching the AutoRest output).
    const cache = newTranslationCache();
    for (const resource of group) {
      buildResourceType(context.program, factory, resource, cache);
    }

    const apiVersion = group[0].apiVersion;
    const outFolder = `${namespace}/${apiVersion}`.toLowerCase();

    await emitFile(context.program, {
      path: resolvePath(context.emitterOutputDir, outFolder, "types.json"),
      content: writeTypesJson(factory.types),
    });

    await emitFile(context.program, {
      path: resolvePath(context.emitterOutputDir, outFolder, "types.md"),
      content: writeMarkdown(factory.types, `${namespace} @ ${apiVersion}`),
    });

    // One reference doc per resource type under docs/<resource>.md.
    const resourceTypes = factory.types.filter(
      (type) => type.type === TypeBaseKind.ResourceType,
    ) as ResourceType[];
    for (const resourceType of resourceTypes) {
      const filename = resourceType.name
        .split("/")[1]
        .split("@")[0]
        .toLowerCase();
      await emitFile(context.program, {
        path: resolvePath(
          context.emitterOutputDir,
          outFolder,
          "docs",
          `${filename}.md`,
        ),
        content: writeTableMarkdown([resourceType], factory.types),
      });
    }
  }
}
