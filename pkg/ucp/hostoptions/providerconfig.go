// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/rest"
)

// UCPConfig includes the resource provider configuration.
type UCPConfig struct {
	StorageProvider dataprovider.StorageProviderOptions `yaml:"storageProvider"`
	Planes          []rest.Plane                        `yaml:"planes"`
}
