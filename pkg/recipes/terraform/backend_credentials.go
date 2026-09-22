/*
Copyright 2026 The Radius Authors.

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

package terraform

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/credentials"
)

const (
	awsBackendTokenFile   = "/var/run/secrets/eks.amazonaws.com/serviceaccount/token"
	azureBackendTokenFile = "/var/run/secrets/azure/tokens/azure-identity-token"
)

// setBackendEnvironment fetches the selected cloud independently of the module's providers.
// The map belongs to this execution; neither registration nor os.Environ is mutated.
func (e executor) setBackendEnvironment(ctx context.Context, backend *datamodel.TerraformBackend, env map[string]string) error {
	if err := backend.Validate(); err != nil {
		return err
	}
	switch backend.Type {
	case "s3":
		provider := e.awsCredentials
		if provider == nil {
			var err error
			provider, err = credentials.NewAWSCredentialProvider(e.secretProvider, e.ucpConn, &tokencredentials.AnonymousCredential{})
			if err != nil {
				return fmt.Errorf("creating s3 backend credential provider: %w", err)
			}
		}
		credential, err := provider.Fetch(ctx, credentials.AWSPublic, "default")
		if err != nil {
			return fmt.Errorf("fetching registered default AWS credentials for s3 backend: %w", err)
		}
		return setAWSBackendEnvironment(credential, env)
	case "azurerm":
		provider := e.azureCredentials
		if provider == nil {
			var err error
			provider, err = credentials.NewAzureCredentialProvider(e.secretProvider, e.ucpConn, &tokencredentials.AnonymousCredential{})
			if err != nil {
				return fmt.Errorf("creating azurerm backend credential provider: %w", err)
			}
		}
		credential, err := provider.Fetch(ctx, credentials.AzureCloud, "default")
		if err != nil {
			return fmt.Errorf("fetching registered default Azure credentials for azurerm backend: %w", err)
		}
		return setAzureBackendEnvironment(credential, env)
	default:
		return fmt.Errorf("unsupported Terraform backend type")
	}
}

func setAWSBackendEnvironment(c *credentials.AWSCredential, env map[string]string) error {
	if c == nil {
		return fmt.Errorf("s3 backend requires registered default AWS credentials")
	}
	values := map[string]string{}
	switch c.Kind {
	case credentials.AWSAccessKeyCredentialKind:
		if c.AccessKeyCredential == nil || strings.TrimSpace(c.AccessKeyCredential.AccessKeyID) == "" || strings.TrimSpace(c.AccessKeyCredential.SecretAccessKey) == "" {
			return fmt.Errorf("s3 backend requires a registered AWS AccessKey with accessKeyID and secretAccessKey")
		}
		values["AWS_ACCESS_KEY_ID"] = c.AccessKeyCredential.AccessKeyID
		values["AWS_SECRET_ACCESS_KEY"] = c.AccessKeyCredential.SecretAccessKey
	case credentials.AWSIRSACredentialKind:
		if c.IRSACredential == nil || strings.TrimSpace(c.IRSACredential.RoleARN) == "" {
			return fmt.Errorf("s3 backend requires a registered AWS IRSA roleARN")
		}
		values["AWS_ROLE_ARN"] = c.IRSACredential.RoleARN
		values["AWS_WEB_IDENTITY_TOKEN_FILE"] = awsBackendTokenFile
		values["AWS_ROLE_SESSION_NAME"] = "radius-terraform-backend"
	default:
		return fmt.Errorf("s3 backend supports only registered AWS AccessKey or IRSA credentials")
	}
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN",
		"AWS_ACCESS_KEY", "AWS_SECRET_KEY", "AWS_PROFILE", "AWS_DEFAULT_PROFILE",
		"AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
	} {
		delete(env, key)
	}
	// Do not let a host's shared profile select different credentials, particularly for IRSA.
	env["AWS_SHARED_CREDENTIALS_FILE"] = os.DevNull
	env["AWS_CONFIG_FILE"] = os.DevNull
	env["AWS_EC2_METADATA_DISABLED"] = "true"
	for key, value := range values {
		env[key] = value
	}
	return nil
}

func setAzureBackendEnvironment(c *credentials.AzureCredential, env map[string]string) error {
	if c == nil {
		return fmt.Errorf("azurerm backend requires registered default Azure credentials")
	}
	values := map[string]string{
		"ARM_USE_AZUREAD":               "true",
		"ARM_USE_CLI":                   "false",
		"ARM_USE_MSI":                   "false",
		"ARM_USE_OIDC":                  "false",
		"ARM_USE_AKS_WORKLOAD_IDENTITY": "false",
	}
	switch c.Kind {
	case credentials.AzureServicePrincipalCredentialKind:
		if c.ServicePrincipal == nil || strings.TrimSpace(c.ServicePrincipal.ClientID) == "" ||
			strings.TrimSpace(c.ServicePrincipal.TenantID) == "" || strings.TrimSpace(c.ServicePrincipal.ClientSecret) == "" {
			return fmt.Errorf("azurerm backend requires a registered Azure ServicePrincipal with clientID, tenantID and clientSecret")
		}
		values["ARM_CLIENT_ID"] = c.ServicePrincipal.ClientID
		values["ARM_TENANT_ID"] = c.ServicePrincipal.TenantID
		values["ARM_CLIENT_SECRET"] = c.ServicePrincipal.ClientSecret
	case credentials.AzureWorkloadIdentityCredentialKind:
		if c.WorkloadIdentity == nil || strings.TrimSpace(c.WorkloadIdentity.ClientID) == "" || strings.TrimSpace(c.WorkloadIdentity.TenantID) == "" {
			return fmt.Errorf("azurerm backend requires a registered Azure WorkloadIdentity with clientID and tenantID")
		}
		values["ARM_CLIENT_ID"] = c.WorkloadIdentity.ClientID
		values["ARM_TENANT_ID"] = c.WorkloadIdentity.TenantID
		values["ARM_USE_OIDC"] = "true"
		values["ARM_OIDC_TOKEN_FILE_PATH"] = azureBackendTokenFile
	default:
		return fmt.Errorf("azurerm backend supports only registered Azure ServicePrincipal or WorkloadIdentity credentials")
	}
	for _, key := range []string{
		"ARM_CLIENT_ID", "ARM_CLIENT_ID_FILE_PATH", "ARM_TENANT_ID", "ARM_CLIENT_SECRET", "ARM_CLIENT_SECRET_FILE_PATH",
		"ARM_CLIENT_CERTIFICATE", "ARM_CLIENT_CERTIFICATE_PATH", "ARM_CLIENT_CERTIFICATE_PASSWORD",
		"ARM_ACCESS_KEY", "ARM_SAS_TOKEN", "ARM_OIDC_TOKEN", "ARM_OIDC_TOKEN_FILE_PATH",
		"ARM_OIDC_REQUEST_URL", "ARM_OIDC_REQUEST_TOKEN", "ARM_MSI_ENDPOINT",
		"ARM_ADO_PIPELINE_SERVICE_CONNECTION_ID", "ARM_ENVIRONMENT",
	} {
		delete(env, key)
	}
	for key, value := range values {
		env[key] = value
	}
	return nil
}
