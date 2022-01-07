// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import "strings"

func (stage Stage) ApplyProfile(profile string) (Stage, error) {
	var override *Profile
	for name, p := range stage.Profiles {
		if strings.EqualFold(name, profile) {
			copy := p
			override = &copy
			break
		}
	}

	// If there are no matching profiles to override, then we return the stage
	// as-is.
	if override == nil {
		return stage, nil
	}

	copy := stage
	bicep, err := CombineBicepStage(stage.Bicep, override.Bicep)
	if err != nil {
		return Stage{}, err
	}

	copy.Bicep = bicep
	return copy, nil
}

func CombineBicepStage(main *BicepStage, override *BicepStage) (*BicepStage, error) {
	if main == nil {
		return override, nil
	}

	if override == nil {
		return main, nil
	}

	// If we get here, both stages define Bicep settings and we need to combine them.
	combined := BicepStage{}
	combined.Template = overrideString(main.Template, override.Template)
	return &combined, nil
}

// Generics please :-/
func overrideString(main *string, override *string) *string {
	if override == nil {
		return main
	}

	return override
}
