// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

type Manifest struct {
	Name   string  `yaml:"name"`
	Stages []Stage `yaml:"stages,omitempty"`
}

type Stage struct {
	Name   string       `yaml:"name"`
	Deploy *DeployStage `yaml:"deploy,omitempty"`
}

type DeployStage struct {
	Bicep  *string                `yaml:"bicep,omitempty"`
	Params []DeployStageParameter `yaml:"params,omitempty"`
}

type DeployStageParameter struct {
	Name      string          `yaml:"name,omitempty"`
	Container *ContainerBuild `yaml:"container,omitempty"`
	NPM       *NPMBuild       `yaml:"npm,omitempty"`
}

type ContainerBuild = map[string]interface{}

type NPMBuild struct {
	Directory string             `yaml:"directory"`
	Script    string             `yaml:"script"`
	Args      []string           `yaml:"args,omitempty"`
	Container *NPMBuildContainer `yaml:"container,omitempty"`
}

type NPMBuildContainer struct {
	Image string `yaml:"image"`
}
