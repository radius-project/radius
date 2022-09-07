// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperationType_String(t *testing.T) {
	opTypeTests := []struct {
		in  OperationType
		out string
	}{
		{
			in:  OperationType{Type: "applications.core/environments", Method: OperationPut},
			out: "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
		},
		{
			in:  OperationType{Type: "applications.core/environments", Method: "ListSecret"},
			out: "APPLICATIONS.CORE/ENVIRONMENTS|LISTSECRET",
		},
	}

	for _, tt := range opTypeTests {
		require.Equal(t, tt.out, tt.in.String())
	}
}

func TestOperationType_ParseOperationType(t *testing.T) {
	opTypeTests := []struct {
		in     string
		out    OperationType
		parsed bool
	}{
		{
			in:     "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
			out:    OperationType{Type: "APPLICATIONS.CORE/ENVIRONMENTS", Method: OperationPut},
			parsed: true,
		},
		{
			in:     "APPLICATIONS.CORE/ENVIRONMENTS|LISTSECRET",
			out:    OperationType{Type: "APPLICATIONS.CORE/ENVIRONMENTS", Method: "LISTSECRET"},
			parsed: true,
		},
		{
			in:     "APPLICATIONS.CORE/ENVIRONMENTS",
			out:    OperationType{},
			parsed: false,
		},
	}

	for _, tt := range opTypeTests {
		actual, ok := ParseOperationType(tt.in)
		require.Equal(t, tt.out, actual)
		require.Equal(t, tt.parsed, ok)
	}
}

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
