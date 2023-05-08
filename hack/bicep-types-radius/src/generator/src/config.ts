// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------.
import { Dictionary } from "lodash";

export interface GeneratorConfig {
  additionalFiles: string[];
}

const defaultConfig: GeneratorConfig = {
  additionalFiles: [],
}

const config: Dictionary<GeneratorConfig> = {
  'keyvault': {
    additionalFiles: [
      'Microsoft.KeyVault/stable/2016-10-01/secrets.json',
      'Microsoft.KeyVault/stable/2018-02-14/secrets.json',
      'Microsoft.KeyVault/preview/2018-02-14-preview/secrets.json',
      'Microsoft.KeyVault/stable/2019-09-01/secrets.json',
    ],
  }
}

export function getConfig(basePath: string): GeneratorConfig {
  return config[basePath.toLowerCase()] || defaultConfig;
}