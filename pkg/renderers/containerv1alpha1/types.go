// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import "github.com/Azure/radius/pkg/model/components"

const (
	Kind = "radius.dev/Container@v1alpha1"
)

// ContainerComponent is the definition of the container component
type ContainerComponent struct {
	Name   string                         `json:"name"`
	Kind   string                         `json:"kind"`
	Config map[string]interface{}         `json:"config,omitempty"`
	Run    ContainerRun                   `json:"run,omitempty"`
	Uses   []components.GenericDependency `json:"uses,omitempty"`
	Traits []map[string]interface{}       `json:"traits,omitempty"`
}

// ContainerRun is the defintion of the run section of a container
type ContainerRun struct {
	Container ContainerRunContainer `json:"container"`
}

type ContainerRunContainer struct {
	Image string            `json:"image"`
	Env   map[string]string `json:"env,omitempty"`
}

const KindHTTP = "http"

// HTTPProvidesService is the definition of an 'http' binding for a container.
type HTTPBinding struct {
	Kind       string `json:"kind"`
	Port       *int   `json:"port"`
	TargetPort *int   `json:"targetPort"`
}

func (h HTTPBinding) GetEffectivePort() int {
	if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}

func (h HTTPBinding) GetEffectiveContainerPort() int {
	if h.TargetPort != nil {
		return *h.TargetPort
	} else if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}
