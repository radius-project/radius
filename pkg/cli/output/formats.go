// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

const (
	FormatJson    = "json"
	FormatTable   = "table"
	DefaultFormat = FormatTable
)

func SupportedFormats() []string {
	return []string{
		FormatJson,
		FormatTable,
	}
}
