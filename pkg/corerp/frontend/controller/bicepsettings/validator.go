/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package bicepsettings hosts request validators and other custom controller
// logic for Radius.Core/bicepSettings that the generic CRUD framework cannot
// express through TypeSpec alone.
package bicepsettings

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

const (
	authMethodBasicAuth = "BasicAuth"
	authMethodAzureWI   = "AzureWI"
	authMethodAwsIrsa   = "AwsIrsa"
)

// ValidateRequest enforces conditional-required-field constraints that TypeSpec
// cannot express on the union-style BicepRegistryAuthentication model:
//
//   - authenticationMethod=BasicAuth requires basicAuthSecretId
//   - authenticationMethod=AzureWI   requires azureWiClientId and azureWiTenantId
//   - authenticationMethod=AwsIrsa   requires awsIamRoleArn
//
// Without this hook the API would silently accept BicepSettings resources that
// reference no credentials, and the failure would surface much later at recipe
// execution time.
func ValidateRequest(ctx context.Context, newResource *datamodel.BicepSettings, oldResource *datamodel.BicepSettings, options *controller.Options) (rest.Response, error) {
	for host, auth := range newResource.Properties.RegistryAuthentications {
		if auth.AuthenticationMethod == "" {
			// AuthenticationMethod is itself optional in TypeSpec; if the user
			// omitted it we have nothing to validate.
			continue
		}
		switch auth.AuthenticationMethod {
		case authMethodBasicAuth:
			if auth.BasicAuthSecretId == "" {
				return rest.NewBadRequestResponse(fmt.Sprintf(
					"registryAuthentications[%q]: basicAuthSecretId is required when authenticationMethod is %q.",
					host, authMethodBasicAuth,
				)), nil
			}
		case authMethodAzureWI:
			if auth.AzureWiClientId == "" || auth.AzureWiTenantId == "" {
				return rest.NewBadRequestResponse(fmt.Sprintf(
					"registryAuthentications[%q]: azureWiClientId and azureWiTenantId are required when authenticationMethod is %q.",
					host, authMethodAzureWI,
				)), nil
			}
		case authMethodAwsIrsa:
			if auth.AwsIamRoleArn == "" {
				return rest.NewBadRequestResponse(fmt.Sprintf(
					"registryAuthentications[%q]: awsIamRoleArn is required when authenticationMethod is %q.",
					host, authMethodAwsIrsa,
				)), nil
			}
		default:
			return rest.NewBadRequestResponse(fmt.Sprintf(
				"registryAuthentications[%q]: unsupported authenticationMethod %q. Expected one of: %s, %s, %s.",
				host, auth.AuthenticationMethod, authMethodBasicAuth, authMethodAzureWI, authMethodAwsIrsa,
			)), nil
		}
	}

	return nil, nil
}
