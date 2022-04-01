package v20220315

import (
	"github.com/project-radius/radius/pkg/corerp/api/common"
)

// EnvironmentComputeKind is the type of compute resource.
type EnvironmentComputeKind string

const (
	// KubernetesComputeKind represents kubernetes compute resource type.
	KubernetesComputeKind EnvironmentComputeKind = "kubernetes"
)

// Environment represents Application environment resource.
type Environment struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	Type       string                      `json:"type"`
	Location   string                      `json:"location"`
	SystemData common.SystemDataProperties `json:"systemData,omitempty"`
	Tags       common.ResourceTags         `json:"tags,omitempty"`

	Properties EnvironmentProperties `json:"properties"`
}

// EnvironmentProperties represents the properties of Environment.
type EnvironmentProperties struct {
	ProvisioningState common.ProvisioningStates `json:"provisioningState,omitempty"`
	Compute           EnvironmentCompute        `json:"compute,omitempty"`
}

// EnvironmentCompute represents the compute resource of Environment.
type EnvironmentCompute struct {
	Kind       EnvironmentComputeKind `json:"kind"`
	ResourceID string                 `json:"resourceId"`
}
