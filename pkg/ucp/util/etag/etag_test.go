// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etag

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// We don't want to unit test the actual hashing algorithm we're using
// because that might change, so the tests are very basic.
func Test_New(t *testing.T) {
	data := []byte("hello, world!")
	value := New(data)
	require.NotEmpty(t, value)
}

func Test_NewFromRevision(t *testing.T) {
	result := NewFromRevision(int64(42))
	require.Equal(t, "2a00000000000000", result)
}

func Test_ParseRevision_Valid(t *testing.T) {
	result, err := ParseRevision("2a00000000000000")
	require.NoError(t, err)
	require.Equal(t, int64(42), result)
}

func Test_ParseRevision_Invalid(t *testing.T) {
	result, err := ParseRevision("dfsfdsf2a00000000000000")
	require.Error(t, err)
	require.Equal(t, int64(0), result)
}
