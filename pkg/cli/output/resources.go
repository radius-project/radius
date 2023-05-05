// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"fmt"

	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	ProgressFailed    = "Failed"
	ProgressCompleted = "Completed"
)

var ProgressDefaultSpinner = []string{".  ", ".. ", "..."}

// ShowResource returns true if the resource should be displayed to the user.
//
// # Function Explanation
// 
//	ShowResource checks if a given resource ID is valid and returns a boolean value indicating the result. If an invalid ID 
//	is provided, an error is returned.
func ShowResource(id ucpresources.ID) bool {
	return true
}

// FormatResourceForDisplay returns a display string for the resource type and name.
//
// # Function Explanation
// 
//	FormatResourceForDisplay takes an ID and returns a formatted string containing the resource name and type. It handles 
//	errors by returning an empty string if the ID is invalid.
func FormatResourceForDisplay(id ucpresources.ID) string {
	return fmt.Sprintf("%-15s %-20s", FormatResourceNameForDisplay(id), FormatResourceTypeForDisplay(id))
}

// FormatResourceForProgressDisplay returns a display string for a progress spinner, resource type, and name.
//
// # Function Explanation
// 
//	FormatResourceForProgressDisplay creates a format string for displaying a resource's name and type in a progress 
//	tracker. It handles errors by returning an empty string if the resource ID is invalid.
func FormatResourceForProgressDisplay(id ucpresources.ID) string {
	// NOTE: this format string creates ... a format string! That's intentional
	// because the progress tracker needs somewhere to put the progress.
	return fmt.Sprintf("%s %-15s %-20s", "%-20s", FormatResourceNameForDisplay(id), FormatResourceTypeForDisplay(id))
}

// FormatResourceNameForDisplay returns a display string for the resource name.
//
// # Function Explanation
// 
//	FormatResourceNameForDisplay takes in an ID object and returns the last segment of the resource name. If the ID is 
//	invalid, an empty string is returned.
func FormatResourceNameForDisplay(id ucpresources.ID) string {
	// Just show the last segment of the resource name.
	return id.Name()
}

// FormatResourceTypeForDisplay returns a display string for the resource type.
//
// # Function Explanation
// 
//	FormatResourceTypeForDisplay returns the type of the given resource ID as a string, using the qualified type if it is an
//	 ARM resource. If an invalid ID is passed, an empty string is returned.
func FormatResourceTypeForDisplay(id ucpresources.ID) string {
	// It's an ARM resource, use the qualified type.
	return id.Type()
}
