// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validation

import (
	"fmt"
)

func (v *Validator) AssignBoolFromMap(destination *bool, field string, input map[string]interface{}, source string, options ...Option) bool {
	fo := FieldOptions{}
	for _, o := range options {
		o.Apply(&fo)
	}

	obj, ok := input[field]
	if !ok && fo.Optional {
		return false
	} else if !ok {
		msg := fmt.Sprintf("field %q is required but was not provided by %s", field, source)
		v.messages = append(v.messages, msg)
		return false
	}

	b, ok := obj.(bool)
	if !ok {
		msg := fmt.Sprintf("field %s provided by %s was expected to be bool but was %T", field, source, obj)
		v.messages = append(v.messages, msg)
		return false
	}

	*destination = b
	return true
}

func (v *Validator) AssignInt32FromMap(destination *int32, field string, input map[string]interface{}, source string, options ...Option) bool {
	fo := FieldOptions{}
	for _, o := range options {
		o.Apply(&fo)
	}

	obj, ok := input[field]
	if !ok && fo.Optional {
		return false
	} else if !ok {
		msg := fmt.Sprintf("field %q is required but was not provided by %s", field, source)
		v.messages = append(v.messages, msg)
		return false
	}

	i32, ok := obj.(int32)
	if !ok {
		// For integers we need to handle some conversion cases that appear with JSON. Since recipes frequently
		// use JSON as an interchange format, the value might appear as a float 64.
		fp, ok := obj.(float64)
		if !ok {
			msg := fmt.Sprintf("field %q provided by %s was expected to be integer but was %T", field, source, obj)
			v.messages = append(v.messages, msg)
			return false
		}

		i32 = int32(fp)
	}

	*destination = i32
	return true
}

func (v *Validator) AssignStringFromMap(destination *string, field string, input map[string]interface{}, source string, options ...Option) bool {
	fo := FieldOptions{}
	for _, o := range options {
		o.Apply(&fo)
	}

	obj, ok := input[field]
	if !ok && fo.Optional {
		return false
	} else if !ok {
		msg := fmt.Sprintf("field %q is required but was not provided by %s", field, source)
		v.messages = append(v.messages, msg)
		return false
	}

	str, ok := obj.(string)
	if !ok {
		msg := fmt.Sprintf("field %q provided by %s was expected to be string but was %T", field, source, obj)
		v.messages = append(v.messages, msg)
		return false
	}

	*destination = str
	return true
}
