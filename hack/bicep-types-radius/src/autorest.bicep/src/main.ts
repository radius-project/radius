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
import { orderBy } from 'lodash';
import { ResourceType } from "./types"
import { CodeModel, codeModelSchema } from "@autorest/codemodel";
import { writeJson } from './writers/json';
import { writeMarkdown } from "./writers/markdown";
import { writeTableMarkdown } from "./writers/markdown-table";
import { getProviderDefinitions } from "./resources";

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
      host.writeFile({ filename: `${outFolder}/types.json`, content: writeJson(types) });

      // writer types.md
      host.writeFile({ filename: `${outFolder}/types.md`, content: writeMarkdown(namespace, apiVersion, types) });

      // writer resource types
      const resourceTypes = orderBy(types.filter(t => t instanceof ResourceType) as ResourceType[], x => x.Name.split('@')[0].toLowerCase());
      for (const resourceType of resourceTypes) {
        const filename = resourceType.Name.split('/')[1].split('@')[0].toLowerCase();
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
