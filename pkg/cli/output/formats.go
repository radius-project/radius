// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

const (
	FormatJson    = "json"
	FormatTable   = "table"
	FormatList    = "list"
	DefaultFormat = FormatTable
)

// # Function Explanation
// 
//	SupportedFormats() returns a slice of strings containing the supported formats for a request. It includes the formats 
//	JSON, Table and List. If an unsupported format is requested, an error is returned.
func SupportedFormats() []string {
	return []string{
		FormatJson,
		FormatTable,
		FormatList,
	}
}
