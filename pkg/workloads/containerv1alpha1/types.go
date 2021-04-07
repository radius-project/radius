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
	Provides  []ContainerProvides      `json:"provides,omitempty"`
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
	Name   string            `json:"name"`
	Kind   string            `json:"kind"`
	SetEnv map[string]string `json:"setEnv"`
	Set    map[string]string `json:"set"`
}

// ContainerProvides is the definition of the provides section
type ContainerProvides struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Port          *int   `json:"port"`
	ContainerPort *int   `json:"containerPort"`
}

// ContainerEnvVar is the definition of an environment variable
type ContainerEnvVar struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}
