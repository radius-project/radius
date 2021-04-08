// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

// GetResourceManagerEndpointAuthorizer returns the authorizer for the ResourceManager endpoint
func GetResourceManagerEndpointAuthorizer() (autorest.Authorizer, error) {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return nil, err
	}

	return getArmAuthorizer(settings.Environment.ResourceManagerEndpoint)
}

// GetGraphEndpointAuthorizer returns the authorizer for the ResourceManager endpoint
func GetGraphEndpointAuthorizer() (autorest.Authorizer, error) {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return nil, err
	}

	return getArmAuthorizer(settings.Environment.GraphEndpoint)
}

func getArmAuthorizer(endpoint string) (autorest.Authorizer, error) {

	var authorizer autorest.Authorizer

	useServicePrincipal, err := IsServicePrincipalConfigured()
	if err != nil {
		return nil, err
	}

	if useServicePrincipal {
		// Use the service principal specified
		authorizer, err = auth.NewAuthorizerFromEnvironment()
		if err != nil {
			return nil, err
		}
	} else {
		authorizer, err = auth.NewAuthorizerFromCLIWithResource(endpoint)
		if err != nil {
			return nil, err
		}
	}

	return authorizer, nil
}

// IsServicePrincipalConfigured determines whether a service principal is specifed
func IsServicePrincipalConfigured() (bool, error) {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return false, err
	}

	spSpecified := settings.Values[auth.ClientID] != "" && settings.Values[auth.ClientSecret] != ""
	return spSpecified, nil
}

// GetResourceNameFromFullyQualifiedPath Returns the resource name from full path of the resource.
// For example returns 'default' for 'radius/frontend-backend/default'
func GetResourceNameFromFullyQualifiedPath(fullyQualifiedPath string) string {
	name := fullyQualifiedPath
	if strings.Contains(fullyQualifiedPath, "/") {
		split := strings.Split(fullyQualifiedPath, "/")
		name = split[len(split)-1]
	}

	return name
}
