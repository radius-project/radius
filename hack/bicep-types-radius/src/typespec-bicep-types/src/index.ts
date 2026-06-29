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

// Public entry point for the TypeSpec compiler. `$lib` registers the library and
// its emitter options; `$onEmit` is the emit hook. Both names are required by the
// TypeSpec compiler's library/emitter contract.
export { $lib } from "./lib.js";
export type { BicepEmitterOptions } from "./lib.js";
export { $onEmit } from "./emitter.js";
