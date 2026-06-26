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
import { createTestHost } from "@typespec/compiler/testing";
import { OpenAPITestLibrary } from "@typespec/openapi/testing";
import type { Model, Type } from "@typespec/compiler";
import {
  type BicepType,
  ObjectTypePropertyFlags,
  TypeBaseKind,
  TypeFactory,
  TypeReference,
} from "../src/bicep.js";
import { translateModelProperties } from "../src/type-translator.js";

/** Compiles a fixture (with `@typespec/openapi` available) and translates `Target`. */
async function translateTarget(
  code: string,
): Promise<Record<string, { flags: number; bicep: BicepType }>> {
  const host = await createTestHost({ libraries: [OpenAPITestLibrary] });
  host.addTypeSpecFile("main.tsp", code);
  const result = (await host.compile("main.tsp")) as Record<string, Type>;
  const target = result.Target as Model;

  const factory = new TypeFactory();
  const translated = translateModelProperties(
    host.program,
    factory,
    target,
    new Map<Type, TypeReference>(),
  );

  const props: Record<string, { flags: number; bicep: BicepType }> = {};
  for (const [name, property] of Object.entries(translated)) {
    props[name] = {
      flags: property.flags,
      bicep: factory.lookupType(property.type),
    };
  }
  return props;
}

describe("properties-bag flatten (x-ms-client-flatten)", () => {
  it("hoists the bag's children as ReadOnly aliases and keeps the wrapper", async () => {
    const props = await translateTarget(`
      import "@typespec/openapi";
      using OpenAPI;

      @test model Target {
        @extension("x-ms-client-flatten", true)
        properties: Inner;
        tags?: Record<string>;
      }
      model Inner {
        a: string;
        b?: int32;
      }
    `);

    // Hoisted children become ReadOnly aliases (flags 2).
    expect(props.a.flags).toBe(ObjectTypePropertyFlags.ReadOnly);
    expect(props.b.flags).toBe(ObjectTypePropertyFlags.ReadOnly);
    expect(props.a.bicep.type).toBe(TypeBaseKind.StringType);
    expect(props.b.bicep.type).toBe(TypeBaseKind.IntegerType);

    // The wrapper property is kept as the writable envelope (Required, object).
    expect(props.properties.flags).toBe(ObjectTypePropertyFlags.Required);
    expect(props.properties.bicep.type).toBe(TypeBaseKind.ObjectType);

    // Non-bag siblings are unaffected.
    expect(props.tags.flags).toBe(ObjectTypePropertyFlags.None);
  });

  it("does not hoist when a child name collides with a sibling", async () => {
    const props = await translateTarget(`
      import "@typespec/openapi";
      using OpenAPI;

      @test model Target {
        @extension("x-ms-client-flatten", true)
        properties: Inner;
        a: string;
      }
      model Inner {
        a: string;
      }
    `);

    // Collision aborts the hoist: the top-level `a` keeps its Required flag (it
    // is not overwritten by a ReadOnly alias), and only the wrapper is added.
    expect(props.a.flags).toBe(ObjectTypePropertyFlags.Required);
    expect(props.properties.bicep.type).toBe(TypeBaseKind.ObjectType);
    expect(Object.keys(props).sort()).toStrictEqual(["a", "properties"]);
  });
});
