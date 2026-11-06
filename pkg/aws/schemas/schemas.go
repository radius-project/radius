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
// aws-docdb-dbcluster.json. The typeName inside each file is what identifies the type. Other files, such as
// README.md, are ignored.
package schemas

import (
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
}

// Load reads the schemas built into Radius.
func Load() (Catalog, error) {
	return LoadFS(embedded)
}

// LoadFS reads the schemas in the native and handwritten directories of fsys. A file that is not valid JSON, a schema
// without a typeName, and a type with two schemas fail.
func LoadFS(fsys fs.FS) (Catalog, error) {
	catalog := Catalog{Native: map[string]string{}, HandWritten: map[string]string{}}
	seen := map[string]string{}
	for _, dir := range []struct {
		name    string
		schemas map[string]string
	}{{NativeDir, catalog.Native}, {HandWrittenDir, catalog.HandWritten}} {
		entries, err := fs.ReadDir(fsys, dir.name)
		if err != nil {
			return Catalog{}, fmt.Errorf("failed to read the AWS schema directory %s: %w", dir.name, err)
		}
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
			dir.schemas[schema.TypeName] = string(data)
		}
	}
	return catalog, nil
}

// isSchemaFile reports whether a file name is a schema: a .json file that is not per-type metadata.
func isSchemaFile(name string) bool {
	return strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".metadata.json")
}
