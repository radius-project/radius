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

package hostoptions

import (
	"os"
	"strings"
)

const (
	RadiusDevEnvironment        = "dev"
	RadiusSelfHostedEnvironment = "self-hosted"
)

var currentEnv = RadiusDevEnvironment

// Environment returns the current environment name. Can be configured by the RADIUS_ENV environment variables. Defaults to "dev" if not set.
func Environment() string {
	return currentEnv
}

// IsDevelopment returns true if the current environment is development environment.
func IsDevelopment() bool {
	return strings.HasPrefix(Environment(), RadiusDevEnvironment)
}

// IsSelfHosted returns true if the current environment is self-hosted environment.
func IsSelfHosted() bool {
	return strings.HasPrefix(Environment(), RadiusSelfHostedEnvironment)
}

func init() {
	currentEnv = strings.TrimSpace(strings.ToLower(os.Getenv("RADIUS_ENV")))
	if currentEnv == "" {
		currentEnv = RadiusDevEnvironment
	}
}
