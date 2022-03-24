// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

import (
	"os"
	"strings"
)

const (
	RadiusDevEnvironment       = "dev"
	RadiusDogfood              = "df-westus3"
	RadiusCanaryEastUS2EUAP    = "prod-eastus2euap"
	RadiusCanaryCentralUS2EUAP = "prod-centralus2euap"
	RadiusProdPrefix           = "prod"
)

var currentEnv = RadiusDevEnvironment

// Environment returns the current environment name.
func Environment() string {
	return currentEnv
}

// IsDevelopment returns true if the current environment is development environment.
func IsDevelopment() bool {
	return strings.HasPrefix(Environment(), RadiusDevEnvironment)
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
