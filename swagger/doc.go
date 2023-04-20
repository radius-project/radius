// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package swagger

/*

Package swagger includes OpenAPI Specification v2 (as known as Swagger Specification) files to describe
the REST APIs of Radius resource providers.

We use OpenAPI Spec v2 as a source of truth to:
  1. Validate the incoming request (/pkg/validator)
  2. Generate Go resource models via autorest (/pkg/corerp/api, /pkg/linkrp/api) and
  3. Generate Bicep types.

OpenAPI specification files must be stored in the structured directory:

  specification/applications/resource-manager/Applications.Link/preview/2023-04-15-preview
                ------------                  ---------------------- ------- -------------------------
				Product Name                  RP Namespace           preview API version
				                                                        or
																	  stable

Each version directory has <resourcetype>.json OpenAPI Spec file which describes the REST API for
<resource type>. We release OpenAPI specifications with resource provider binary using "go:embed".
Therefore, whenever we added new specification, we need to add new file path to spec.go in this
package.

*/
