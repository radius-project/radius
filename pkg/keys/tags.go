// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keys

import "strings"

const TagRadiusEnvironment = "rad-environment"
const TagRadiusApplication = "radius-application"
const TagRadiusResource = "radius-resource"

func HasRadiusEnvironmentTag(tags map[string]*string) bool {
	return HasTag(tags, TagRadiusEnvironment, "true")
}

func HasRadiusApplicationTag(tags map[string]*string, application string) bool {
	return HasTag(tags, TagRadiusApplication, application)
}

func HasRadiusResourceTag(tags map[string]*string, resource string) bool {
	return HasTag(tags, TagRadiusResource, resource)
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

func MatchesRadiusResource(tags map[string]*string, application string, resource string) bool {
	return HasRadiusApplicationTag(tags, application) && HasRadiusResourceTag(tags, resource)
}

func MakeTagsForRadiusResource(application string, resource string) map[string]*string {
	return map[string]*string{
		TagRadiusApplication: &application,
		TagRadiusResource:    &resource,
	}
}
