// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import "strings"

const (
	// Separator represents the resource name separator.
	Separator = "-"
)

// MakeResourceName builds resource name with prefix for azure resources.
// It does not use a separator between prefix and name due to the limitation of
// some of azure resource. If you need a separator, you can add it as a prefix
// argument.
//
// For instance, when user uses keyvault persistent volume, RP will
// auto-provision per-container managed identity in the resource group
// which is specified by environment resource. In this case,
// RP uses application name as prefix to avoid the name conflict in the same
// resource group.
func MakeResourceName(name string, prefix ...string) string {
	var sb strings.Builder

	for _, p := range prefix {
		sb.WriteString(strings.ToLower(p))
	}
	sb.WriteString(strings.ToLower(name))
	return sb.String()
}
