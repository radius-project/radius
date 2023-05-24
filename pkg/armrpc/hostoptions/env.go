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
	RadiusDevEnvironment           = "dev"
	RadiusSelfHostedDevEnvironment = "self-hosted-dev"
	RadiusSelfHostedEnvironment    = "self-hosted"
	RadiusDogfood                  = "df-westus3"
	RadiusCanaryEastUS2EUAP        = "prod-eastus2euap"
	RadiusCanaryCentralUS2EUAP     = "prod-centralus2euap"
	RadiusProdPrefix               = "prod"
)

var currentEnv = RadiusDevEnvironment

// Environment returns the current environment name.
func Environment() string {
	return currentEnv
}

// IsDevelopment returns true if the current environment is development environment.
func IsDevelopment() bool {
	return strings.HasPrefix(Environment(), RadiusDevEnvironment) || strings.HasPrefix(Environment(), RadiusSelfHostedDevEnvironment)
}

// IsSelfHosted returns true if the current environment is self-hosted environment.
func IsSelfHosted() bool {
	return strings.HasPrefix(Environment(), RadiusSelfHostedEnvironment)
}

// IsDogfood returns true if the current environment is dogfood environment.
func IsDogfood() bool {
	return Environment() == RadiusDogfood
}

// IsCanary returns true if the current environment is canary region.
func IsCanary() bool {
	env := Environment()
	return env == RadiusCanaryEastUS2EUAP || env == RadiusCanaryCentralUS2EUAP
}

// IsProduction returns true if the current environment is production, but not canary.
func IsProduction() bool {
	return !IsCanary() && strings.HasPrefix(Environment(), RadiusProdPrefix)
}

func init() {
	currentEnv = strings.TrimSpace(strings.ToLower(os.Getenv("RADIUS_ENV")))
	if currentEnv == "" {
		currentEnv = RadiusDevEnvironment
	}
}
