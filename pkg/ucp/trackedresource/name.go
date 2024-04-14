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
	"crypto/sha1"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// NameFor computes the resource name of a tracked resource from its ID.
//
// This can be used to compute the name of a tracked resource based on the resource that it is tracking.
//
// Names are computed by taking the name of the resource being tracked and appending a suffix to it based
// on the hash of the resource ID. This ensures that the name is unique and deterministic.
func NameFor(id *resources.ID) string {
	if id.IsEmpty() {
		return ""
	}

	// We need to generate a valid ARM/UCP name. The original resource name is used as a prefix for readability
	// followed by the hash of the resource ID.
	//
	// example:  my-resource-ec291e26078b7ea8a74abfac82530005a0ecbf15
	//
	// We want this to fit in 63 characters so we allow a prefix of 22 characters a separator and a hash of 40 characters.
	const prefixLength = 22

	prefix := strings.ToLower(id.Name())
	if len(prefix) > prefixLength {
		prefix = prefix[:prefixLength]
	}

	hasher := sha1.New()

	// It's OK to ignore the error here, it's part of the API because io.Writer is being used, but the implementation
	// does not return errors.
	_, err := hasher.Write([]byte(strings.ToLower(id.String())))
	if err != nil {
		panic("unexpected error writing to hash: " + err.Error())
	}

	hash := hasher.Sum(nil)

	return fmt.Sprintf("%s-%x", prefix, hash)
}

// IDFor computes the resource ID of a tracked resource entry from the original resource ID.
func IDFor(id *resources.ID) *resources.ID {
	if id.IsEmpty() {
		return &resources.ID{}
	}

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
				Name: NameFor(id),
			},
		}, nil))
}
