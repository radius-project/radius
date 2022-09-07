// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package conv

// DataModelInterface is the interface for version agnostic datamodel.
type DataModelInterface interface {
	ResourceTypeName() string
}

// VersionedModelInterface is the interface for versioned models.
type VersionedModelInterface interface {
	// ConvertFrom converts version agnostic datamodel to versioned model.
	ConvertFrom(src DataModelInterface) error

	// ConvertTo converts versioned model to version agnostic datamodel.
	ConvertTo() (DataModelInterface, error)
}

// RequestConverter is the function to convert to data model.
type RequestConverter[T any] func(content []byte, version string) (*T, error)

// ResponseConverter is the function to convert data model to version agnostic model.
type ResponseConverter[T any] func(model *T, version string) (VersionedModelInterface, error)
