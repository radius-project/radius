// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"fmt"

	"github.com/project-radius/radius/pkg/azure/azresources"
)

const (
	ProgressFailed    = "Failed"
	ProgressCompleted = "Completed"
)

var ProgressDefaultSpinner = []string{".  ", ".. ", "..."}

// ShowResource returns true if the resource should be displayed to the user.
func ShowResource(id azresources.ResourceID) bool {
	if len(id.Types) == 1 && id.Types[0].Name == "radiusv3" {
		// Hide operations on the provider (custom action)
		return false
	}

	return true
}

// FormatResourceForDisplay returns a display string for the resource type and name.
func FormatResourceForDisplay(id azresources.ResourceID) string {
	return fmt.Sprintf("%-15s %-20s", FormatResourceNameForDisplay(id), FormatResourceTypeForDisplay(id))
}

// FormatResourceForProgressDisplay returns a display string for a progress spinner, resource type, and name.
func FormatResourceForProgressDisplay(id azresources.ResourceID) string {
	// NOTE: this format string creates ... a format string! That's intentional
	// because the progress tracker needs somewhere to put the progress.
	return fmt.Sprintf("%s %-15s %-20s", "%-20s", FormatResourceNameForDisplay(id), FormatResourceTypeForDisplay(id))
}

// FormatResourceNameForDisplay returns a display string for the resource name.
func FormatResourceNameForDisplay(id azresources.ResourceID) string {
	// Just show the last segment of the resource name.
	return id.Name()
}

// FormatResourceTypeForDisplay returns a display string for the resource type.
func FormatResourceTypeForDisplay(id azresources.ResourceID) string {
	if len(id.Types) > 0 && id.Types[0].Name == "radiusv3" {
		// It's a Radius type - just use the last segment.
		return id.Types[len(id.Types)-1].Type
	}

	// It's an ARM resource, use the qualified type.
	return id.Type()
}
