// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

const (
	Bicep = "bicep"
)

// Configuration represent kubernetes runtime and cloud provider configuration, which is used by the drive while deploying recipes.
type Configuration struct {
	// Kubernetes Runtime configuration for the environment.
	Runtime RuntimeConfiguration
	// Cloud providers configuration for the environment
	Providers datamodel.Providers
}

type RuntimeConfiguration struct {
	Kubernetes *KubernetesRuntime `json:"kubernetes,omitempty"`
}

type KubernetesRuntime struct {
	Namespace string `json:"namespace,omitempty"`
}
