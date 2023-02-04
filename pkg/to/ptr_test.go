// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azcore/to

package to

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPtr(t *testing.T) {
	b := true
	pb := Ptr(b)

	require.NotNil(t, pb, "unexpected nil conversion")
	require.Equal(t, b, *pb)
}

func TestSliceOfPtrs(t *testing.T) {
	arr := SliceOfPtrs[int]()
	require.Len(t, arr, 0, "expected zero length")

	arr = SliceOfPtrs(1, 2, 3, 4, 5)
	for i, v := range arr {
		require.Equal(t, i+1, *v)
	}
}
