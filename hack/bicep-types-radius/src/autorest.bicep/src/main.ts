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

import { AutoRestExtension, AutorestExtensionHost, startSession } from "@autorest/extension-base";
import { generateTypes } from "./type-generator";
import { CodeModel, codeModelSchema } from "@autorest/codemodel";
import { orderBy } from 'lodash';
import { getProviderDefinitions } from "./resources";
import { writeTypesJson, writeMarkdown, TypeBaseKind, ResourceType } from "bicep-types";
import { writeTableMarkdown } from "./writers/markdown-table"; 

export async function processRequest(host: AutorestExtensionHost) {
  try {
    const session = await startSession<CodeModel>(
      host,
      undefined,
      codeModelSchema
    );
    const start = Date.now();

    for (const definition of getProviderDefinitions(session.model, host)) {
      const { namespace, apiVersion } = definition;
      const types = generateTypes(host, definition);

      const outFolder = `${namespace}/${apiVersion}`.toLowerCase();

      // write types.json
      host.writeFile({ filename: `${outFolder}/types.json`, content: writeTypesJson(types) });

      // writer types.md
      host.writeFile({ filename: `${outFolder}/types.md`, content: writeMarkdown(types, `${namespace} @ ${apiVersion}`) });

      // writer resource types
      const resourceTypes = orderBy(types.filter(t => t.type == TypeBaseKind.ResourceType) as ResourceType[], x => x.name.split('@')[0].toLowerCase());
      for (const resourceType of resourceTypes) {
        const filename = resourceType.name.split('/')[1].split('@')[0].toLowerCase();
        host.writeFile({ filename: `${outFolder}/docs/${filename}.md`, content: writeTableMarkdown(namespace, apiVersion, [resourceType], types) });
      }
    }

    session.info(`autorest.bicep took ${Date.now() - start}ms`);
  } catch (err) {
    console.error("An error was encountered while handling a request:", err);
    throw err;
  }
}

async function main() {
  const pluginHost = new AutoRestExtension();
  pluginHost.add("bicep", processRequest);
  await pluginHost.run();
}

// eslint-disable-next-line jest/require-hook
main();
