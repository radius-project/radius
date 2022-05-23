// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"reflect"
)

func (o Object) MatchesFilters(filters []QueryFilter) (bool, error) {
	if len(filters) == 0 {
		// Skip expensive work if there is nothing to filter-by.
		return true, nil
	}

	data := o.Data
	if data == nil {
		// Treat nil as "empty" data
		data = map[string]interface{}{}
	} else if reflect.TypeOf(o.Data).Kind() != reflect.Map ||
		reflect.TypeOf(o.Data).Key().Kind() != reflect.String {
		// It's most likely for our use case that the data is a map[string]interface{}. However, if it's not
		// then we need to convert This is basically just here for safety and completeness.
		data = map[string]interface{}{}
		err := o.As(&data)
		if err != nil {
			return false, err
		}
	}

	wrapper := reflect.ValueOf(data)
	for _, filter := range filters {
		value := wrapper.MapIndex(reflect.ValueOf(filter.Field))
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
