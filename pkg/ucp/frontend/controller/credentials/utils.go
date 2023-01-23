// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package credentials

import (
	"strings"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// GetSecretName returns the secret name of credential storage.
func GetSecretName(id resources.ID) string {
	planeNamespace := id.PlaneNamespace()
	planeNamespace = strings.ReplaceAll(planeNamespace, "/", "-")
	return kubernetes.NormalizeResourceName(planeNamespace + "-" + id.Name())
}
