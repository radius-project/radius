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
  TypeFactory,
  writeTypesJson
} from "../src/bicep.js";
import { getStandardizedResourceProperties } from "../src/standardized-props.js";

describe("getStandardizedResourceProperties", () => {
  it("builds the id/name/type/apiVersion envelope with AutoRest-compatible flags", () => {
    const factory = new TypeFactory();
    const resourceName = factory.addStringType();

    const props = getStandardizedResourceProperties(
      factory,
      "Applications.Messaging/rabbitMQQueues",
      "2023-10-01-preview",
      resourceName
    );

    expect(Object.keys(props)).toStrictEqual([
      "id",
      "name",
      "type",
      "apiVersion"
    ]);

    // These literal flag values are the contract with the committed golden
    // files (see generated/applications/applications.messaging/.../types.json).
    // ReadOnly | DeployTimeConstant = 2 | 8 = 10
    expect(props.id.flags).toBe(
      ObjectTypePropertyFlags.ReadOnly |
        ObjectTypePropertyFlags.DeployTimeConstant
    );
    expect(props.id.flags).toBe(10);
    // Required | DeployTimeConstant | Identifier = 1 | 8 | 16 = 25
    expect(props.name.flags).toBe(25);
    expect(props.type.flags).toBe(10);
    expect(props.apiVersion.flags).toBe(10);

    expect(props.id.description).toBe("The resource id");
    expect(props.name.description).toBe("The resource name");
  });

  it("serializes a resource body to valid Bicep types.json", () => {
    const factory = new TypeFactory();
    const resourceName = factory.addStringType();
    const props = getStandardizedResourceProperties(
      factory,
      "Applications.Messaging/rabbitMQQueues",
      "2023-10-01-preview",
      resourceName
    );
    factory.addObjectType("Applications.Messaging/rabbitMQQueues", props);

    const parsed = JSON.parse(writeTypesJson(factory.types));
    expect(Array.isArray(parsed)).toBe(true);

    const objectType = parsed.find(
      (t: { $type: string }) => t.$type === "ObjectType"
    );
    expect(objectType.name).toBe("Applications.Messaging/rabbitMQQueues");
    expect(objectType.properties.apiVersion.description).toBe(
      "The resource api version"
    );
  });
});
