// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/project-radius/radius/pkg/kubernetes"
)

const (
	// Separator represents the resource name separator.
	Separator = "-"
)

// MakeResourceName builds resource name with prefix for azure resources.
// For instance, when user uses keyvault persistent volume, RP will
// auto-provision per-container managed identity in the resource group
// which is specified by environment resource. In this case,
// RP uses application name as prefix to avoid the name conflict in the same
// resource group.
func MakeResourceName(prefix, name, separator string) string {
	if name == "" {
		panic("name is empty.")
	}
	if prefix != "" {
		prefix += separator
	}
	return kubernetes.NormalizeResourceName(prefix + name)
}
