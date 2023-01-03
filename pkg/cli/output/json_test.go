// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
