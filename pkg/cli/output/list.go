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
