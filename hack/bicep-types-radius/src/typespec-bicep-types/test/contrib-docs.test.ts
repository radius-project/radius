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
  createObjectProperty,
  writeMarkdown
} from "../src/bicep.js";
import {
  buildResourceDocs,
  filterResourceTypes
} from "../src/writers/resource-docs.js";

const RESOURCE_NAME = "Radius.Compute/containers@2025-08-01-preview";

function buildFactory(): TypeFactory {
  const factory = new TypeFactory();
  const stringRef = factory.addStringType();
  const bodyRef = factory.addObjectType(RESOURCE_NAME, {
    application: createObjectProperty(
      stringRef,
      ObjectTypePropertyFlags.Required,
      "(Required) The Radius Application ID."
    )
  });
  factory.addResourceType(
    RESOURCE_NAME,
    bodyRef,
    ScopeType.ResourceGroup,
    ScopeType.ResourceGroup
  );
  return factory;
}

/** GitHub-style heading slug, matching how the rendered types.md anchors resolve. */
function headingSlug(heading: string): string {
  return heading
    .toLowerCase()
    .replace(/[^a-z0-9 -]/g, "")
    .trim()
    .replace(/ /g, "-");
}

describe("contrib reference docs", () => {
  it("leads with the description block when the manifest supplies one", () => {
    const factory = buildFactory();

    const [doc] = buildResourceDocs(
      filterResourceTypes(factory.types),
      factory.types,
      () => "Runs one or more containers."
    );

    expect(doc.filename).toBe("containers.md");
    expect(doc.content.startsWith("## Description")).toBe(true);
    expect(doc.content).toContain("Runs one or more containers.");
  });

  it("emits a types.md anchor matching the link format used by index.md", () => {
    const factory = buildFactory();

    const typesMarkdown = writeMarkdown(
      factory.types,
      "Radius.Compute @ 2025-08-01-preview"
    );

    // The generated index.md links to `types.md#resource-<name with separators stripped>`.
    const indexLinkAnchor = `resource-${RESOURCE_NAME.toLowerCase().replace(/[^a-z0-9-]/g, "")}`;
    const anchors = [...typesMarkdown.matchAll(/^#+\s+(.*)$/gm)].map(
      ([, heading]) => headingSlug(heading)
    );

    expect(anchors).toContain(indexLinkAnchor);
  });
});
