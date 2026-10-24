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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_GeneratedFileIsCurrent fails when zz_generated_readonly.go does not match what the generator renders from the
// snapshot. Run `go generate ./pkg/aws/awstypes` to fix it.
func Test_GeneratedFileIsCurrent(t *testing.T) {
	f, err := os.Open(filepath.Join("..", defaultSnapshot))
	require.NoError(t, err)
	defer f.Close()
	entries, err := parseSnapshot(f)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	want, err := render(readOnlyTypes(entries))
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join("..", defaultOut))
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run `go generate ./pkg/aws/awstypes`")
}

func Test_ParseSnapshot(t *testing.T) {
	entries, err := parseSnapshot(strings.NewReader("# comment\n\nAWS::A::ReadOnly list,read\nAWS::A::None none\nAWS::A::Legacy create,delete,update\n"))
	require.NoError(t, err)
	require.Equal(t, []entry{
		{TypeName: "AWS::A::ReadOnly", Handlers: []string{"list", "read"}},
		{TypeName: "AWS::A::None"},
		{TypeName: "AWS::A::Legacy", Handlers: []string{"create", "delete", "update"}},
	}, entries)

	_, err = parseSnapshot(strings.NewReader("AWS::A::Missing\n"))
	require.ErrorContains(t, err, "line 1")
}

func Test_ReadOnlyTypes(t *testing.T) {
	entries := []entry{
		{TypeName: "AWS::B::ReadOnly", Handlers: []string{"list", "read"}},
		{TypeName: "AWS::A::ReadOnly", Handlers: []string{"read"}},
		{TypeName: "AWS::A::Creatable", Handlers: []string{"create", "read"}},
		{TypeName: "AWS::A::None"},
	}
	require.Equal(t, []string{"AWS::A::ReadOnly", "AWS::B::ReadOnly"}, readOnlyTypes(entries))
}
