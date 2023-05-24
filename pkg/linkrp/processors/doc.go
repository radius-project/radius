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

// processors contains the shared logic and interfaces for implementing a resource processor.
//
// Resource processors are responsible for processing the results of recipe execution or any
// other change to the lifecycle of a link resource.
//
// For example a mongo processor might take the results of executing a recipe and compute
// and store the connection string as part of the resource data model.
package processors
