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
	ConfigFilePrefix = "rp-setting"

	RadiusDevEnvironment       = "dev"
	RadiusDogfood              = "df-westus3"
	RadiusCanaryEastUS2EUAP    = "prod-eastus2euap"
	RadiusCanaryCentralUS2EUAP = "prod-centralus2euap"
	RadiusProdPrefix           = "prod"
)

var currentEnv = RadiusDevEnvironment

func Environment() string {
	return currentEnv
}

func IsDevelopment() bool {
	return strings.HasPrefix(Environment(), RadiusDevEnvironment)
}

func IsDogfood() bool {
	return Environment() == RadiusDogfood
}

func IsCanary() bool {
	env := Environment()
	return env == RadiusCanaryEastUS2EUAP || env == RadiusCanaryCentralUS2EUAP
}

func IsProduction() bool {
	return !IsCanary() && strings.HasPrefix(Environment(), RadiusProdPrefix)
}

func init() {
	currentEnv = strings.TrimSpace(strings.ToLower(os.Getenv("RADIUS_ENV")))
	if currentEnv == "" {
		currentEnv = RadiusDevEnvironment
	}
}
