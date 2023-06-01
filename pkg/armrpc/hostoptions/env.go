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
//
// # Function Explanation
// 
//	Environment() returns the current environment as a string, and will panic if the environment is not set.
func Environment() string {
	return currentEnv
}

// IsDevelopment returns true if the current environment is development environment.
//
// # Function Explanation
// 
//	IsDevelopment() checks the environment variable and returns true if it is a development environment. If an error occurs 
//	while retrieving the environment variable, it will be logged and false will be returned.
func IsDevelopment() bool {
	return strings.HasPrefix(Environment(), RadiusDevEnvironment) || strings.HasPrefix(Environment(), RadiusSelfHostedDevEnvironment)
}

// IsSelfHosted returns true if the current environment is self-hosted environment.
//
// # Function Explanation
// 
//	IsSelfHosted() checks if the environment is a self-hosted environment and returns a boolean value. If an error occurs, 
//	it will be returned as part of the boolean value.
func IsSelfHosted() bool {
	return strings.HasPrefix(Environment(), RadiusSelfHostedEnvironment)
}

// IsDogfood returns true if the current environment is dogfood environment.
//
// # Function Explanation
// 
//	"IsDogfood" checks the environment and returns true if it is the Radius Dogfood environment, otherwise it returns false.
//	 If an error occurs, it will be logged and the function will return false.
func IsDogfood() bool {
	return Environment() == RadiusDogfood
}

// IsCanary returns true if the current environment is canary region.
//
// # Function Explanation
// 
//	IsCanary() checks the environment and returns true if it is either RadiusCanaryEastUS2EUAP or 
//	RadiusCanaryCentralUS2EUAP, otherwise it returns false. If an unexpected environment is encountered, the function will 
//	panic.
func IsCanary() bool {
	env := Environment()
	return env == RadiusCanaryEastUS2EUAP || env == RadiusCanaryCentralUS2EUAP
}

// IsProduction returns true if the current environment is production, but not canary.
//
// # Function Explanation
// 
//	IsProduction() checks if the environment is a production environment, and returns true if it is. It checks if the 
//	environment is not a canary environment and if it starts with the prefix "RadiusProdPrefix". If either of these 
//	conditions are not met, it returns false.
func IsProduction() bool {
	return !IsCanary() && strings.HasPrefix(Environment(), RadiusProdPrefix)
}

func init() {
	currentEnv = strings.TrimSpace(strings.ToLower(os.Getenv("RADIUS_ENV")))
	if currentEnv == "" {
		currentEnv = RadiusDevEnvironment
	}
}
