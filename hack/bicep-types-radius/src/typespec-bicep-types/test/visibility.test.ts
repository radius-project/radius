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
import { createTestHost, createTestWrapper } from "@typespec/compiler/testing";
import type { Model, Type } from "@typespec/compiler";
import {
  ObjectType,
  ObjectTypePropertyFlags,
  TypeFactory,
  TypeReference,
} from "../src/bicep.js";
import { translateModelProperties } from "../src/type-translator.js";

/** Compiles a `@test model Target` and returns each translated property's flags. */
async function flagsFor(body: string): Promise<Record<string, number>> {
  const host = await createTestHost();
  const runner = createTestWrapper(host);
  const types = await runner.compile(body);
  const target = types.Target as Model;

  const factory = new TypeFactory();
  const translated = translateModelProperties(
    runner.program,
    factory,
    target,
    new Map<Type, TypeReference>(),
  );

  const flags: Record<string, number> = {};
  for (const [name, property] of Object.entries(translated)) {
    flags[name] = property.flags;
  }
  return flags;
}

describe("property flags from @visibility", () => {
  it("maps lifecycle visibility to ReadOnly / WriteOnly / Required", async () => {
    const flags = await flagsFor(`
      @test model Target {
        @visibility(Lifecycle.Read) readOnly: string;
        @visibility(Lifecycle.Create) createOnly: string;
        @visibility(Lifecycle.Read, Lifecycle.Create) readCreate: string;
        readWrite: string;
        optionalReadWrite?: string;
      }
    `);

    // Visible for Read only -> ReadOnly (2), never also Required.
    expect(flags.readOnly).toBe(ObjectTypePropertyFlags.ReadOnly);
    // Visible for Create only and required -> WriteOnly | Required (4 | 1 = 5).
    expect(flags.createOnly).toBe(
      ObjectTypePropertyFlags.WriteOnly | ObjectTypePropertyFlags.Required,
    );
    // Visible for both Read and Create -> read-write, required -> Required (1).
    expect(flags.readCreate).toBe(ObjectTypePropertyFlags.Required);
    // No @visibility -> visible everywhere -> read-write; required -> Required.
    expect(flags.readWrite).toBe(ObjectTypePropertyFlags.Required);
    expect(flags.optionalReadWrite).toBe(ObjectTypePropertyFlags.None);
  });
});

describe("top-level location exception", () => {
  it("never marks a top-level location property Required, but keeps nested ones", async () => {
    const host = await createTestHost();
    const runner = createTestWrapper(host);
    const types = await runner.compile(`
      @test model Target { location: string; other: string; nested: Nested; }
      model Nested { location: string; }
    `);
    const target = types.Target as Model;

    const factory = new TypeFactory();
    const translated = translateModelProperties(
      runner.program,
      factory,
      target,
      new Map<Type, TypeReference>(),
      undefined,
      true,
    );

    // A required top-level `location` is emitted without the Required flag.
    expect(translated.location.flags).toBe(ObjectTypePropertyFlags.None);
    // Other required top-level properties keep Required (the exception is scoped).
    expect(translated.other.flags).toBe(ObjectTypePropertyFlags.Required);
    // A `location` nested inside another model is unaffected and stays Required.
    const nested = factory.lookupType(translated.nested.type) as ObjectType;
    expect(nested.properties.location.flags).toBe(
      ObjectTypePropertyFlags.Required,
    );
  });
});
