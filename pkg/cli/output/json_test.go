/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package output

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

type jsonInput struct {
	Size   string
	IsCool bool
}

func Test_JSON_Scalar(t *testing.T) {
	obj := jsonInput{
		Size:   "mega",
		IsCool: true,
	}

	formatter := &JSONFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, FormatterOptions{})
	require.NoError(t, err)

	expected := `{
  "Size": "mega",
  "IsCool": true
}
`
	require.Equal(t, expected, buffer.String())
}

func Test_JSON_Slice(t *testing.T) {
	obj := []any{
		jsonInput{
			Size:   "mega",
			IsCool: true,
		},
		jsonInput{
			Size:   "medium",
			IsCool: false,
		},
	}

	formatter := &JSONFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, FormatterOptions{})
	require.NoError(t, err)

	expected := `[
  {
    "Size": "mega",
    "IsCool": true
  },
  {
    "Size": "medium",
    "IsCool": false
  }
]
`
	require.Equal(t, expected, buffer.String())
}
