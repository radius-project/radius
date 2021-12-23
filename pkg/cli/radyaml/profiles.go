// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"strings"
)

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

	build, err := CombineBuildStage(stage.Build, override.Build)
	if err != nil {
		return Stage{}, err
	}

	bicep, err := CombineBicepStage(stage.Bicep, override.Bicep)
	if err != nil {
		return Stage{}, err
	}

	copy.Build = build
	copy.Bicep = bicep
	return copy, nil
}

func CombineBuildStage(main BuildStage, override BuildStage) (BuildStage, error) {
	if main == nil {
		return override, nil
	}

	if override == nil {
		return main, nil
	}

	// If we get here, both stages define Build settings and we need to combine them.
	combined := BuildStage{}
	for key, value := range main {
		combined[key] = value
	}

	for key, value := range override {
		target, err := CombineBuildTarget(combined[key], value)
		if err != nil {
			return nil, err
		}

		combined[key] = target
	}

	return combined, nil
}

func CombineBuildTarget(main *BuildTarget, override *BuildTarget) (*BuildTarget, error) {
	if main == nil {
		return override, nil
	}

	if override == nil {
		return main, nil
	}

	if strings.EqualFold(main.Builder, override.Builder) {
		return &BuildTarget{
			Builder: main.Builder,
			Values:  overrideMap(main.Values, override.Values),
		}, nil
	}

	return override, nil
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

func overrideMap(main map[string]interface{}, override map[string]interface{}) map[string]interface{} {
	if override == nil {
		return main
	}

	if main == nil {
		return override
	}

	// If we get here both main and override are non-nil. We want to combine them recursively.
	combined := map[string]interface{}{}
	for key, value := range main {
		combined[key] = value
	}

	for key, value := range override {
		if mainValue, ok := combined[key]; ok {
			mainValueMap, isMainValueMap := mainValue.(map[string]interface{})
			overrideValueMap, isOverrideValueMap := value.(map[string]interface{})
			if isMainValueMap && isOverrideValueMap {
				combined[key] = overrideMap(mainValueMap, overrideValueMap)
				continue
			}
		}

		combined[key] = value
	}

	return combined
}
