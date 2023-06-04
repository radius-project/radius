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
import os from 'os';
import path from 'path';
import { rm, mkdir } from 'fs/promises';
import { compare } from 'dir-compare';
import { defaultLogger, executeCmd, ILogger } from './utils';

const extensionDir = path.resolve(`${__dirname}/../../`);
const autorestBinary = os.platform() === 'win32' ? 'autorest.cmd' : 'autorest';
const outputBaseDir = `${__dirname}/generated`;

async function generateSchema(logger: ILogger, readme: string, outputBaseDir: string, verbose: boolean, waitForDebugger: boolean) {
  let autoRestParams = [
    `--use=@autorest/modelerfour`,
    `--use=${extensionDir}`,
    '--bicep',
    `--output-folder=${outputBaseDir}`,
    `--multiapi`,
    '--title=none',
    // This is necessary to avoid failures such as "ERROR: Semantic violation: Discriminator must be a required property." blocking type generation.
    // In an ideal world, we'd raise issues in https://github.com/Azure/azure-rest-api-specs and force RP teams to fix them, but this isn't very practical
    // as new validations are added continuously, and there's often quite a lag before teams will fix them - we don't want to be blocked by this in generating types. 
    `--skip-semantics-validation`,
    readme,
  ];

  if (verbose) {
    autoRestParams = autoRestParams.concat([
      `--debug`,
      `--verbose`,
    ]);
  }

  if (waitForDebugger) {
    autoRestParams = autoRestParams.concat([
      `--bicep.debugger`,
    ]);
  }

  return await executeCmd(logger, verbose, __dirname, autorestBinary, autoRestParams);
}

describe('integration tests', () => {
  // add any new spec paths under ./specs to this list
  const specs = [
    `basic`,
  ]

  // set to true to overwrite baselines
  const record = false;

  // bump timeout - autorest can take a while to run
  jest.setTimeout(60000);

  for (const spec of specs) {
    it(spec, async () => {
      const readmePath = path.join(__dirname, `specs/${spec}/resource-manager/README.md`);
      const outputDir = `${outputBaseDir}/${spec}`;

      if (record) {
        await rm(outputDir, { recursive: true, force: true, });
        await generateSchema(defaultLogger, readmePath, outputDir, false, false);
      } else {
        const stagingOutputDir = `${__dirname}/temp/${spec}`;
        await rm(stagingOutputDir, { recursive: true, force: true, });
  
        await generateSchema(defaultLogger, readmePath, stagingOutputDir, false, false);
  
        const compareResult = await compare(stagingOutputDir, outputDir, { compareContent: true });

        // Assert that the generated files match the baseline files which have been checked in.
        // Set 'record' to true to run the tests in record mode and overwrite baselines.
        expect(compareResult.differences).toBe(0);
      }
    });
  }
});