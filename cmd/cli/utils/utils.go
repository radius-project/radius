// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/radclient"
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

// UnwrapErrorFromRawResponse raw http response into ErrorResponse format and builds
// an error message with error code, message and details.
// This is a temporary fix until we fix this through a combination of changes on server side error implementation
// and SDK Error interface implementation. https://github.com/Azure/radius/issues/243
func UnwrapErrorFromRawResponse(err error) error {
	var httpResp azcore.HTTPResponse
	ok := errors.As(err, &httpResp)
	if ok {
		respBytes, err := ioutil.ReadAll(httpResp.RawResponse().Body)
		if err != nil {
			return fmt.Errorf("failed to parse the response: %w", err)
		}

		var unwrappedError radclient.ErrorResponse
		err = json.Unmarshal(respBytes, &unwrappedError)
		if err != nil {
			return fmt.Errorf("failed to unmarshall the response %w", err)
		}

		return errors.New(GenerateErrorMessage(unwrappedError))
	}

	return err
}

// GenerateErrorMessage generates error message from InnerError
// Mostly replicated from Error interface implementation of generated model.
func GenerateErrorMessage(e radclient.ErrorResponse) string {
	msg := ""
	if e.InnerError != nil {
		msg += "Error: \n"
		if e.InnerError.Code != nil {
			msg += fmt.Sprintf("\tCode: %v\n", *e.InnerError.Code)
		}
		if e.InnerError.Message != nil {
			msg += fmt.Sprintf("\tMessage: %v\n", *e.InnerError.Message)
		}
		if e.InnerError.Target != nil {
			msg += fmt.Sprintf("\tTarget: %v\n", *e.InnerError.Target)
		}
		if e.InnerError.Details != nil {
			for _, value := range *e.InnerError.Details {
				if value.Message != nil {
					msg += fmt.Sprintf("\tDetails: %v\n", *value.Message)
				}
			}
		}
		if e.InnerError.AdditionalInfo != nil {
			msg += fmt.Sprintf("\tAdditionalInfo: %v\n", *e.InnerError.AdditionalInfo)
		}
	}
	if msg == "" {
		msg = "missing error info"
	}
	return msg
}

func GenerateResourceGroupUrl(subscriptionID string, resourceGroup string) (string) {
	
	rgUrl := "https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/resource/subscriptions/" + subscriptionID + 
		"/resourceGroups/" + resourceGroup + "/overview"
	return rgUrl

}