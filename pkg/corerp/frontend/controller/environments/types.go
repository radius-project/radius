// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

const (
	ResourceTypeName = "Applications.Core/environments"

	// Supported operation names which are the unique names to process the operation request
	// in frontend API server and backend async operation process worker.
	EnvironmentList   = "APPLICATIONSCORE.ENVIRONMENT.LIST"
	EnvironmentGet    = "APPLICATIONSCORE.ENVIRONMENT.GET"
	EnvironmentPut    = "APPLICATIONSCORE.ENVIRONMENT.PUT"
	EnvironmentPatch  = "APPLICATIONSCORE.ENVIRONMENT.PATCH"
	EnvironmentDelete = "APPLICATIONSCORE.ENVIRONMENT.DELETE"
)
