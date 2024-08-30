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

package authClient

import (
	"context"
	"errors"
	"net/url"

	"github.com/radius-project/radius/pkg/recipes"
	"oras.land/oras-go/v2/registry/remote"
)

//go:generate mockgen -typed -destination=./mock_authClient.go -package=authClient -self_package github.com/radius-project/radius/pkg/rp/util/authClient github.com/radius-project/radius/pkg/rp/util/authClient AuthClient
type AuthClient interface {
	GetAuthClient(ctx context.Context, templatePath string) (remote.Client, error)
}

func GetNewRegistryAuthClient(secrets recipes.SecretData) (AuthClient, error) {
	secretsData := secrets.Data
	switch secrets.Type {
	case "awsIRSA":
		return NewAwsIRSA(secretsData["roleARN"]), nil
	case "azureWorkloadIdentity":
		return NewAzureWorkloadIdentity(secretsData["clientId"], secretsData["tenantId"]), nil
	case "basicAuthentication":
		return NewBasicAuthentication(secretsData["username"], secretsData["password"]), nil
	default:
		return nil, errors.New("invalid type")
	}

}

func getRegistryHostname(templatePath string) (string, error) {
	registryURL, err := url.Parse("https://" + templatePath)
	if err != nil {
		return "", err
	}
	return registryURL.Host, nil
}
