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
	"errors"
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

func Test_MetadataOf(t *testing.T) {
	catalog := Catalog{Metadata: map[string]Metadata{
		"AWS::IAM::AccountPasswordPolicy": {TypeName: "AWS::IAM::AccountPasswordPolicy", IdentityScope: IdentityScopeAccount, OnDelete: OnDeleteNothing},
	}}
	require.Equal(t, catalog.Metadata["AWS::IAM::AccountPasswordPolicy"], catalog.MetadataOf("aws::iam::accountpasswordpolicy"))
	require.Equal(t, Metadata{TypeName: "AWS::S3::Object", IdentityScope: IdentityScopeRegion, OnDelete: OnDeleteDelete}, catalog.MetadataOf("AWS::S3::Object"))
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
				Metadata: map[string]Metadata{
					"AWS::S3::Object": {TypeName: "AWS::S3::Object", IdentityScope: IdentityScopeRegion, OnDelete: OnDeleteDelete},
				},
			},
		},
		{
			name: "metadata with every field",
			files: fstest.MapFS{
				"native/README.md": file(""),
				"handwritten/aws-iam-accountpasswordpolicy.json":          file(`{"typeName": "AWS::IAM::AccountPasswordPolicy"}`),
				"handwritten/aws-iam-accountpasswordpolicy.metadata.json": file(`{"typeName": "aws::iam::accountpasswordpolicy", "identityScope": "account", "onDelete": "nothing", "arnProperty": ""}`),
			},
			want: Catalog{
				Native:      map[string]string{},
				HandWritten: map[string]string{"AWS::IAM::AccountPasswordPolicy": `{"typeName": "AWS::IAM::AccountPasswordPolicy"}`},
				Metadata: map[string]Metadata{
					"AWS::IAM::AccountPasswordPolicy": {TypeName: "AWS::IAM::AccountPasswordPolicy", IdentityScope: IdentityScopeAccount, OnDelete: OnDeleteNothing, ARNProperty: new("")},
				},
			},
		},
		{
			name: "metadata with only identityScope gets the default onDelete",
			files: fstest.MapFS{
				"native/a.json":          file(`{"typeName": "AWS::A::B"}`),
				"native/a.metadata.json": file(`{"typeName": "AWS::A::B", "identityScope": "account"}`),
				"handwritten/README.md":  file(""),
			},
			want: Catalog{
				Native:      map[string]string{"AWS::A::B": `{"typeName": "AWS::A::B"}`},
				HandWritten: map[string]string{},
				Metadata:    map[string]Metadata{"AWS::A::B": {TypeName: "AWS::A::B", IdentityScope: IdentityScopeAccount, OnDelete: OnDeleteDelete}},
			},
		},
		{
			name: "metadata with no schema next to it",
			files: fstest.MapFS{
				"native/aws-docdb-dbcluster.json":               file(`{"typeName": "AWS::DocDB::DBCluster"}`),
				"handwritten/aws-docdb-dbcluster.metadata.json": file(`{"typeName": "AWS::DocDB::DBCluster"}`),
			},
			wantErr: "the AWS type metadata handwritten/aws-docdb-dbcluster.metadata.json has no schema handwritten/aws-docdb-dbcluster.json next to it",
		},
		{
			name: "metadata for another type",
			files: fstest.MapFS{
				"native/aws-docdb-dbcluster.json":          file(`{"typeName": "AWS::DocDB::DBCluster"}`),
				"native/aws-docdb-dbcluster.metadata.json": file(`{"typeName": "AWS::RDS::DBCluster"}`),
				"handwritten/README.md":                    file(""),
			},
			wantErr: `the AWS type metadata native/aws-docdb-dbcluster.metadata.json is for "AWS::RDS::DBCluster", but its schema native/aws-docdb-dbcluster.json is for AWS::DocDB::DBCluster`,
		},
		{
			name: "metadata with an unknown field",
			files: fstest.MapFS{
				"native/a.json":          file(`{"typeName": "AWS::A::B"}`),
				"native/a.metadata.json": file(`{"typeName": "AWS::A::B", "deletePolicy": "keep"}`),
				"handwritten/README.md":  file(""),
			},
			wantErr: "the AWS type metadata native/a.metadata.json is not valid",
		},
		{
			name: "metadata with an unknown identityScope",
			files: fstest.MapFS{
				"native/a.json":          file(`{"typeName": "AWS::A::B"}`),
				"native/a.metadata.json": file(`{"typeName": "AWS::A::B", "identityScope": "organization"}`),
				"handwritten/README.md":  file(""),
			},
			wantErr: `the AWS type metadata native/a.metadata.json has identityScope "organization". Use "account" or "region"`,
		},
		{
			name: "metadata with an unknown onDelete",
			files: fstest.MapFS{
				"native/a.json":          file(`{"typeName": "AWS::A::B"}`),
				"native/a.metadata.json": file(`{"typeName": "AWS::A::B", "onDelete": "retain"}`),
				"handwritten/README.md":  file(""),
			},
			wantErr: `the AWS type metadata native/a.metadata.json has onDelete "retain". Use "delete", "reset", "nothing", "reject", or "leave"`,
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

// unreadable is a file system whose file name can be listed but not read.
type unreadable struct {
	fstest.MapFS
	name string
}

func (u unreadable) ReadFile(name string) ([]byte, error) {
	if name == u.name {
		return nil, errors.New("permission denied")
	}
	return u.MapFS.ReadFile(name)
}

func Test_LoadFS_UnreadableMetadata(t *testing.T) {
	files := fstest.MapFS{
		"native/a.json":          {Data: []byte(`{"typeName": "AWS::A::B"}`)},
		"native/a.metadata.json": {Data: []byte(`{"typeName": "AWS::A::B"}`)},
		"handwritten/README.md":  {},
	}
	_, err := LoadFS(unreadable{MapFS: files, name: "native/a.metadata.json"})
	require.ErrorContains(t, err, "failed to read the AWS type metadata native/a.metadata.json: permission denied")
}
