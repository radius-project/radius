// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

import (
	"strings"

	"regexp"

	"github.com/project-radius/radius/pkg/cli"
)

func ValidateResourceGroupName(resourceGroupName string) error {

	if len(resourceGroupName) < 1 || len(resourceGroupName) > 90 {
		return &cli.FriendlyError{Message: "ResourceGroup name should be between 1 and 90 characters long."}
	}
	if strings.HasSuffix(resourceGroupName, ".") {
		return &cli.FriendlyError{Message: "ResourceGroupNames cannot end with period"}
	}

	allAllowedChars := regexp.MustCompile(`^[A-Za-z0-9-_(){}\[\]]+$`).MatchString

	if !allAllowedChars(resourceGroupName) {
		return &cli.FriendlyError{Message: "Resource group name can only contain alphanumerics, underscores, parentheses, hyphens, periods."}
	}

	return nil
}
