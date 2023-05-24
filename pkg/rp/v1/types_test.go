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

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEqualLinkedResource(t *testing.T) {
	parentResourceTests := []struct {
		propA BasicResourceProperties
		propB BasicResourceProperties
		eq    bool
	}{
		{
			propA: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/invalid",
				Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			},
			propB: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/invalid",
				Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			},
			eq: true,
		},
		{
			propA: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/INVALID",
				Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.core/environments/env0",
			},
			propB: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/invalid",
				Environment: "/SUBSCRIPTIONS/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			},
			eq: true,
		},
		{
			propA: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/INVALID",
				Environment: "",
			},
			propB: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/invalid",
				Environment: "",
			},
			eq: true,
		},
		{
			propA: BasicResourceProperties{
				Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.core/environments/env0",
			},
			propB: BasicResourceProperties{
				Environment: "/SUBSCRIPTIONS/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/Applications.Core/environments/env0",
			},
			eq: true,
		},
		{
			propA: BasicResourceProperties{
				Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.core/environments/env0",
			},
			propB: BasicResourceProperties{
				Environment: "/SUBSCRIPTIONS/00000000-0000-0000-0000-000000000000/resourcegroups/radius-test-rg/providers/Applications.Core/environments/env1",
			},
			eq: false,
		},
		{
			propA: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/INVALID",
				Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.core/environments/env0",
			},
			propB: BasicResourceProperties{
				Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/invalid",
				Environment: "/plans/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			},
			eq: false,
		},
	}

	for _, tt := range parentResourceTests {
		require.Equal(t, tt.propA.EqualLinkedResource(&tt.propB), tt.eq)
	}
}
