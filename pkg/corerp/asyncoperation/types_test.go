// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

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
