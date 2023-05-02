// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

func (o *Object) As(out any) error {
	return DecodeMap(o.Data, out)
}

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
