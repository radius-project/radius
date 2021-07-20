// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keys

import "strings"

const TagRadiusEnvironment = "rad-environment"
const TagRadiusApplication = "radius-application"
const TagRadiusComponent = "radius-component"

func HasRadiusEnvironmentTag(tags map[string]*string) bool {
	return HasTag(tags, TagRadiusEnvironment, "true")
}

func HasRadiusApplicationTag(tags map[string]*string, application string) bool {
	return HasTag(tags, TagRadiusApplication, application)
}

func HasRadiusComponentTag(tags map[string]*string, component string) bool {
	return HasTag(tags, TagRadiusComponent, component)
}

func HasTag(tags map[string]*string, key string, expectedValue string) bool {
	value, ok := tags[key]

	// For SOME REASON values in tags can get normalized or have their casing changed.
	// eg: 'true' in a tag gets normalized to 'True'
	//
	// So it's very intentional that we compare tags case-insensitively
	if ok && value != nil && strings.EqualFold(expectedValue, *value) {
		return true
	}
	return false
}

// HasTagSet returns true if all of the tags in expected are present in actual.
// Allows actual to define extra tags not present in expected.
func HasTagSet(actual map[string]*string, expected map[string]string) bool {
	for k, v := range expected {
		if !HasTag(actual, k, v) {
			return false
		}
	}

	return true
}

func MatchesRadiusComponent(tags map[string]*string, application string, component string) bool {
	return HasRadiusApplicationTag(tags, application) && HasRadiusComponentTag(tags, component)
}

func MakeTagsForRadiusComponent(application string, component string) map[string]*string {
	return map[string]*string{
		TagRadiusApplication: &application,
		TagRadiusComponent:   &component,
	}
}
