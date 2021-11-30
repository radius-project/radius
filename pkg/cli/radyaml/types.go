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
	Name  string      `yaml:"name"`
	Bicep *BicepStage `yaml:"bicep,omitempty"`
}

type BicepStage struct {
	Template *string `yaml:"template,omitempty"`
}
