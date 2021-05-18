// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

const Kind = "radius.dev/Container@v1alpha1"

// ContainerComponent is the definition of the container component
type ContainerComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    map[string]interface{}   `json:"config,omitempty"`
	Run       ContainerRun             `json:"run,omitempty"`
	DependsOn []ContainerDependsOn     `json:"dependson,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// ContainerRun is the defintion of the run section of a container
type ContainerRun struct {
	Container ContainerRunContainer `json:"container"`
}

type ContainerRunContainer struct {
	Image       string            `json:"image"`
	Environment []ContainerEnvVar `json:"env,omitempty"`
}

// ContainerDependsOn is the definition of the dependsOn section
type ContainerDependsOn struct {
	Name      string                 `json:"name"`
	Kind      string                 `json:"kind"`
	SetEnv    map[string]string      `json:"setEnv"`
	SetSecret map[string]interface{} `json:"setSecret"`
}

// ContainerEnvVar is the definition of an environment variable
type ContainerEnvVar struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

const KindHTTP = "http"

// HTTPProvidesService is the definition of an 'http' service for a container.
type HTTPProvidesService struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Port          *int   `json:"port"`
	ContainerPort *int   `json:"containerPort"`
}

func (h HTTPProvidesService) GetEffectivePort() int {
	if h.Port != nil {
		return *h.Port
	} else if h.ContainerPort != nil {
		return *h.ContainerPort
	} else {
		return 80
	}
}

func (h HTTPProvidesService) GetEffectiveContainerPort() int {
	if h.ContainerPort != nil {
		return *h.ContainerPort
	} else if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}
