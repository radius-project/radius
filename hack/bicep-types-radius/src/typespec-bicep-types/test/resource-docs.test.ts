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
import {
  ObjectTypePropertyFlags,
  ScopeType,
  TypeFactory,
  createObjectProperty
} from "../src/bicep.js";
import {
  buildResourceDocs,
  filterResourceTypes
} from "../src/writers/resource-docs.js";

/**
 * Builds a factory containing one resource type per supplied name, plus the
 * non-resource types they reference, so the helper can be exercised against the
 * same flat `BicepType[]` shape that `types.json` deserializes into.
 */
function buildTypes(names: string[]): TypeFactory {
  const factory = new TypeFactory();
  const stringRef = factory.addStringType();

  for (const name of names) {
    const bodyRef = factory.addObjectType(name, {
      name: createObjectProperty(
        stringRef,
        ObjectTypePropertyFlags.Required,
        "The resource name."
      )
    });
    factory.addResourceType(
      name,
      bodyRef,
      ScopeType.ResourceGroup,
      ScopeType.ResourceGroup
    );
  }

  return factory;
}

describe("filterResourceTypes", () => {
  it("returns only the resource types from a flat type list", () => {
    const factory = buildTypes([
      "Radius.Compute/containers@2025-08-01-preview"
    ]);

    const resourceTypes = filterResourceTypes(factory.types);

    expect(resourceTypes.map((resourceType) => resourceType.name)).toEqual([
      "Radius.Compute/containers@2025-08-01-preview"
    ]);
  });

  it("returns an empty list when there are no resource types", () => {
    const factory = new TypeFactory();
    factory.addStringType();

    expect(filterResourceTypes(factory.types)).toEqual([]);
  });
});

describe("buildResourceDocs", () => {
  it("derives the file name from the resource type name", () => {
    const factory = buildTypes([
      "Radius.Compute/containers@2025-08-01-preview"
    ]);

    const docs = buildResourceDocs(
      filterResourceTypes(factory.types),
      factory.types
    );

    expect(docs).toHaveLength(1);
    expect(docs[0].filename).toBe("containers.md");
  });

  it("joins nested resource type segments so child types get distinct names", () => {
    const factory = buildTypes([
      "Radius.Compute/containers/sidecars@2025-08-01-preview"
    ]);

    const docs = buildResourceDocs(
      filterResourceTypes(factory.types),
      factory.types
    );

    expect(docs[0].filename).toBe("containers-sidecars.md");
  });

  it("renders the description when a lookup supplies one", () => {
    const factory = buildTypes([
      "Radius.Compute/containers@2025-08-01-preview"
    ]);

    const docs = buildResourceDocs(
      filterResourceTypes(factory.types),
      factory.types,
      () => "Runs a containerized workload."
    );

    expect(docs[0].content).toContain("Runs a containerized workload.");
  });

  it("omits the description block when no lookup is supplied", () => {
    const factory = buildTypes([
      "Radius.Compute/containers@2025-08-01-preview"
    ]);

    const docs = buildResourceDocs(
      filterResourceTypes(factory.types),
      factory.types
    );

    expect(docs[0].content).not.toContain("## Description");
  });

  it("rejects a resource type name without a namespace segment", () => {
    const factory = new TypeFactory();
    const bodyRef = factory.addObjectType("Malformed", {});
    factory.addResourceType(
      "malformed@2025-08-01-preview",
      bodyRef,
      ScopeType.ResourceGroup,
      ScopeType.ResourceGroup
    );

    expect(() =>
      buildResourceDocs(filterResourceTypes(factory.types), factory.types)
    ).toThrow(/expected 'Namespace\/type@version'/);
  });

  it("rejects two API versions of one resource type, which share a file name", () => {
    const factory = buildTypes([
      "Radius.Compute/containers@2025-08-01-preview",
      "Radius.Compute/containers@2026-01-01"
    ]);

    expect(() =>
      buildResourceDocs(filterResourceTypes(factory.types), factory.types)
    ).toThrow(/same reference doc file name.*containers\.md/s);
  });

  it("rejects nested resource types that collapse to the same hyphenated name", () => {
    const factory = buildTypes([
      "Radius.Compute/containers-sidecars@2025-08-01-preview",
      "Radius.Compute/containers/sidecars@2025-08-01-preview"
    ]);

    expect(() =>
      buildResourceDocs(filterResourceTypes(factory.types), factory.types)
    ).toThrow(/same reference doc file name/);
  });
});
