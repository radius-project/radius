package featureflag

import "os"

type Flag string

const (
	EnableBicepExtensibility Flag = "RAD_FF_ENABLE_BICEP_EXTENSIBILITY"
	EnableUnifiedControlPlane Flag = "RAD_FF_ENABLE_UCP"
)

// IsActive returns if true if a feature flag is set, and false otherwise
func (f Flag) IsActive() bool {
	_, varFound := os.LookupEnv(string(f))
	return varFound
}
