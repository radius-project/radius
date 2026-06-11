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

// Functional test for x-ms-client-flatten support in the Bicep type generator.
//
// This test runs autorest end-to-end against the shared "basic" spec (which
// includes resources exercising the happy-path flatten, the polymorphic-child
// fallback, and the name-collision fallback) and asserts directly on the
// generated types.json. Unlike the integration test (which only baseline-diffs
// the output), this test fails with a focused error message if the flatten
// contract regresses.

import os from "os";
import path from "path";
import { readFile, rm, mkdir, mkdtemp } from "fs/promises";
import { ObjectTypePropertyFlags } from "bicep-types";
import { defaultLogger, executeCmd } from "../integration/utils";

const extensionDir = path.resolve(`${__dirname}/../../`);
const autorestBinary = os.platform() === "win32" ? "autorest.cmd" : "autorest";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type GeneratedType = Record<string, any> & { $type: string };

async function generate(outDir: string): Promise<GeneratedType[]> {
  await rm(outDir, { recursive: true, force: true });
  await mkdir(outDir, { recursive: true });

  const readme = path.join(
    __dirname,
    "..",
    "integration",
    "specs",
    "basic",
    "resource-manager",
    "README.md"
  );

  await executeCmd(defaultLogger, false, __dirname, autorestBinary, [
    "--use=@autorest/modelerfour",
    `--use=${extensionDir}`,
    "--bicep",
    `--output-folder=${outDir}`,
    "--multiapi",
    "--title=none",
    "--skip-semantics-validation",
    readme
  ]);

  const typesPath = path.join(outDir, "test.rp1", "2021-10-31", "types.json");
  const raw = await readFile(typesPath, "utf-8");
  return JSON.parse(raw) as GeneratedType[];
}

function indexOf(ref: string): number {
  // refs are of the form "#/<index>"
  const m = /^#\/(\d+)$/.exec(ref);
  if (!m) {
    throw new Error(`Unexpected ref: ${ref}`);
  }
  return parseInt(m[1], 10);
}

function findResource(
  types: GeneratedType[],
  resourceName: string
): GeneratedType {
  // ResourceType.name is "<type>@<apiVersion>"; accept either form.
  const resource = types.find(
    (t) =>
      t.$type === "ResourceType" &&
      (t.name === resourceName ||
        (typeof t.name === "string" && t.name.startsWith(`${resourceName}@`)))
  );
  if (!resource) {
    throw new Error(`Resource not found: ${resourceName}`);
  }
  return resource;
}

function findObjectByName(
  types: GeneratedType[],
  name: string
): GeneratedType | undefined {
  return types.find(
    (t) =>
      (t.$type === "ObjectType" || t.$type === "DiscriminatedObjectType") &&
      t.name === name
  );
}

function resourceBody(
  types: GeneratedType[],
  resourceName: string
): GeneratedType {
  const resource = findResource(types, resourceName);
  return types[indexOf(resource.body.$ref)];
}

describe("x-ms-client-flatten functional tests", () => {
  let types: GeneratedType[];
  let stagingDir: string;

  beforeAll(async () => {
    stagingDir = await mkdtemp(path.join(os.tmpdir(), "flatten-types-"));
    types = await generate(stagingDir);
  }, 120000);

  afterAll(async () => {
    if (stagingDir) {
      await rm(stagingDir, { recursive: true, force: true });
    }
  });

  describe("happy path: TestType1", () => {
    it("hoists flattened child properties to the resource body as ReadOnly aliases", () => {
      const body = resourceBody(types, "Test.Rp1/testType1");

      // Flattened children must appear at the top level.
      expect(body.properties).toHaveProperty("basicString");
      expect(body.properties).toHaveProperty("stringEnum");

      // ...as ReadOnly aliases (so they show up for output references but
      // cannot be assigned to from a template body, which would generate a
      // payload the RP cannot parse).
      expect(
        body.properties.basicString.flags & ObjectTypePropertyFlags.ReadOnly
      ).toBe(ObjectTypePropertyFlags.ReadOnly);
      expect(
        body.properties.stringEnum.flags & ObjectTypePropertyFlags.ReadOnly
      ).toBe(ObjectTypePropertyFlags.ReadOnly);
      // ...and never Required (the wrapper `properties` carries Required).
      expect(
        body.properties.basicString.flags & ObjectTypePropertyFlags.Required
      ).toBe(0);
      expect(
        body.properties.stringEnum.flags & ObjectTypePropertyFlags.Required
      ).toBe(0);
      // ...and never WriteOnly (read-side surface only).
      expect(
        body.properties.basicString.flags & ObjectTypePropertyFlags.WriteOnly
      ).toBe(0);
      expect(
        body.properties.stringEnum.flags & ObjectTypePropertyFlags.WriteOnly
      ).toBe(0);
    });

    it("keeps the writable `properties` envelope so existing templates still compile", () => {
      const body = resourceBody(types, "Test.Rp1/testType1");

      expect(body.properties).toHaveProperty("properties");
      const propsRef = body.properties.properties.type.$ref;
      const wrapper = types[indexOf(propsRef)];
      expect(wrapper.$type).toBe("ObjectType");
      expect(wrapper.name).toBe("TestType1Properties");
    });

    it("preserves standardized resource properties alongside flattened children", () => {
      const body = resourceBody(types, "Test.Rp1/testType1");

      for (const standard of [
        "id",
        "name",
        "type",
        "apiVersion",
        "location",
        "tags",
        "systemData"
      ]) {
        expect(body.properties).toHaveProperty(standard);
      }
    });

    it("propagates the flattened child's description, not the wrapper's", () => {
      const body = resourceBody(types, "Test.Rp1/testType1");
      expect(body.properties.basicString.description).toBe(
        "Description for a basic string property."
      );
    });
  });

  describe("polymorphic-child fallback: TestType2", () => {
    it("keeps the nested properties bag when the child has a discriminator", () => {
      const body = resourceBody(types, "Test.Rp1/testType2");

      expect(body.properties).toHaveProperty("properties");

      const propsRef = body.properties.properties.type.$ref;
      const referenced = types[indexOf(propsRef)];
      expect(referenced.$type).toBe("DiscriminatedObjectType");
      expect(referenced.name).toBe("TestType2Properties");
    });
  });

  describe("name-collision fallback: TestType3", () => {
    it("keeps the nested properties bag when a flattened child would collide", () => {
      const body = resourceBody(types, "Test.Rp1/testType3");

      expect(body.properties).toHaveProperty("properties");

      const wrapper = findObjectByName(types, "TestType3Properties");
      expect(wrapper).toBeDefined();
      expect(wrapper?.$type).toBe("ObjectType");

      // The wrapper retains both children, including the would-collide "name" field.
      const wrapperProps = wrapper!.properties as Record<string, unknown>;
      expect(wrapperProps).toHaveProperty("name");
      expect(wrapperProps).toHaveProperty("extra");

      // Fall-back-to-nested-representation contract: the non-colliding child
      // `extra` must NOT have been partially hoisted to the resource body —
      // collision on any child rejects the whole flatten.
      expect(body.properties).not.toHaveProperty("extra");

      // The resource body's standardized "name" was not overwritten by the
      // child: it should still resolve to a plain StringType.
      const bodyName = body.properties.name.type.$ref;
      const nameType = types[indexOf(bodyName)];
      expect(nameType.$type).toBe("StringType");
    });
  });
});
