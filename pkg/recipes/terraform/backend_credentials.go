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

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/ucp/credentials"
)

// resolveBackendAuth fetches the selected cloud's registered default credential, independently of the
// module's providers.
//
// Identity-based modes (AWS IRSA, Azure WorkloadIdentity) carry no secret, so they are returned for
// rendering into the generated backend block and nothing is written to the process environment. That
// keeps recipe provider authentication untouched. Secret-bearing modes (AWS AccessKey, Azure
// ServicePrincipal) must stay out of on-disk configuration, so they are delivered through the
// execution environment instead.
//
// The map belongs to this execution; neither registration nor os.Environ is mutated.
func (e executor) resolveBackendAuth(ctx context.Context, backend *datamodel.TerraformBackend, env map[string]string) (backends.CloudBackendAuth, error) {
	if err := backend.Validate(); err != nil {
		return backends.CloudBackendAuth{}, err
	}
	switch backend.Type {
	case backends.BackendS3:
		provider, err := e.awsCredentialProvider()
		if err != nil {
			return backends.CloudBackendAuth{}, fmt.Errorf("creating s3 backend credential provider: %w", err)
		}
		credential, err := provider.Fetch(ctx, credentials.AWSPublic, "default")
		if err != nil {
			return backends.CloudBackendAuth{}, fmt.Errorf("fetching registered default AWS credentials for s3 backend: %w", err)
		}
		return setAWSBackendAuth(credential, env)
	case backends.BackendAzureRM:
		provider, err := e.azureCredentialProvider()
		if err != nil {
			return backends.CloudBackendAuth{}, fmt.Errorf("creating azurerm backend credential provider: %w", err)
		}
		credential, err := provider.Fetch(ctx, credentials.AzureCloud, "default")
		if err != nil {
			return backends.CloudBackendAuth{}, fmt.Errorf("fetching registered default Azure credentials for azurerm backend: %w", err)
		}
		return setAzureBackendAuth(credential, env)
	default:
		return backends.CloudBackendAuth{}, fmt.Errorf("unsupported Terraform backend type")
	}
}

func setAWSBackendAuth(c *credentials.AWSCredential, env map[string]string) (backends.CloudBackendAuth, error) {
	// Reject rather than remove endpoints: providers share this environment, and removing
	// an emulator endpoint could silently redirect provider operations to real AWS.
	// This applies to both credential modes, because an override redirects state traffic
	// and the IRSA token exchange regardless of where authentication is configured.
	for _, key := range []string{
		"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3", "AWS_S3_ENDPOINT",
		"AWS_ENDPOINT_URL_STS", "AWS_STS_ENDPOINT",
	} {
		if env[key] != "" {
			return backends.CloudBackendAuth{}, fmt.Errorf("s3 backend does not support endpoint override %s; remove it from the execution environment before using Radius-managed S3 state storage", key)
		}
	}
	if c == nil {
		return backends.CloudBackendAuth{}, fmt.Errorf("s3 backend requires registered default AWS credentials")
	}
	switch c.Kind {
	case credentials.AWSIRSACredentialKind:
		// IRSA carries no secret, so it is rendered into the backend block and the process
		// environment is left alone for the recipe's providers.
		if c.IRSACredential == nil || strings.TrimSpace(c.IRSACredential.RoleARN) == "" {
			return backends.CloudBackendAuth{}, fmt.Errorf("s3 backend requires a registered AWS IRSA roleARN")
		}
		return backends.CloudBackendAuth{AWSRoleARN: c.IRSACredential.RoleARN}, nil
	case credentials.AWSAccessKeyCredentialKind:
		if c.AccessKeyCredential == nil || strings.TrimSpace(c.AccessKeyCredential.AccessKeyID) == "" || strings.TrimSpace(c.AccessKeyCredential.SecretAccessKey) == "" {
			return backends.CloudBackendAuth{}, fmt.Errorf("s3 backend requires a registered AWS AccessKey with accessKeyID and secretAccessKey")
		}
	default:
		return backends.CloudBackendAuth{}, fmt.Errorf("s3 backend supports only registered AWS AccessKey or IRSA credentials")
	}

	// AccessKey holds a secret, which must not be written into on-disk backend configuration.
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN",
		"AWS_ACCESS_KEY", "AWS_SECRET_KEY", "AWS_PROFILE", "AWS_DEFAULT_PROFILE",
		"AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
	} {
		delete(env, key)
	}
	// Do not let a host's shared profile select different credentials.
	env["AWS_SHARED_CREDENTIALS_FILE"] = os.DevNull
	env["AWS_CONFIG_FILE"] = os.DevNull
	env["AWS_EC2_METADATA_DISABLED"] = "true"
	env["AWS_ACCESS_KEY_ID"] = c.AccessKeyCredential.AccessKeyID
	env["AWS_SECRET_ACCESS_KEY"] = c.AccessKeyCredential.SecretAccessKey
	return backends.CloudBackendAuth{}, nil
}

func setAzureBackendAuth(c *credentials.AzureCredential, env map[string]string) (backends.CloudBackendAuth, error) {
	if c == nil {
		return backends.CloudBackendAuth{}, fmt.Errorf("azurerm backend requires registered default Azure credentials")
	}
	switch c.Kind {
	case credentials.AzureWorkloadIdentityCredentialKind:
		// WorkloadIdentity carries no secret, so it is rendered into the backend block and the
		// process environment is left alone for the recipe's providers.
		if c.WorkloadIdentity == nil || strings.TrimSpace(c.WorkloadIdentity.ClientID) == "" || strings.TrimSpace(c.WorkloadIdentity.TenantID) == "" {
			return backends.CloudBackendAuth{}, fmt.Errorf("azurerm backend requires a registered Azure WorkloadIdentity with clientID and tenantID")
		}
		return backends.CloudBackendAuth{
			AzureClientID:    c.WorkloadIdentity.ClientID,
			AzureTenantID:    c.WorkloadIdentity.TenantID,
			AzureEnvironment: env["ARM_ENVIRONMENT"],
		}, nil
	case credentials.AzureServicePrincipalCredentialKind:
		if c.ServicePrincipal == nil || strings.TrimSpace(c.ServicePrincipal.ClientID) == "" ||
			strings.TrimSpace(c.ServicePrincipal.TenantID) == "" || strings.TrimSpace(c.ServicePrincipal.ClientSecret) == "" {
			return backends.CloudBackendAuth{}, fmt.Errorf("azurerm backend requires a registered Azure ServicePrincipal with clientID, tenantID and clientSecret")
		}
	default:
		return backends.CloudBackendAuth{}, fmt.Errorf("azurerm backend supports only registered Azure ServicePrincipal or WorkloadIdentity credentials")
	}

	// ServicePrincipal holds a secret, which must not be written into on-disk backend configuration.
	// ARM_ENVIRONMENT is deliberately preserved: it selects the Azure cloud rather than an
	// authentication mode, and clearing it would silently retarget the backend and every
	// environment-configured provider at public Azure.
	for _, key := range []string{
		"ARM_CLIENT_ID", "ARM_CLIENT_ID_FILE_PATH", "ARM_TENANT_ID", "ARM_CLIENT_SECRET", "ARM_CLIENT_SECRET_FILE_PATH",
		"ARM_CLIENT_CERTIFICATE", "ARM_CLIENT_CERTIFICATE_PATH", "ARM_CLIENT_CERTIFICATE_PASSWORD",
		"ARM_ACCESS_KEY", "ARM_SAS_TOKEN", "ARM_OIDC_TOKEN", "ARM_OIDC_TOKEN_FILE_PATH",
		"ARM_OIDC_REQUEST_URL", "ARM_OIDC_REQUEST_TOKEN", "ARM_MSI_ENDPOINT",
		"ARM_ADO_PIPELINE_SERVICE_CONNECTION_ID",
	} {
		delete(env, key)
	}
	env["ARM_USE_AZUREAD"] = "true"
	env["ARM_USE_CLI"] = "false"
	env["ARM_USE_MSI"] = "false"
	env["ARM_USE_OIDC"] = "false"
	env["ARM_USE_AKS_WORKLOAD_IDENTITY"] = "false"
	env["ARM_CLIENT_ID"] = c.ServicePrincipal.ClientID
	env["ARM_TENANT_ID"] = c.ServicePrincipal.TenantID
	env["ARM_CLIENT_SECRET"] = c.ServicePrincipal.ClientSecret

	// ServicePrincipal authentication is delivered through the environment, but cleanup still needs
	// to know which Azure cloud the backend targets.
	return backends.CloudBackendAuth{AzureEnvironment: env["ARM_ENVIRONMENT"]}, nil
}
