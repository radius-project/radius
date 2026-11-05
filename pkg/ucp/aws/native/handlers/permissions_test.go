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

package handlers

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/aws/schemas"
)

// iamPrefixes maps the AWS SDK service package a handler's API uses to the service's prefix in IAM actions.
var iamPrefixes = map[string]string{
	"databasemigrationservice": "dms",
	"dax":                      "dax",
	"docdb":                    "rds",
	"emr":                      "elasticmapreduce",
	"fsx":                      "fsx",
}

// operationsOf maps a handler method to the schema handler operations whose permissions must cover the AWS calls it
// makes. Find runs when a PUT or a DELETE looks for a lost create, and Progress while UCP waits for a create, an
// update, or a delete.
var operationsOf = map[string][]string{
	"Create":          {"create"},
	"Find":            {"create", "delete"},
	"Read":            {"read"},
	"Update":          {"update"},
	"Delete":          {"delete"},
	"DeleteToken":     {"delete"},
	"DeleteWithToken": {"delete"},
	"Reset":           {"delete"},
	"List":            {"list"},
	"Progress":        {"create", "update", "delete"},
}

// Test_SchemaPermissionsCoverAWSCalls checks that every AWS call a native handler makes is in its schema's handler
// permissions, for each operation that can make it. It reads the handler's source: a method's calls are the methods
// of the package's API interface it, or a function or method of the package it calls, refers to.
func Test_SchemaPermissionsCoverAWSCalls(t *testing.T) {
	catalog, err := schemas.Load()
	require.NoError(t, err)
	packages := map[string]*handlerSource{}
	missing := []string{}
	for _, h := range All(aws.Config{}) {
		typ := reflect.TypeOf(h).Elem()
		src, ok := packages[typ.PkgPath()]
		if !ok {
			src = parseHandlerSource(t, filepath.Base(typ.PkgPath()))
			packages[typ.PkgPath()] = src
		}

		var schema struct {
			Handlers map[string]struct {
				Permissions []string `json:"permissions"`
			} `json:"handlers"`
		}
		require.NoError(t, json.Unmarshal([]byte(catalog.Native[h.TypeName()]), &schema), h.TypeName())
		for method, ops := range operationsOf {
			for _, call := range src.calls(typ.Name(), method) {
				action := src.prefix + ":" + call
				for _, op := range ops {
					gap := h.TypeName() + " " + op + ": " + action
					if !slices.Contains(schema.Handlers[op].Permissions, action) && !slices.Contains(missing, gap) {
						missing = append(missing, gap)
					}
				}
			}
		}
	}
	slices.Sort(missing)
	require.Empty(t, missing, "add each AWS call to the handler permissions of the operation that makes it")
}

// handlerSource is the parsed source of a handler package.
type handlerSource struct {
	prefix  string
	api     map[string]bool
	methods map[string]*ast.FuncDecl
	funcs   map[string]*ast.FuncDecl
}

func parseHandlerSource(t *testing.T, dir string) *handlerSource {
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	src := &handlerSource{api: map[string]bool{}, methods: map[string]*ast.FuncDecl{}, funcs: map[string]*ast.FuncDecl{}}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		imports := map[string]string{}
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			local := filepath.Base(path)
			if spec.Name != nil {
				local = spec.Name.Name
			}
			imports[local] = path
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					src.funcs[d.Name.Name] = d
					continue
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if ident, ok := recv.(*ast.Ident); ok {
					src.methods[ident.Name+"."+d.Name.Name] = d
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts.Name.Name != "API" {
						continue
					}
					for _, m := range ts.Type.(*ast.InterfaceType).Methods.List {
						src.api[m.Names[0].Name] = true
						param := m.Type.(*ast.FuncType).Params.List[1].Type.(*ast.StarExpr).X.(*ast.SelectorExpr)
						service := filepath.Base(imports[param.X.(*ast.Ident).Name])
						prefix, ok := iamPrefixes[service]
						require.True(t, ok, "add the IAM prefix of the AWS SDK package %s to iamPrefixes", service)
						src.prefix = prefix
					}
				}
			}
		}
	}
	require.NotEmpty(t, src.api, "the handler package %s has no API interface", dir)
	return src
}

// calls returns the API methods the method of the handler type refers to, directly or through the package's functions
// and the type's other methods, sorted.
func (s *handlerSource) calls(typeName string, method string) []string {
	start, ok := s.methods[typeName+"."+method]
	if !ok {
		return nil
	}
	out := []string{}
	seen := map[*ast.FuncDecl]bool{}
	var visit func(fn *ast.FuncDecl)
	visit = func(fn *ast.FuncDecl) {
		if seen[fn] {
			return
		}
		seen[fn] = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.SelectorExpr:
				name := e.Sel.Name
				if s.api[name] && !slices.Contains(out, name) {
					out = append(out, name)
				}
				if m, ok := s.methods[typeName+"."+name]; ok {
					visit(m)
				}
			case *ast.Ident:
				if f, ok := s.funcs[e.Name]; ok {
					visit(f)
				}
			}
			return true
		})
	}
	visit(start)
	slices.Sort(out)
	return out
}
