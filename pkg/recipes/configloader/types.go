// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// Configuration represents kubernetes runtime and cloud provider configuration, which is used by the driver while deploying recipes.
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
	// Namespace is set to the applicationNamespace when the Link is application-scoped, and set to the environmentNamespace when the Link is environment scoped
	Namespace string `json:"namespace,omitempty"`
	// EnvironmentNamespace is set to environment namespace.
	EnvironmentNamespace string `json:"environmentNamespace"`
}
