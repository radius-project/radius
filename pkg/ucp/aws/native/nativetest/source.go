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

package nativetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// sdkServicePrefix is the import path prefix of the AWS SDK service packages.
const sdkServicePrefix = "github.com/aws/aws-sdk-go-v2/service/"

// HandlerSource is the parsed source of a handler package, without its tests. API maps each AWS SDK operation the
// package refers to, to the SDK service package it belongs to, such as docdb: the methods of the package's SDK client
// interfaces, such as API and appsync's ObjectAPI, and the operations whose input the package builds, such as
// &docdb.CreateDBClusterInput{}, so a call through a concrete client or an imported interface counts too.
type HandlerSource struct {
	API     map[string]string
	methods map[string]*ast.FuncDecl
	funcs   map[string]*ast.FuncDecl
}

// ParseHandlerSource parses the Go files of the handler package in dir.
func ParseHandlerSource(t testing.TB, dir string) *HandlerSource {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	src := &HandlerSource{API: map[string]string{}, methods: map[string]*ast.FuncDecl{}, funcs: map[string]*ast.FuncDecl{}}
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
						if service := sdkService(m, imports); service != "" {
							src.API[m.Names[0].Name] = service
						}
					}
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok {
				if op, service := sdkInput(lit.Type, imports); op != "" {
					src.API[op] = service
				}
			}
			return true
		})
	}
	return src
}

// sdkInput returns the operation and AWS SDK service package of an SDK input type, such as CreateDBCluster and docdb
// for docdb.CreateDBClusterInput, or "" for any other type.
func sdkInput(expr ast.Expr, imports map[string]string) (string, string) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	op, ok := strings.CutSuffix(sel.Sel.Name, "Input")
	if !ok || op == "" {
		return "", ""
	}
	service, ok := strings.CutPrefix(imports[pkg.Name], sdkServicePrefix)
	if !ok || strings.Contains(service, "/") {
		return "", ""
	}
	return op, service
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

// Calls returns the API methods the method of the handler type refers to, directly or through the package's functions
// and the type's other methods, sorted. An SDK paginator, such as NewDescribeRouteTablesPaginator, counts as the call
// it is named for.
func (s *HandlerSource) Calls(typeName string, method string) []string {
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
					if op, ok := strings.CutSuffix(op, "Paginator"); ok {
						name = op
					}
				}
				if _, ok := s.API[name]; ok && !slices.Contains(out, name) {
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

// AllCalls returns the API methods any method of the handler type refers to, sorted.
func (s *HandlerSource) AllCalls(typeName string) []string {
	out := []string{}
	for key := range s.methods {
		name, method, _ := strings.Cut(key, ".")
		if name != typeName {
			continue
		}
		for _, call := range s.Calls(typeName, method) {
			if !slices.Contains(out, call) {
				out = append(out, call)
			}
		}
	}
	slices.Sort(out)
	return out
}

// handlerCalls returns the AWS calls the handler makes, read from its package's source, and the name of the handler's
// type. A test may wrap the handler in a type of its own that embeds it, so a type with no methods in its package's
// source stands for the first type it embeds that has.
func handlerCalls(t testing.TB, handler any) ([]string, string) {
	t.Helper()
	sources := map[string]*HandlerSource{}
	var find func(v reflect.Value) ([]string, string, bool)
	find = func(v reflect.Value) ([]string, string, bool) {
		for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil, "", false
			}
			v = v.Elem()
		}
		typ := v.Type()
		if typ.Kind() != reflect.Struct {
			return nil, "", false
		}
		// An unnamed struct, such as struct{ native.Handler }, has no source of its own.
		if typ.Name() != "" {
			src, ok := sources[typ.PkgPath()]
			if !ok {
				src = ParseHandlerSource(t, packageDir(t, typ.PkgPath()))
				sources[typ.PkgPath()] = src
			}
			if src.hasMethods(typ.Name()) {
				return src.AllCalls(typ.Name()), typ.Name(), true
			}
		}
		for i := range typ.NumField() {
			if typ.Field(i).Anonymous {
				if calls, name, ok := find(v.Field(i)); ok {
					return calls, name, true
				}
			}
		}
		return nil, "", false
	}
	calls, name, ok := find(reflect.ValueOf(handler))
	require.True(t, ok, "the handler %T has no methods in its package's source", handler)
	return calls, name
}

// hasMethods reports whether the source declares methods of the type.
func (s *HandlerSource) hasMethods(typeName string) bool {
	for key := range s.methods {
		if name, _, _ := strings.Cut(key, "."); name == typeName {
			return true
		}
	}
	return false
}

// packageDir returns the directory of the package with the import path pkgPath in the module that holds the current
// directory, as go test runs a test in its package's directory.
func packageDir(t testing.TB, pkgPath string) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if module, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					rel, ok := strings.CutPrefix(pkgPath, strings.TrimSpace(module)+"/")
					require.True(t, ok, "the package %s isn't in the module %s", pkgPath, module)
					return filepath.Join(dir, filepath.FromSlash(rel))
				}
			}
			require.Fail(t, "go.mod has no module line", dir)
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above the current directory")
		dir = parent
	}
}
