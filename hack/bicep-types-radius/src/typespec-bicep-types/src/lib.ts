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

import { createTypeSpecLibrary, JSONSchemaType } from "@typespec/compiler";

/**
 * Options accepted by the Bicep types emitter. The emitter currently takes no
 * options - the output location is controlled by the standard
 * `emitter-output-dir`. The (empty) options object is still declared so the
 * emitter can be listed in `tspconfig.yaml` without a schema error, and so
 * future options have a home.
 */
export type BicepEmitterOptions = Record<string, never>;

const BicepEmitterOptionsSchema: JSONSchemaType<BicepEmitterOptions> = {
  type: "object",
  additionalProperties: false,
  properties: {},
  required: [],
};

/**
 * The TypeSpec library definition. `name` MUST match the package name so that
 * `tsp compile --emit=@radius-project/typespec-bicep-types` can resolve it.
 */
export const $lib = createTypeSpecLibrary({
  name: "@radius-project/typespec-bicep-types",
  // The emitter reports no diagnostics of its own today; entries can be added
  // here if it ever needs to surface emit-time warnings or errors.
  diagnostics: {},
  emitter: {
    options: BicepEmitterOptionsSchema,
  },
});
