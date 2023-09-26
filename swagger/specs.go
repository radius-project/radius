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

import (
	"embed"
)

var (
	// The listing of files below has an ordering to them, because
	// each file may depend on one or more files on the preceding
	// lines.

	//go:embed specification/applications/resource-manager/Applications.Datastores/preview/2023-10-01-preview/*.json
	//go:embed specification/applications/resource-manager/Applications.Dapr/preview/2023-10-01-preview/*.json
	//go:embed specification/applications/resource-manager/Applications.Messaging/preview/2023-10-01-preview/*.json
	//go:embed specification/applications/resource-manager/Applications.Core/preview/2023-10-01-preview/openapi.json
	//go:embed specification/common-types/resource-management/v2/types.json
	//go:embed specification/common-types/resource-management/v3/types.json
	SpecFiles embed.FS

	//go:embed specification/common-types/resource-management/v2/types.json
	//go:embed specification/common-types/resource-management/v3/types.json
	//go:embed specification/ucp/resource-manager/UCP/preview/2023-10-01-preview/*.json
	SpecFilesUCP embed.FS
)
