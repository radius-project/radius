// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import "strings"

// MakeResourceName builds resource name with prefix for azure resources.
// For instance, when user uses keyvault persistent volume, RP will
// auto-provision per-container managed identity in the resource group
// which is specified by environment resource. In this case,
// RP uses application name as prefix to avoid the name conflict in the same
// resource group.
func MakeResourceName(name string, prefix ...string) string {
	var sb strings.Builder

	for _, p := range prefix {
		sb.WriteString(strings.ToLower(p))
		sb.WriteString("-")
	}
	sb.WriteString(strings.ToLower(name))
	return sb.String()
}
