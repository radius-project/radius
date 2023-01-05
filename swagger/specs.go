// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package swagger

import (
	"embed"
)

var (
	// The listing of files below has an ordering to them, because
	// each file may depend on one or more files on the preceding
	// lines.

	//go:embed specification/applications/resource-manager/Applications.Link/preview/2022-03-15-privatepreview/*.json
	//go:embed specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/*.json
	//go:embed specification/common-types/resource-management/v2/types.json
	//go:embed specification/common-types/resource-management/v3/types.json
	SpecFiles embed.FS

	//go:embed specification/common-types/resource-management/v2/types.json
	//go:embed specification/ucp/resource-manager/UCP/preview/2022-09-01-privatepreview/*.json
	SpecFilesUCP embed.FS
)
