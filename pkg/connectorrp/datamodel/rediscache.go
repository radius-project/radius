// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// RedisCache represents RedisCache connector resource.
type RedisCache struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties RedisCacheProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata
}

func (redis RedisCache) ResourceTypeName() string {
	return "Applications.Connector/redisCaches"
}

// RedisCacheProperties represents the properties of RedisCache resource.
type RedisCacheProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Environment       string               `json:"environment"`
	Application       string               `json:"application,omitempty"`
	Resource          string               `json:"resource,omitempty"`
	Host              string               `json:"host,omitempty"`
	Port              int32                `json:"port,omitempty"`
	Secrets           RedisCacheSecrets    `json:"secrets,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RedisCacheSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
}

func (mongo RedisCacheSecrets) ResourceTypeName() string {
	return "Applications.Connector/mongoDatabases"
}
