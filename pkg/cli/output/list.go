// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"fmt"
	"io"
	"reflect"
)

type ListFormatter struct{}

var _ Formatter = (*ListFormatter)(nil)

func (f *ListFormatter) Format(obj interface{}, writer io.Writer, options FormatterOptions) error {

	items, err := f.normalize(obj)
	if err != nil {
		return err
	}

	for _, item := range items {
		v := reflect.ValueOf(item)
		typeOfObj := v.Type()
		for i := 0; i < v.NumField(); i++ {
			fmt.Fprintf(writer, "%v: %v\n", typeOfObj.Field(i).Name, v.Field(i).Interface())
		}
		fmt.Fprint(writer, "\n")
	}

	return nil
}

func (f *ListFormatter) normalize(obj interface{}) ([]interface{}, error) {
	var vv []interface{}
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
