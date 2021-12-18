// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radclient

import (
	"encoding/json"

	"github.com/Azure/radius/pkg/radrp/rest"
)

// GetStatus returns the `.properties.status` element of a Radius resource. Will return null if it
// is not found.
func (r RadiusResource) GetStatus() (*rest.ResourceStatus, error) {
	obj, ok := r.Properties["status"]
	if !ok {
		return nil, nil
	}

	b, err := json.Marshal(&obj)
	if err != nil {
		return nil, err
	}

	status := rest.ResourceStatus{}
	err = json.Unmarshal(b, &status)
	if err != nil {
		return nil, err
	}

	return &status, nil
}
