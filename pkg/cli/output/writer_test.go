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

package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

type writerInput struct {
	Size   string
	IsCool bool
}

// Just a single E2E test for the Write method, writers are tested in detail elsewhere.
func Test_Write(t *testing.T) {
	obj := writerInput{
		Size:   "mega",
		IsCool: true,
	}

	buffer := &bytes.Buffer{}
	err := Write(FormatJson, obj, buffer, FormatterOptions{})
	require.NoError(t, err)

	expected := `{
  "Size": "mega",
  "IsCool": true
}
`
	require.Equal(t, expected, buffer.String())
}
