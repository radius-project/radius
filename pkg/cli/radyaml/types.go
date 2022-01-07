// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

// Manifest represents the overall content of a rad.yaml.
type Manifest struct {
	Name   string  `yaml:"name"`
	Stages []Stage `yaml:"stages,omitempty"`
}

// Stage represents a stage of processing inside rad.yaml.
type Stage struct {
	Name     string             `yaml:"name"`
	Profiles map[string]Profile `yaml:"profiles,omitempty"`
	Bicep    *BicepStage        `yaml:"bicep,omitempty"`
}

// Profile represents an override profile for a stage.
type Profile struct {
	Bicep *BicepStage `yaml:"bicep,omitempty"`
}

// Bicep stage represents a Bicep deployment as part of a stage or profile.
type BicepStage struct {
	Template *string `yaml:"template,omitempty"`
}
