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
	"fmt"
	"io"
	"reflect"
	"strings"
)

type FormatterOptions struct {
	// Columns used for table formatting
	Columns []Column
}

type Column struct {
	Heading     string
	JSONPath    string
	Transformer func(string) string
}

type Formatter interface {
	Format(obj any, writer io.Writer, options FormatterOptions) error
}

func NewFormatter(format string) (Formatter, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	switch normalized {
	case FormatJson:
		return &JSONFormatter{}, nil
	case FormatList:
		return &ListFormatter{}, nil
	case FormatTable:
		return &TableFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported format %s", format)
	}
}

func convertToSlice(obj any) ([]any, error) {
	// We use reflection here because we're building a table and thus need to handle both scalars (structs)
	// and slices/arrays of structs.
	var vv []any
	v := reflect.ValueOf(obj)

	// Follow pointers at the top level
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("value is nil")
		}

		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct, reflect.Interface:
		vv = append(vv, v.Interface())
	case reflect.Array, reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			item := v.Index(i)
			vv = append(vv, item.Interface())
		}
	default:
		return nil, fmt.Errorf("unsupported value kind: %v", v.Kind())
	}

	return vv, nil
}
