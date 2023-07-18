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

package store

import (
	"reflect"
	"strings"
)

// # Function Explanation
// 
// MatchesFilters checks if the object's data matches the given filters and returns a boolean and an error.
func (o Object) MatchesFilters(filters []QueryFilter) (bool, error) {
	if len(filters) == 0 {
		// Skip expensive work if there is nothing to filter-by.
		return true, nil
	}

	data := o.Data
	if data == nil {
		// Treat nil as "empty" data
		data = map[string]any{}
	} else if reflect.TypeOf(o.Data).Kind() != reflect.Map ||
		reflect.TypeOf(o.Data).Key().Kind() != reflect.String {
		// It's most likely for our use case that the data is a map[string]interface{}. However, if it's not
		// then we need to convert This is basically just here for safety and completeness.
		data = map[string]any{}
		err := o.As(&data)
		if err != nil {
			return false, err
		}
	}

	for _, filter := range filters {
		value := reflect.ValueOf(data)
		fields := strings.Split(filter.Field, ".")
		for i, field := range fields {
			value = value.MapIndex(reflect.ValueOf(field))
			if i < len(fields)-1 {
				// Need to go further into the nested fields
				value = reflect.ValueOf(value.Interface())
			}
		}
		comparator := reflect.ValueOf(filter.Value)

		if value.Type().Kind() == reflect.Interface {
			// Unwrap interface{}
			value = reflect.ValueOf(value.Interface())
		}

		if value.Type().Kind() != reflect.String {
			// not a string, can't compare!
			return false, nil
		}

		if value.String() != comparator.String() {
			// not the same value!
			return false, nil
		}
	}

	return true, nil
}
