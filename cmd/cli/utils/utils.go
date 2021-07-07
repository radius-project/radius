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
	"github.com/Azure/radius/pkg/radclient"
)

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
	if e.InnerError == nil {
		return "missing error info"
	}
	b, _ := json.MarshalIndent(e.InnerError, "", "\t")
	return string(b)
}

// GenerateEnvUrl Returns the URL string for an environment based on its subscriptionID and resourceGroup.
// Uses environment kind to determine how which kind-specific function should build the URL string.
func GenerateEnvUrl(kind, subscriptionID string, resourceGroup string) string {
	envUrl := ""
	if kind == "azure" {
		envUrl = generateEnvUrlAzure(subscriptionID, resourceGroup)
	} else {
		envUrl = "Env URL unknown."
	}

	return envUrl
}

// generateEnvUrlAzure Returns Returns the URL string for an Azure environment.
func generateEnvUrlAzure(subscriptionID string, resourceGroup string) string {

	envUrl := "https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/resource/subscriptions/" +
		subscriptionID + "/resourceGroups/" + resourceGroup + "/overview"

	return envUrl
}
