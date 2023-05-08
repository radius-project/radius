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
