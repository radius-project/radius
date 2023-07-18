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
	"context"
)

type ETag = string

type Metadata struct {
	ID          string
	ETag        ETag
	APIVersion  string
	ContentType string
}

type Object struct {
	Metadata

	// Data is the payload of the object. It will be marshaled to and from JSON for storage.
	Data any
}

// ObjectQueryResult represents the result of Query().
type ObjectQueryResult struct {
	// PaginationToken represents the token for pagination, such as continuation token.
	PaginationToken string
	// Items represents the list of documents.
	Items []Object
}

// # Function Explanation
//
// As decodes the Data field of the Object instance into the out parameter.
func (o *Object) As(out any) error {
	return DecodeMap(o.Data, out)
}

// # Function Explanation
//
// GetResource gets the resource data from StorageClient for id.
func GetResource[T any](ctx context.Context, client StorageClient, id string) (*T, error) {
	var out T

	obj, err := client.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if err = obj.As(&out); err != nil {
		return nil, err
	}

	return &out, nil
}
