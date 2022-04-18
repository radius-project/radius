// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

type ETag = string

type Metadata struct {
	ID              string
	ETag            ETag
	PaginationToken string
	APIVersion      string
	ContentType     string
}

type Object struct {
	Metadata
	Data interface{} // Data []byte
}
