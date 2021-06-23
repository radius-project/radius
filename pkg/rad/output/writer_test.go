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
