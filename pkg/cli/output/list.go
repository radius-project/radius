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

func (f *ListFormatter) Format(obj any, writer io.Writer, options FormatterOptions) error {

	items, err := convertToSlice(obj)
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
