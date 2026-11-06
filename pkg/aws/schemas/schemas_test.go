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

package schemas

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func Test_Load(t *testing.T) {
	catalog, err := Load()
	require.NoError(t, err)
	require.NotNil(t, catalog.Native)
	require.NotNil(t, catalog.HandWritten)
}

func Test_LoadFS(t *testing.T) {
	file := func(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }
	tests := []struct {
		name    string
		files   fstest.MapFS
		want    Catalog
		wantErr string
	}{
		{
			name: "both directories",
			files: fstest.MapFS{
				"native/aws-docdb-dbcluster.json":               file(`{"typeName": "AWS::DocDB::DBCluster"}`),
				"native/README.md":                              file("not a schema"),
				"handwritten/aws-s3-object.json":                file(`{"typeName": "AWS::S3::Object"}`),
				"handwritten/aws-s3-object.metadata.json":       file(`{"typeName": "AWS::S3::Object", "onDelete": "delete"}`),
				"handwritten/nested/aws-ignored-directory.json": file(`{"typeName": "AWS::Ignored::Directory"}`),
			},
			want: Catalog{
				Native:      map[string]string{"AWS::DocDB::DBCluster": `{"typeName": "AWS::DocDB::DBCluster"}`},
				HandWritten: map[string]string{"AWS::S3::Object": `{"typeName": "AWS::S3::Object"}`},
			},
		},
		{
			name:    "missing directory",
			files:   fstest.MapFS{"native/README.md": file("")},
			wantErr: "failed to read the AWS schema directory handwritten",
		},
		{
			name:    "invalid JSON",
			files:   fstest.MapFS{"native/bad.json": file("{"), "handwritten/README.md": file("")},
			wantErr: "the AWS schema native/bad.json is not valid JSON",
		},
		{
			name:    "no typeName",
			files:   fstest.MapFS{"native/empty.json": file("{}"), "handwritten/README.md": file("")},
			wantErr: "the AWS schema native/empty.json has no typeName",
		},
		{
			name: "a type in both directories",
			files: fstest.MapFS{
				"native/a.json":      file(`{"typeName": "AWS::S3::Object"}`),
				"handwritten/b.json": file(`{"typeName": "aws::s3::object"}`),
			},
			wantErr: "the AWS type aws::s3::object has two schemas: native/a.json and handwritten/b.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadFS(tt.files)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
