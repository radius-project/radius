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

// iamPrefixes maps the AWS SDK service package a handler's client uses to the service's prefix in IAM actions.
var iamPrefixes = map[string]string{
	"appsync":                  "appsync",
	"cloudwatch":               "cloudwatch",
	"codebuild":                "codebuild",
	"databasemigrationservice": "dms",
	"dax":                      "dax",
	"docdb":                    "rds",
	"ec2":                      "ec2",
	"elasticloadbalancingv2":   "elasticloadbalancing",
	"emr":                      "elasticmapreduce",
	"fsx":                      "fsx",
	"iam":                      "iam",
	"kinesisanalyticsv2":       "kinesisanalytics",
	"lakeformation":            "lakeformation",
	"medialive":                "medialive",
	"s3":                       "s3",
	"sagemaker":                "sagemaker",
	"servicecatalog":           "servicecatalog",
}

// sdkServicePrefix is the import path prefix of the AWS SDK service packages.
const sdkServicePrefix = "github.com/aws/aws-sdk-go-v2/service/"

// operationsOf maps a handler method to the schema handler operations whose permissions must cover the AWS calls it
// makes. Find runs when a PUT or a DELETE looks for a lost create, and Progress while UCP waits for a create, an
// update, or a delete.
var operationsOf = map[string][]string{
	"Create":          {"create"},
	"Find":            {"create", "delete"},
	"Read":            {"read"},
	"ReadScoped":      {"read"},
	"Update":          {"update"},
	"Delete":          {"delete"},
	"DeleteScoped":    {"delete"},
	"DeleteToken":     {"delete"},
	"DeleteWithToken": {"delete"},
	"Reset":           {"delete"},
	"List":            {"list"},
	"Progress":        {"create", "update", "delete"},
}

// Test_SchemaPermissionsCoverAWSCalls checks that every AWS call a native handler makes is in its schema's handler
// permissions, for each operation that can make it. It reads the handler's source: a method's calls are the methods
// of the package's AWS SDK client interfaces it, or a function or method of the package it calls, refers to, directly
// or through an SDK paginator.
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
				action := src.api[call] + ":" + call
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

// handlerSource is the parsed source of a handler package. api maps each method of the package's AWS SDK client
// interfaces, such as API and appsync's ObjectAPI, to the IAM prefix of its service.
type handlerSource struct {
	api     map[string]string
	methods map[string]*ast.FuncDecl
	funcs   map[string]*ast.FuncDecl
}

func parseHandlerSource(t *testing.T, dir string) *handlerSource {
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	src := &handlerSource{api: map[string]string{}, methods: map[string]*ast.FuncDecl{}, funcs: map[string]*ast.FuncDecl{}}
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
					if !ok {
						continue
					}
					iface, ok := ts.Type.(*ast.InterfaceType)
					if !ok {
						continue
					}
					for _, m := range iface.Methods.List {
						service := sdkService(m, imports)
						if service == "" {
							continue
						}
						prefix, ok := iamPrefixes[service]
						require.True(t, ok, "add the IAM prefix of the AWS SDK package %s to iamPrefixes", service)
						src.api[m.Names[0].Name] = prefix
					}
				}
			}
		}
	}
	require.NotEmpty(t, src.api, "the handler package %s has no AWS SDK client interface", dir)
	return src
}

// sdkService returns the AWS SDK service package of an interface method shaped like an SDK client method, such as
// GetObject(ctx, *s3.GetObjectInput, ...), or "" for any other method.
func sdkService(m *ast.Field, imports map[string]string) string {
	fn, ok := m.Type.(*ast.FuncType)
	if !ok || len(m.Names) != 1 || len(fn.Params.List) < 2 {
		return ""
	}
	star, ok := fn.Params.List[1].Type.(*ast.StarExpr)
	if !ok {
		return ""
	}
	param, ok := star.X.(*ast.SelectorExpr)
	if !ok || param.Sel.Name != m.Names[0].Name+"Input" {
		return ""
	}
	pkg, ok := param.X.(*ast.Ident)
	if !ok {
		return ""
	}
	path, ok := strings.CutPrefix(imports[pkg.Name], sdkServicePrefix)
	if !ok || strings.Contains(path, "/") {
		return ""
	}
	return path
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
				if op, ok := strings.CutPrefix(name, "New"); ok {
					// An SDK paginator, such as NewDescribeRouteTablesPaginator, makes the call it is named for.
					if op, ok := strings.CutSuffix(op, "Paginator"); ok {
						name = op
					}
				}
				if _, ok := s.api[name]; ok && !slices.Contains(out, name) {
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
