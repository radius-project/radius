// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// DaprStateStore represents DaprStateStore link resource.
type DaprStateStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprStateStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (daprStateStore DaprStateStore) ResourceTypeName() string {
	return "Applications.Link/daprStateStores"
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	rp.BasicResourceProperties
	rp.BasicDaprResourceProperties
	ProvisioningState v1.ProvisioningState   `json:"provisioningState,omitempty"`
	Mode              LinkMode               `json:"mode,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Recipe            LinkRecipe             `json:"recipe,omitempty"`
	Resource          string                 `json:"resource,omitempty"`
	Type              string                 `json:"type,omitempty"`
	Version           string                 `json:"version,omitempty"`
}
