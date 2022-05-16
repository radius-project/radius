// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

// RedisCache represents RedisCache connector resource.
type RedisCache struct {
	basedatamodel.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties RedisCacheProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	basedatamodel.InternalMetadata
}

func (redis RedisCache) ResourceTypeName() string {
	return "Applications.Connector/redisCaches"
}

// RedisCacheProperties represents the properties of RedisCache resource.
type RedisCacheProperties struct {
	ProvisioningState basedatamodel.ProvisioningStates `json:"provisioningState,omitempty"`
	Environment       string                           `json:"environment"`
	Application       string                           `json:"application,omitempty"`
	Resource          string                           `json:"resource,omitempty"`
	Host              string                           `json:"host,omitempty"`
	Port              int32                            `json:"port,omitempty"`
	Secrets           RedisSecrets                     `json:"secrets,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RedisSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
}
