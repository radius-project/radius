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

type ObjectQueryResult struct {
	PaginationToken string
	Items           []Object
}
