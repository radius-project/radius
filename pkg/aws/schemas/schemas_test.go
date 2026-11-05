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
	"encoding/json"
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

// objectSchema is a hand-written schema that follows every rule.
const objectSchema = `{
	"typeName": "AWS::S3::Object",
	"properties": {"Bucket": {"type": "string"}, "Key": {"type": "string"}, "Body": {"type": "string"}, "VersionId": {"type": "string"}},
	"primaryIdentifier": ["/properties/Bucket", "/properties/Key"],
	"createOnlyProperties": ["/properties/Bucket", "/properties/Key"],
	"readOnlyProperties": ["/properties/VersionId"],
	"writeOnlyProperties": ["/properties/Body"],
	"handlers": {
		"create": {"permissions": ["s3:PutObject"]},
		"read": {"permissions": ["s3:GetObject"]},
		"delete": {"permissions": ["s3:DeleteObject"]}
	}
}`

// policySchema is a hand-written schema of a setting whose delete calls nothing in AWS, so it needs no delete handler.
const policySchema = `{
	"typeName": "AWS::IAM::AccountPasswordPolicy",
	"properties": {"MinimumPasswordLength": {"type": "integer"}, "Id": {"type": "string"}},
	"primaryIdentifier": ["/properties/Id"],
	"handlers": {
		"create": {"permissions": ["iam:UpdateAccountPasswordPolicy"]},
		"read": {"permissions": ["iam:GetAccountPasswordPolicy"]}
	}
}`

func file(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }

func Test_LoadFS(t *testing.T) {
	tests := []struct {
		name    string
		files   fstest.MapFS
		want    Catalog
		wantErr string
	}{
		{
			name: "both directories",
			files: fstest.MapFS{
				"native/aws-docdb-dbcluster.json":         file(`{"typeName": "AWS::DocDB::DBCluster"}`),
				"native/README.md":                        file("not a schema"),
				"handwritten/aws-s3-object.json":          file(objectSchema),
				"handwritten/aws-s3-object.metadata.json": file(`{"typeName": "AWS::S3::Object", "onDelete": "delete"}`),
			},
			want: Catalog{
				Native:      map[string]string{"AWS::DocDB::DBCluster": `{"typeName": "AWS::DocDB::DBCluster"}`},
				HandWritten: map[string]string{"AWS::S3::Object": objectSchema},
				Metadata: map[string]Metadata{
					"AWS::S3::Object": {TypeName: "AWS::S3::Object", IdentityScope: IdentityScopeRegion, OnDelete: OnDeleteDelete},
				},
			},
		},
		{
			name: "metadata with every field",
			files: fstest.MapFS{
				"native/README.md": file(""),
				"handwritten/aws-iam-accountpasswordpolicy.json":          file(policySchema),
				"handwritten/aws-iam-accountpasswordpolicy.metadata.json": file(`{"typeName": "aws::iam::accountpasswordpolicy", "identityScope": "account", "onDelete": "nothing", "arnProperty": ""}`),
			},
			want: Catalog{
				Native:      map[string]string{},
				HandWritten: map[string]string{"AWS::IAM::AccountPasswordPolicy": policySchema},
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
		{
			name: "a subdirectory",
			files: fstest.MapFS{
				"native/README.md":                      file(""),
				"handwritten/nested/aws-s3-object.json": file(objectSchema),
			},
			wantErr: "the AWS schema directory handwritten has a subdirectory nested. Put each schema directly in handwritten",
		},
		{
			name: "a hand-written schema that breaks a rule",
			files: fstest.MapFS{
				"native/README.md":        file(""),
				"handwritten/object.json": file(objectSchema),
			},
			wantErr: "the hand-written AWS schema handwritten/object.json is for AWS::S3::Object, so name the file aws-s3-object.json",
		},
		{
			name: "a native schema need not follow the hand-written rules",
			files: fstest.MapFS{
				"native/a.json":         file(`{"typeName": "AWS::A::B", "properties": {"lowerCase": {}}}`),
				"handwritten/README.md": file(""),
			},
			want: Catalog{
				Native:      map[string]string{"AWS::A::B": `{"typeName": "AWS::A::B", "properties": {"lowerCase": {}}}`},
				HandWritten: map[string]string{},
				Metadata:    map[string]Metadata{},
			},
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

func Test_checkHandWritten(t *testing.T) {
	// with returns objectSchema with field set to value, or removed when value is nil.
	with := func(field string, value any) string {
		schema := map[string]any{}
		require.NoError(t, json.Unmarshal([]byte(objectSchema), &schema))
		if value == nil {
			delete(schema, field)
		} else {
			schema[field] = value
		}
		data, err := json.Marshal(schema)
		require.NoError(t, err)
		return string(data)
	}
	handlers := func(names ...string) map[string]any {
		out := map[string]any{}
		for _, name := range names {
			out[name] = map[string]any{"permissions": []string{"s3:Any"}}
		}
		return out
	}
	deleteMetadata := Metadata{TypeName: "AWS::S3::Object", OnDelete: OnDeleteDelete}
	tests := []struct {
		name     string
		file     string
		schema   string
		metadata Metadata
		wantErr  string
	}{
		{name: "valid", schema: objectSchema},
		{name: "valid with every handler", schema: with("handlers", handlers("create", "read", "update", "delete", "list"))},
		{name: "a nested path", schema: with("writeOnlyProperties", []string{"/properties/Body/Data"})},
		{
			name:     "no delete handler when a delete calls nothing in AWS",
			schema:   with("handlers", handlers("create", "read")),
			metadata: Metadata{TypeName: "AWS::S3::Object", OnDelete: OnDeleteNothing},
		},
		{
			name:     "no delete handler when a delete resets",
			schema:   with("handlers", handlers("create", "read")),
			metadata: Metadata{TypeName: "AWS::S3::Object", OnDelete: OnDeleteReset},
			wantErr:  "has no delete handler. List the AWS permissions it needs under handlers",
		},
		{name: "not a schema", schema: with("properties", []string{"Bucket"}), wantErr: "is not a valid schema"},
		{name: "a typeName with no service", schema: with("typeName", "AWS::Object"), wantErr: `has typeName "AWS::Object". Use CloudFormation's naming, such as AWS::S3::Object`},
		{name: "a typeName in Bicep form", schema: with("typeName", "AWS.S3/Object"), wantErr: `has typeName "AWS.S3/Object"`},
		{name: "a file named after another type", file: "aws-s3-objects.json", schema: objectSchema, wantErr: "is for AWS::S3::Object, so name the file aws-s3-object.json"},
		{name: "a file name in another case", file: "AWS-S3-Object.json", schema: objectSchema, wantErr: "so name the file aws-s3-object.json"},
		{name: "no properties", schema: with("properties", nil), wantErr: "has no properties"},
		{name: "a camelCase property", schema: with("properties", map[string]any{"Bucket": map[string]any{}, "Key": map[string]any{}, "Body": map[string]any{}, "VersionId": map[string]any{}, "contentType": map[string]any{}}), wantErr: `has the property "contentType". Use the AWS API's parameter name in PascalCase`},
		{name: "a property with a separator", schema: with("properties", map[string]any{"Bucket": map[string]any{}, "Key": map[string]any{}, "Body": map[string]any{}, "VersionId": map[string]any{}, "Content_Type": map[string]any{}}), wantErr: `has the property "Content_Type"`},
		{name: "no primaryIdentifier", schema: with("primaryIdentifier", nil), wantErr: "has no primaryIdentifier"},
		{name: "a primaryIdentifier that names no property", schema: with("primaryIdentifier", []string{"/properties/Name"}), wantErr: `has "/properties/Name" in primaryIdentifier, which names no property in properties`},
		{name: "a create-only path outside properties", schema: with("createOnlyProperties", []string{"/definitions/Bucket"}), wantErr: `has "/definitions/Bucket" in createOnlyProperties`},
		{name: "a read-only path that names no property", schema: with("readOnlyProperties", []string{"/properties/Arn"}), wantErr: `has "/properties/Arn" in readOnlyProperties`},
		{name: "a write-only path that names no property", schema: with("writeOnlyProperties", []string{"/properties/Password"}), wantErr: `has "/properties/Password" in writeOnlyProperties`},
		{name: "no handlers", schema: with("handlers", nil), wantErr: "has no create handler"},
		{name: "no read handler", schema: with("handlers", handlers("create", "delete")), wantErr: "has no read handler"},
		{name: "no delete handler", schema: with("handlers", handlers("create", "read")), wantErr: "has no delete handler"},
		{name: "an unknown handler", schema: with("handlers", handlers("create", "read", "delete", "upsert")), wantErr: `has the handler "upsert". Use create, read, update, delete, or list`},
		{
			name:    "a handler with no permissions",
			schema:  with("handlers", map[string]any{"create": map[string]any{"permissions": []string{"s3:PutObject"}}, "read": map[string]any{"permissions": []string{}}, "delete": map[string]any{"permissions": []string{"s3:DeleteObject"}}}),
			wantErr: "lists no permissions for its read handler",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.file
			if name == "" {
				name = "aws-s3-object.json"
			}
			metadata := tt.metadata
			if metadata.TypeName == "" {
				metadata = deleteMetadata
			}
			err := checkHandWritten(name, tt.schema, metadata)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
