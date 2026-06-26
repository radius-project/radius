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
import type { Model, Program, Type } from "@typespec/compiler";
import {
  ArrayType,
  BicepType,
  ObjectType,
  ObjectTypePropertyFlags,
  StringLiteralType,
  TypeBaseKind,
  TypeFactory,
  TypeReference,
  UnionType,
} from "../src/bicep.js";
import { translateModelProperties } from "../src/type-translator.js";

/** Compiles a single `@test model Target { ... }` and returns it plus the program. */
async function compileTarget(
  body: string,
): Promise<{ program: Program; target: Model }> {
  const host = await createTestHost();
  const runner = createTestWrapper(host);
  const types = await runner.compile(body);
  return { program: runner.program, target: types.Target as Model };
}

/** Translates `Target`'s properties and returns the factory + the resolved property types. */
async function translate(body: string): Promise<{
  factory: TypeFactory;
  props: Record<
    string,
    { flags: number; description?: string; bicep: BicepType }
  >;
}> {
  const { program, target } = await compileTarget(body);
  const factory = new TypeFactory();
  const translated = translateModelProperties(
    program,
    factory,
    target,
    new Map<Type, TypeReference>(),
  );
  const props: Record<
    string,
    { flags: number; description?: string; bicep: BicepType }
  > = {};
  for (const [name, property] of Object.entries(translated)) {
    props[name] = {
      flags: property.flags,
      description: property.description,
      bicep: factory.lookupType(property.type),
    };
  }
  return { factory, props };
}

describe("parseType", () => {
  it("maps scalars to the closest Bicep primitive", async () => {
    const { props } = await translate(
      `@test model Target { s: string; i: int32; n: int64; f: float64; b: boolean; u: url; }`,
    );

    expect(props.s.bicep.type).toBe(TypeBaseKind.StringType);
    expect(props.i.bicep.type).toBe(TypeBaseKind.IntegerType);
    expect(props.n.bicep.type).toBe(TypeBaseKind.IntegerType);
    // Floats collapse to integer (Bicep has no float), matching AutoRest.
    expect(props.f.bicep.type).toBe(TypeBaseKind.IntegerType);
    expect(props.b.bicep.type).toBe(TypeBaseKind.BooleanType);
    // url is a string-derived scalar.
    expect(props.u.bicep.type).toBe(TypeBaseKind.StringType);
  });

  it("sets the Required flag from property optionality and carries @doc", async () => {
    const { props } = await translate(
      `@test model Target { required: string; optional?: string; @doc("hi") documented: string; }`,
    );

    expect(props.required.flags).toBe(ObjectTypePropertyFlags.Required);
    expect(props.optional.flags).toBe(ObjectTypePropertyFlags.None);
    expect(props.documented.description).toBe("hi");
  });

  it("maps arrays and records", async () => {
    const { factory, props } = await translate(
      `@test model Target { list: string[]; map: Record<int32>; }`,
    );

    expect(props.list.bicep.type).toBe(TypeBaseKind.ArrayType);
    const itemType = factory.lookupType(
      (props.list.bicep as ArrayType).itemType,
    );
    expect(itemType.type).toBe(TypeBaseKind.StringType);

    expect(props.map.bicep.type).toBe(TypeBaseKind.ObjectType);
    const additional = (props.map.bicep as ObjectType).additionalProperties;
    expect(additional).toBeDefined();
    expect(factory.lookupType(additional!).type).toBe(TypeBaseKind.IntegerType);
  });

  it("maps enums and literal unions to unions of string literals", async () => {
    const { factory, props } = await translate(
      `@test model Target { color: Color; choice: "a" | "b"; }
       enum Color { red: "red", green: "green" }`,
    );

    expect(props.color.bicep.type).toBe(TypeBaseKind.UnionType);
    const colorValues = (props.color.bicep as UnionType).elements
      .map((ref) => factory.lookupType(ref) as StringLiteralType)
      .map((literal) => literal.value);
    expect(colorValues).toStrictEqual(["red", "green"]);

    expect(props.choice.bicep.type).toBe(TypeBaseKind.UnionType);
    const choiceValues = (props.choice.bicep as UnionType).elements
      .map((ref) => factory.lookupType(ref) as StringLiteralType)
      .map((literal) => literal.value);
    expect(choiceValues).toStrictEqual(["a", "b"]);
  });

  it("closes extensible enums by dropping the open string arm", async () => {
    const { factory, props } = await translate(
      `@test model Target { kind: Kind; }
       union Kind { a: "a", b: "b", string }`,
    );

    // The union of string literals plus a bare \`string\` collapses to the closed
    // set of known values (matching how Bicep represents extensible enums).
    expect(props.kind.bicep.type).toBe(TypeBaseKind.UnionType);
    const elements = (props.kind.bicep as UnionType).elements.map((ref) =>
      factory.lookupType(ref),
    );
    expect(
      elements.every(
        (element) => element.type === TypeBaseKind.StringLiteralType,
      ),
    ).toBe(true);
    expect(
      (elements as StringLiteralType[]).map((literal) => literal.value),
    ).toStrictEqual(["a", "b"]);
  });

  it("maps string literals", async () => {
    const { props } = await translate(
      `@test model Target { fixed: "constant"; }`,
    );
    expect(props.fixed.bicep.type).toBe(TypeBaseKind.StringLiteralType);
    expect((props.fixed.bicep as StringLiteralType).value).toBe("constant");
  });

  it("recurses into nested models", async () => {
    const { factory, props } = await translate(
      `@test model Target { child: Child; }
       model Child { x: string; }`,
    );

    expect(props.child.bicep.type).toBe(TypeBaseKind.ObjectType);
    const childProps = (props.child.bicep as ObjectType).properties;
    expect(factory.lookupType(childProps.x.type).type).toBe(
      TypeBaseKind.StringType,
    );
  });

  it("terminates on self-referential (cyclic) models", async () => {
    const { factory, props } = await translate(
      `@test model Target { next?: Target; }`,
    );

    expect(props.next.bicep.type).toBe(TypeBaseKind.ObjectType);
    // The cache makes the cycle resolve back to a registered object type.
    const nextProps = (props.next.bicep as ObjectType).properties;
    expect(factory.lookupType(nextProps.next.type).type).toBe(
      TypeBaseKind.ObjectType,
    );
  });
});
