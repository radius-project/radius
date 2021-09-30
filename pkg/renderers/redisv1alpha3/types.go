// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

const (
	Port         = 6379
	ResourceType = "redislabs.com.RedisComponent"
)

// RedisComponentProperties is the defintion of the config section
type RedisComponentProperties struct {
	Managed bool `json:"managed"`
}
