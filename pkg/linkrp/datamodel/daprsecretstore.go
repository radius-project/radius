// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// DaprSecretStore represents DaprSecretStore link resource.
type DaprSecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (daprSecretStore DaprSecretStore) ResourceTypeName() string {
	return "Applications.Link/daprSecretStores"
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	rp.BasicResourceProperties
	rp.BasicDaprResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Mode              LinkMode             `json:"mode"`
	Type              string               `json:"type"`
	Version           string               `json:"version"`
	Metadata          map[string]any       `json:"metadata"`
	Recipe            LinkRecipe           `json:"recipe,omitempty"`
}
