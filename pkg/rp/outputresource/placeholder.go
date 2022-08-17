// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"github.com/xeipuuv/gojsonpointer"
)

// Placeholder represents a placeholder value in a resource that can get and set a value.
//
// Placeholders specify a source and destination using JSON-Pointer notation: https://datatracker.ietf.org/doc/html/rfc6901.
// Placeholders operate on weakly-typed JSON documents (map[string]interface{} and similar).
type Placeholder struct {
	// SourcePointer is the JSON-Pointer to a value in the source document.
	SourcePointer string

	// DestinationPointer is the JSON-Pointer to a value in the destination document.
	DestinationPointer string
}

// GetValue gets a go value from the provided document using SourcePointer. GetValue will return an error if the property
// pointed to by SourcePointer does not exist.
func (p Placeholder) GetValue(resource interface{}) (interface{}, error) {
	pointer, err := gojsonpointer.NewJsonPointer(p.SourcePointer)
	if err != nil {
		return nil, err
	}

	value, _, err := pointer.Get(resource)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// ApplyValue sets a go value on the provided document using DestinationPointer. SetValue can be used
// to set an existing property or create a new one. SetValue will return an error if any of the intermediate
// properties pointed to by DestinationPointer are missing.
func (p Placeholder) ApplyValue(resource interface{}, value interface{}) error {
	pointer, err := gojsonpointer.NewJsonPointer(p.DestinationPointer)
	if err != nil {
		return err
	}

	_, err = pointer.Set(resource, value)
	if err != nil {
		return err
	}

	return nil
}
