/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package edges hosts helpers used by
// both the CLI static-graph builder (`rad app graph <app.bicep>`) and
// the Radius.Core/2025-08-01-preview runtime handler (`getGraph`
// action) to capture edges denoted by both Radius `connections` construct as well
// as by ARM json's dependsOn.
//

package edges
