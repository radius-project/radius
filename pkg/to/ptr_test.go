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
	require.Exactly(t, b, *pb)
}

func TestSliceOfPtrs(t *testing.T) {
	arr := SliceOfPtrs[int]()
	require.Len(t, arr, 0, "expected zero length")

	arr = SliceOfPtrs(1, 2, 3, 4, 5)
	for i, v := range arr {
		require.Exactly(t, i+1, *v)
	}
}
