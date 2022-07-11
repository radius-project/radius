// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package ucphandler

import (
	ucpplanes "github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/planes"
	ucpresourcegroups "github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/resourcegroups"
)

type UCPHandlerOptions struct {
	BasePath string
	Address  string
}

// NewUCPHandler creates a new UCP handler
func NewUCPHandler(options UCPHandlerOptions) UCPHandler {
	return UCPHandler{
		Options: options,
		Planes: ucpplanes.NewPlanesUCPHandler(ucpplanes.Options{
			Address:  options.Address,
			BasePath: options.BasePath,
		}),
		ResourceGroups: ucpresourcegroups.NewResourceGroupsUCPHandler(ucpresourcegroups.Options{
			BasePath: options.BasePath,
		}),
	}
}

type UCPHandler struct {
	Options        UCPHandlerOptions
	Planes         ucpplanes.PlanesUCPHandler
	ResourceGroups ucpresourcegroups.ResourceGroupsUCPHandler
}
