// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

// # Function Explanation
// 
//	NewFormatter takes in a format string and returns a Formatter interface based on the format string. It handles errors by
//	 returning a nil Formatter and an error if the format string is not supported.
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
