/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
func ShowResource(id ucpresources.ID) bool {
	return true
}

// FormatResourceForDisplay returns a display string for the resource type and name.
func FormatResourceForDisplay(id ucpresources.ID) string {
	return fmt.Sprintf("%-15s %-20s", FormatResourceNameForDisplay(id), FormatResourceTypeForDisplay(id))
}

// FormatResourceForProgressDisplay returns a display string for a progress spinner, resource type, and name.
func FormatResourceForProgressDisplay(id ucpresources.ID) string {
	// NOTE: this format string creates ... a format string! That's intentional
	// because the progress tracker needs somewhere to put the progress.
	return fmt.Sprintf("%s %-15s %-20s", "%-20s", FormatResourceNameForDisplay(id), FormatResourceTypeForDisplay(id))
}

// FormatResourceNameForDisplay returns a display string for the resource name.
func FormatResourceNameForDisplay(id ucpresources.ID) string {
	// Just show the last segment of the resource name.
	return id.Name()
}

// FormatResourceTypeForDisplay returns a display string for the resource type.
func FormatResourceTypeForDisplay(id ucpresources.ID) string {
	// It's an ARM resource, use the qualified type.
	return id.Type()
}
