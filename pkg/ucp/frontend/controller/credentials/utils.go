// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package credentials

import (
	"strings"

	"github.com/project-radius/radius/pkg/ucp/resources"
)

func GetSecretName(id resources.ID) string {
	id.Name()
	planeNamespace := id.PlaneNamespace()
	planeNamespace = strings.ReplaceAll(planeNamespace, "/", "_")
	return planeNamespace + "_" + id.Name()
}
