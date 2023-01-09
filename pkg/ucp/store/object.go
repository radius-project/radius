// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

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
