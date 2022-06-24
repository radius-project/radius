// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

const (
	EnvironmentsResource = "environments"
	ApplicationsResource = "applications"
)

type Resource struct {
	Type string
	Name string
}
