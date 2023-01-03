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

var formatter = &ListFormatter{}

func Test_List_Scalar(t *testing.T) {
	obj := struct {
		Size   string
		IsCool bool
	}{
		Size:   "mega",
		IsCool: true,
	}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, FormatterOptions{})
	require.NoError(t, err)

	want := "Size: mega\nIsCool: true\n\n"
	require.Equal(t, want, buffer.String())
}

func Test_List_Slice(t *testing.T) {
	obj := []struct {
		Size   string
		IsCool bool
	}{
		{
			Size:   "mega",
			IsCool: true,
		},
		{
			Size:   "medium",
			IsCool: false,
		},
	}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, FormatterOptions{})
	require.NoError(t, err)
	want := "Size: mega\nIsCool: true\n\nSize: medium\nIsCool: false\n\n"
	require.Equal(t, want, buffer.String())
}
