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
