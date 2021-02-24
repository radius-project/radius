// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

// ContainerWorkload is the definition of the spec element of the workload
type ContainerWorkload struct {
	Container *ContainerSpec
	DependsOn []ContainerDependsOn
	Provides  []ContainerProvides
}

// ContainerProvides is the definition of the provides section
type ContainerProvides struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Port          *int   `json:"port"`
	ContainerPort *int   `json:"containerPort"`
}

// ContainerDependsOn is the definition of the dependsOn section
type ContainerDependsOn struct {
	Name   string            `json:"name"`
	Kind   string            `json:"kind"`
	SetEnv map[string]string `json:"setEnv"`
}

// ContainerSpec is the defintion of a container
type ContainerSpec struct {
	Image       string            `json:"image"`
	Environment []ContainerEnvVar `json:"env,omitempty"`
}

// ContainerEnvVar is the definition of an environment variable
type ContainerEnvVar struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}
