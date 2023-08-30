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

package swagger

/*

Package swagger includes OpenAPI Specification v2 (as known as Swagger Specification) files to describe
the REST APIs of Radius resource providers.

We use OpenAPI Spec v2 as a source of truth to:
  1. Validate the incoming request (/pkg/validator)
  2. Generate Go resource models via autorest (/pkg/corerp/api, /pkg/datastores/api, /pkg/dapr/api, /pkg/messaging/api) and
  3. Generate Bicep types.

OpenAPI specification files must be stored in the structured directory:

  specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview
                ------------                  ---------------------- ------- -------------------------
				Product Name                  RP Namespace           preview API version
				                                                        or
																	  stable

Each version directory has <resourcetype>.json OpenAPI Spec file which describes the REST API for
<resource type>. We release OpenAPI specifications with resource provider binary using "go:embed".
Therefore, whenever we added new specification, we need to add new file path to spec.go in this
package.

*/
