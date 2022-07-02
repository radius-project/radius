// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

var (
	// TODO
	// Resources that will be processed asynchronously
	// Must come up with a better solution
	ResourceTypeNames = []string{
		"Applications.Core/containers",
		"Applications.Core/gateways",
		"Applications.Core/httproutes",
	}
)
