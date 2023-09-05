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

package radius

import (
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_IsRadiusResource(t *testing.T) {
	values := []struct {
		testID   resources.ID
		expected bool
	}{
		{
			testID:   resources.MustParse("/planes/radius/local/resourceGroups/r1/providers/Applications.Core/containers/test-container"),
			expected: true,
		},
		{
			testID:   resources.MustParse("/planes/radius/local/resourceGroups/r1/providers/Applications.Datastores/mongoDatabases/test-mongo"),
			expected: true,
		},
		{
			testID:   resources.MustParse("/planes/radius/local/resourceGroups/r1/providers/Applications.foo/containers/test-container"),
			expected: true,
		},
		{
			testID:   resources.MustParse("/planes/kubernetes/local/resourceGroups/r1/providers/Applications.core/containers/test-container"),
			expected: false,
		},
	}

	for i, v := range values {
		t.Run(fmt.Sprintf("%d: %v", i, v.testID.String()), func(t *testing.T) {
			radiusResource := IsRadiusResource(v.testID)
			require.Equal(t, v.expected, radiusResource)
		})
	}
}
