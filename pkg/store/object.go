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
	Data interface{} // Data []byte
}

// ObjectQueryResult represents the result of Query().
type ObjectQueryResult struct {
	// PaginationToken represents the token for pagination, such as continuation token.
	PaginationToken string
	// Items represents the list of documents.
	Items []Object
}
