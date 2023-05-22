/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"strings"

	"regexp"

	"github.com/project-radius/radius/pkg/cli"
)

func ValidateResourceGroupName(resourceGroupName string) error {

	if len(resourceGroupName) < 1 || len(resourceGroupName) > 90 {
		return &cli.FriendlyError{Message: "Resource group name should be between 1 and 90 characters long."}
	}
	if strings.HasSuffix(resourceGroupName, ".") {
		return &cli.FriendlyError{Message: "Resource group names cannot end with a period"}
	}

	allAllowedChars := regexp.MustCompile(`^[A-Za-z0-9-_(){}\[\]]+$`).MatchString

	if !allAllowedChars(resourceGroupName) {
		return &cli.FriendlyError{Message: "Resource group name can only contain alphanumerics, underscores, parentheses, hyphens, periods"}
	}

	return nil
}
