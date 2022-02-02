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
	Build    BuildStage         `yaml:"build,omitempty"`
	Profiles map[string]Profile `yaml:"profiles,omitempty"`
	Bicep    *BicepStage        `yaml:"bicep,omitempty"`
}

type BuildStage = map[string]*BuildTarget

// BuildTarget implements its own serialization so we can enforce some constraints.
type BuildTarget struct {
	Builder string
	Values  map[string]interface{}
}

// Profile represents an override profile for a stage.
type Profile struct {
	Build BuildStage  `yaml:"build,omitempty"`
	Bicep *BicepStage `yaml:"bicep,omitempty"`
}

// Bicep stage represents a Bicep deployment as part of a stage or profile.
type BicepStage struct {
	Template *string `yaml:"template,omitempty"`
}
