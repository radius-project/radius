// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rad

import "github.com/spf13/viper"

// EnvironmentKey is the key used for the environment section
const EnvironmentKey string = "environment"

// EnvironmentSection is the representation of the environment section of elbow config.
type EnvironmentSection struct {
	Default string                            `mapstructure:"default" yaml:"default"`
	Items   map[string]map[string]interface{} `mapstructure:"items" yaml:"items"`
}

// ReadEnvironmentSection reads the EnvironmentSection from elbow config.
func ReadEnvironmentSection(v *viper.Viper) (EnvironmentSection, error) {
	section := EnvironmentSection{}

	s := v.Sub(EnvironmentKey)
	if s == nil {
		section.Items = map[string]map[string]interface{}{}
		return section, nil
	}

	err := s.UnmarshalExact(&section)
	if err != nil {
		return EnvironmentSection{}, nil
	}

	return section, nil
}

// UpdateEnvironmentSection updates the EnvironmentSection in elbow config.
func UpdateEnvironmentSection(v *viper.Viper, env EnvironmentSection) {
	v.Set(EnvironmentKey, env)
}
