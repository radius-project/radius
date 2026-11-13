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

// Package schemas holds the schemas of the AWS types UCP serves with a native handler, in CloudFormation's resource
// schema format. Two directories hold them:
//
//   - native/ has the schemas of types that have a CloudFormation type that Cloud Control can't create in some
//     regions. Each replaces the schema CloudFormation returns, with either handler.
//   - handwritten/ has the schemas of types that have no CloudFormation type.
//
// File names follow CloudFormation's schema downloads: the type name in lowercase, with "::" replaced by "-", such as
// aws-docdb-dbcluster.json. The typeName inside each file is what identifies the type.
//
// A type can also have per-type metadata, which CloudFormation's schema format has no field for, in a file next to its
// schema named after it with .metadata.json in place of .json, such as aws-iam-accountpasswordpolicy.metadata.json.
// See Metadata. Other files, such as README.md, are ignored.
package schemas

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Directories in the embedded file system.
const (
	NativeDir      = "native"
	HandWrittenDir = "handwritten"
)

//go:embed native handwritten
var embedded embed.FS

// Catalog maps each type name, such as AWS::DocDB::DBCluster, to its schema.
type Catalog struct {
	// Native holds the schemas of types that have a CloudFormation type.
	Native map[string]string

	// HandWritten holds the schemas of types that have no CloudFormation type.
	HandWritten map[string]string

	// Metadata holds the per-type metadata of the types that have a metadata file, with defaults set. Use MetadataOf to
	// read it.
	Metadata map[string]Metadata
}

// IdentityScope is how far a type's identity reaches: the AWS identifier names one resource in the whole scope.
type IdentityScope string

const (
	// IdentityScopeRegion means an identifier names one resource in an account and region. It is the default.
	IdentityScopeRegion IdentityScope = "region"

	// IdentityScopeAccount means an identifier names one resource in the whole account, across regions, such as the IAM
	// password policy.
	IdentityScopeAccount IdentityScope = "account"
)

// OnDelete is what deleting a resource of a type does.
type OnDelete string

const (
	// OnDeleteDelete deletes the resource in AWS and removes its record. It is the default.
	OnDeleteDelete OnDelete = "delete"

	// OnDeleteReset sets the resource, a setting, back to AWS's default, and removes its record.
	OnDeleteReset OnDelete = "reset"

	// OnDeleteNothing calls nothing in AWS and removes the record, such as for a setting that protects the account.
	OnDeleteNothing OnDelete = "nothing"

	// OnDeleteReject fails the delete with 400 and changes nothing.
	OnDeleteReject OnDelete = "reject"

	// OnDeleteLeave calls nothing in AWS and keeps the record, such as for a purchase: a later deploy of the alias
	// finds it and buys nothing.
	OnDeleteLeave OnDelete = "leave"
)

// Metadata is what a type declares that CloudFormation's schema format has no field for.
type Metadata struct {
	// TypeName is the type, such as AWS::IAM::AccountPasswordPolicy. It must match the schema's typeName.
	TypeName string `json:"typeName"`

	// IdentityScope is how far the type's identity reaches. UCP searches the whole scope for another alias that holds
	// an identifier.
	IdentityScope IdentityScope `json:"identityScope,omitempty"`

	// OnDelete is what deleting a resource of the type does.
	OnDelete OnDelete `json:"onDelete,omitempty"`

	// ARNProperty is the property that holds the resource's ARN, which recipes save on their output resources. An
	// empty value means the type has no such property. nil means the metadata doesn't say, and the generated catalog
	// in pkg/aws/arnproperties is used.
	ARNProperty *string `json:"arnProperty,omitempty"`
}

// MetadataOf returns the metadata of typeName, ignoring case, or the defaults when the type has no metadata file.
func (c Catalog) MetadataOf(typeName string) Metadata {
	for name, metadata := range c.Metadata {
		if strings.EqualFold(name, typeName) {
			return metadata
		}
	}
	return Metadata{TypeName: typeName, IdentityScope: IdentityScopeRegion, OnDelete: OnDeleteDelete}
}

// Load reads the schemas built into Radius.
func Load() (Catalog, error) {
	return LoadFS(embedded)
}

// LoadFS reads the schemas and metadata in the native and handwritten directories of fsys. A file that is not valid
// JSON, a schema without a typeName, a type with two schemas, and metadata with no schema of its type next to it, or
// with an unknown field or value, fail.
func LoadFS(fsys fs.FS) (Catalog, error) {
	catalog := Catalog{Native: map[string]string{}, HandWritten: map[string]string{}, Metadata: map[string]Metadata{}}
	seen := map[string]string{}
	for _, dir := range []struct {
		name    string
		schemas map[string]string
	}{{NativeDir, catalog.Native}, {HandWrittenDir, catalog.HandWritten}} {
		entries, err := fs.ReadDir(fsys, dir.name)
		if err != nil {
			return Catalog{}, fmt.Errorf("failed to read the AWS schema directory %s: %w", dir.name, err)
		}
		// typeNames maps each schema file name in the directory to its type, for the metadata files.
		typeNames := map[string]string{}
		for _, entry := range entries {
			if entry.IsDir() || !isSchemaFile(entry.Name()) {
				continue
			}
			file := path.Join(dir.name, entry.Name())
			data, err := fs.ReadFile(fsys, file)
			if err != nil {
				return Catalog{}, fmt.Errorf("failed to read the AWS schema %s: %w", file, err)
			}
			var schema struct {
				TypeName string `json:"typeName"`
			}
			if err := json.Unmarshal(data, &schema); err != nil {
				return Catalog{}, fmt.Errorf("the AWS schema %s is not valid JSON: %w", file, err)
			}
			if schema.TypeName == "" {
				return Catalog{}, fmt.Errorf("the AWS schema %s has no typeName", file)
			}
			key := strings.ToLower(schema.TypeName)
			if other, ok := seen[key]; ok {
				return Catalog{}, fmt.Errorf("the AWS type %s has two schemas: %s and %s", schema.TypeName, other, file)
			}
			seen[key] = file
			typeNames[entry.Name()] = schema.TypeName
			dir.schemas[schema.TypeName] = string(data)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), metadataSuffix) {
				continue
			}
			metadata, err := readMetadata(fsys, dir.name, entry.Name(), typeNames)
			if err != nil {
				return Catalog{}, err
			}
			catalog.Metadata[metadata.TypeName] = metadata
		}
	}
	return catalog, nil
}

const metadataSuffix = ".metadata.json"

// readMetadata reads the metadata file name in dir. typeNames maps each schema file name in dir to its type.
func readMetadata(fsys fs.FS, dir string, name string, typeNames map[string]string) (Metadata, error) {
	file := path.Join(dir, name)
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to read the AWS type metadata %s: %w", file, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	metadata := Metadata{}
	if err := decoder.Decode(&metadata); err != nil {
		return Metadata{}, fmt.Errorf("the AWS type metadata %s is not valid: %w", file, err)
	}

	schemaFile := strings.TrimSuffix(name, metadataSuffix) + ".json"
	typeName, ok := typeNames[schemaFile]
	if !ok {
		return Metadata{}, fmt.Errorf("the AWS type metadata %s has no schema %s next to it", file, path.Join(dir, schemaFile))
	}
	if !strings.EqualFold(metadata.TypeName, typeName) {
		return Metadata{}, fmt.Errorf("the AWS type metadata %s is for %q, but its schema %s is for %s", file, metadata.TypeName, path.Join(dir, schemaFile), typeName)
	}
	metadata.TypeName = typeName

	switch metadata.IdentityScope {
	case "":
		metadata.IdentityScope = IdentityScopeRegion
	case IdentityScopeRegion, IdentityScopeAccount:
	default:
		return Metadata{}, fmt.Errorf("the AWS type metadata %s has identityScope %q. Use %q or %q", file, metadata.IdentityScope, IdentityScopeAccount, IdentityScopeRegion)
	}
	switch metadata.OnDelete {
	case "":
		metadata.OnDelete = OnDeleteDelete
	case OnDeleteDelete, OnDeleteReset, OnDeleteNothing, OnDeleteReject, OnDeleteLeave:
	default:
		return Metadata{}, fmt.Errorf("the AWS type metadata %s has onDelete %q. Use %q, %q, %q, %q, or %q", file, metadata.OnDelete, OnDeleteDelete, OnDeleteReset, OnDeleteNothing, OnDeleteReject, OnDeleteLeave)
	}
	return metadata, nil
}

// isSchemaFile reports whether a file name is a schema: a .json file that is not per-type metadata.
func isSchemaFile(name string) bool {
	return strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, metadataSuffix)
}
