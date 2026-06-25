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

package trackedresource

import (
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/hashutil"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// hashLength is the number of hexadecimal characters of the resource-ID hash appended to a
// tracked resource name. It matches the width of the legacy SHA-1 hash (40) so that names
// continue to fit within the 63-character ARM/UCP name limit (22 prefix + 1 separator + 40 hash).
const hashLength = 40

// NameFor computes the resource name of a tracked resource from its ID.
//
// This can be used to compute the name of a tracked resource based on the resource that it is tracking.
//
// Names are computed by taking the name of the resource being tracked and appending a suffix to it based
// on the hash of the resource ID. This ensures that the name is unique and deterministic.
func NameFor(id resources.ID) string {
	if id.IsEmpty() {
		return ""
	}

	return nameWithHash(id, hashutil.Hex([]byte(strings.ToLower(id.String()))))
}

// LegacyNameFor computes the tracked resource name using the legacy SHA-1 hash.
//
// Deprecated: SHA-1 is retained only to locate tracked resource entries written by older versions of
// Radius during the migration to SHA-256. Use NameFor for new values. See
// https://github.com/radius-project/radius/issues/8084.
func LegacyNameFor(id resources.ID) string {
	if id.IsEmpty() {
		return ""
	}

	return nameWithHash(id, hashutil.LegacyHex([]byte(strings.ToLower(id.String()))))
}

// nameWithHash builds a tracked resource name from the tracked resource's name (used as a
// human-readable prefix) and a hex hash of its ID.
func nameWithHash(id resources.ID, hash string) string {
	// We need to generate a valid ARM/UCP name. The original resource name is used as a prefix for
	// readability followed by the hash of the resource ID.
	//
	// example:  my-resource-ec291e26078b7ea8a74abfac82530005a0ecbf15
	//
	// We want this to fit in 63 characters so we allow a prefix of 22 characters, a separator, and a
	// hash of 40 characters.
	const prefixLength = 22

	prefix := strings.ToLower(id.Name())
	if len(prefix) > prefixLength {
		prefix = prefix[:prefixLength]
	}

	if len(hash) > hashLength {
		hash = hash[:hashLength]
	}

	return fmt.Sprintf("%s-%s", prefix, hash)
}

// IDFor computes the resource ID of a tracked resource entry from the original resource ID.
func IDFor(id resources.ID) resources.ID {
	if id.IsEmpty() {
		return resources.ID{}
	}

	return idWithName(id, NameFor(id))
}

// LegacyIDFor computes the resource ID of a tracked resource entry using the legacy SHA-1 name.
//
// Deprecated: used only to locate tracked resource entries written by older versions of Radius
// during the migration to SHA-256. See https://github.com/radius-project/radius/issues/8084.
func LegacyIDFor(id resources.ID) resources.ID {
	if id.IsEmpty() {
		return resources.ID{}
	}

	return idWithName(id, LegacyNameFor(id))
}

// idWithName builds the tracking entry ID for the given resource ID and computed name.
func idWithName(id resources.ID, name string) resources.ID {
	// Tracking ID is the ID of the entry that will store the data.
	//
	// Example:
	//	id: /planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app
	//	trackingID: /planes/radius/local/resourceGroups/test-group/providers/System.Resources/genericResources/test-app-ec291e26078b7ea8a74abfac82530005a0ecbf15
	return resources.MustParse(resources.MakeUCPID(
		id.ScopeSegments(),
		[]resources.TypeSegment{
			{
				Type: v20231001preview.ResourceType,
				Name: name,
			},
		}, nil))
}
