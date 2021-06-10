// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import "strings"

const TagRadiusEnvironment = "rad-environment"
const TagRadiusApplication = "radius-application"
const TagRadiusComponent = "radius-component"

func HasRadiusEnvironmentTag(tags map[string]*string) bool {
	value, ok := tags[TagRadiusEnvironment]

	// For SOME REASON the value 'true' in a tag gets normalized to 'True'
	if ok && value != nil && strings.EqualFold("true", *value) {
		return true
	}

	return false
}

func HasRadiusApplicationTag(tags map[string]*string, application string) bool {
	value, ok := tags[TagRadiusEnvironment]
	if ok && value != nil && strings.EqualFold(application, *value) {
		return true
	}

	return false
}
