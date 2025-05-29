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

package renderers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_IsURL(t *testing.T) {
	const valid_url = "http://examplehost:80"
	const invalid_url = "http://abc:def"
	const path = "/testpath/testfolder/testfile.txt"

	require.True(t, IsURL(valid_url))
	require.False(t, IsURL(invalid_url))
	require.False(t, IsURL(path))
}

func Test_ParseURL(t *testing.T) {
	const valid_url = "http://examplehost:80"
	const invalid_url = "http://abc:def"

	t.Run("valid URL test", func(t *testing.T) {
		scheme, hostname, port, err := ParseURL(valid_url)
		require.Equal(t, scheme, "http")
		require.Equal(t, hostname, "examplehost")
		require.Equal(t, port, "80")
		require.Equal(t, err, nil)
	})

	t.Run("invalid URL test", func(t *testing.T) {
		scheme, hostname, port, err := ParseURL(invalid_url)
		require.Equal(t, scheme, "")
		require.Equal(t, hostname, "")
		require.Equal(t, port, "")
		require.NotEqual(t, err, nil)
	})
}
