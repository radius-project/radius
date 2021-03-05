// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

// GetArmAuthorizer returns the authorizer for ResourceManagerEndpoint and GraphEndpoint
func GetArmAuthorizer() (autorest.Authorizer, autorest.Authorizer, error) {

	var armauth autorest.Authorizer
	var graphauth autorest.Authorizer

	useServicePrincipal, err := UseServicePrincipal()
	if err != nil {
		return nil, nil, err
	}

	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return nil, nil, err
	}

	if useServicePrincipal {
		// Use the service principal specified
		armauth, err = auth.NewAuthorizerFromEnvironment()
		if err != nil {
			return nil, nil, err
		}

		graphauth, err = auth.NewAuthorizerFromEnvironment()
		if err != nil {
			return armauth, nil, err
		}
	} else {
		armauth, err = auth.NewAuthorizerFromCLIWithResource(settings.Environment.ResourceManagerEndpoint)
		if err != nil {
			return nil, nil, err
		}

		graphauth, err = auth.NewAuthorizerFromCLIWithResource(settings.Environment.GraphEndpoint)
		if err != nil {
			return armauth, nil, err
		}
	}

	return armauth, graphauth, nil
}

// UseServicePrincipal determines whether a service principal is specifed
func UseServicePrincipal() (bool, error) {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return false, err
	}

	spSpecified := settings.Values[auth.ClientID] != "" && settings.Values[auth.ClientSecret] != ""
	return spSpecified, nil
}
