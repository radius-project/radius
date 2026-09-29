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
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	azcredential "github.com/radius-project/radius/pkg/azure/credential"
	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// deleteCloudState removes the state object Terraform leaves behind after destroy, so that deleting a
// Radius resource does not accumulate empty objects in the user's bucket or container.
//
// It is best effort by design. The resources it tracked are already destroyed by the time this runs, so
// a cleanup failure is an operator cleanup task rather than a reason to fail the Radius delete and leave
// the resource undeletable. Failures are logged with the state key so the object can be removed manually.
func (e *executor) deleteCloudState(ctx context.Context, settings *datamodel.TerraformBackend, auth backends.CloudBackendAuth, key string) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if settings == nil || key == "" {
		return
	}

	// The best-effort guarantee has to hold against panics too, not just returned errors. A panic
	// unwinding through Delete would leave the Radius resource undeletable even though everything it
	// tracked is already destroyed.
	defer func() {
		if r := recover(); r != nil {
			logger.Info(fmt.Sprintf("Recovered from panic while deleting Terraform %s state object %q after destroy; the object can be removed manually: %v", settings.Type, key, r))
		}
	}()

	del := e.deleteStateObject
	if del == nil {
		del = e.deleteCloudStateObject
	}
	if err := del(ctx, settings, auth, key); err != nil {
		logger.Info(fmt.Sprintf("Unable to delete Terraform %s state object %q after destroy; the resources it tracked were destroyed and the object can be removed manually: %s", settings.Type, key, err.Error()))
		return
	}
	logger.Info(fmt.Sprintf("Deleted Terraform %s state object %q after destroy", settings.Type, key))
}

func (e *executor) deleteCloudStateObject(ctx context.Context, settings *datamodel.TerraformBackend, auth backends.CloudBackendAuth, key string) error {
	switch settings.Type {
	case backends.BackendS3:
		return e.deleteS3State(ctx, settings, key)
	case backends.BackendAzureRM:
		return e.deleteAzureState(ctx, settings, auth, key)
	default:
		return fmt.Errorf("unsupported Terraform backend type %q", settings.Type)
	}
}

func (e *executor) deleteS3State(ctx context.Context, settings *datamodel.TerraformBackend, key string) error {
	provider, err := e.awsCredentialProvider()
	if err != nil {
		return err
	}

	// Reuse UCP's credential provider so cleanup authenticates exactly like the backend did,
	// including the IRSA token exchange.
	client := s3.New(s3.Options{
		Region:      settings.Region,
		Credentials: aws.NewCredentialsCache(ucpaws.NewUCPCredentialProvider(provider, 0)),
	})

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(settings.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// S3 DeleteObject is idempotent, but treat an explicit not-found as success for clarity.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound") {
			return nil
		}
		return err
	}
	return nil
}

func (e *executor) deleteAzureState(ctx context.Context, settings *datamodel.TerraformBackend, auth backends.CloudBackendAuth, key string) error {
	// Resolve the cloud from the same variable the azurerm backend uses, so cleanup always targets
	// the endpoint Terraform actually wrote to. Guessing public Azure here would silently orphan
	// state on every sovereign-cloud installation.
	cloudConfig, storageSuffix, err := azureCloudForEnvironment(auth.AzureEnvironment)
	if err != nil {
		return err
	}

	provider, err := e.azureCredentialProvider()
	if err != nil {
		return err
	}

	// TokenFilePath must match the path rendered into the backend block. Without it the credential
	// falls back to AZURE_FEDERATED_TOKEN_FILE, which Radius no longer sets for Workload Identity.
	credential, err := azcredential.NewUCPCredential(azcredential.UCPCredentialOptions{
		Provider:      provider,
		TokenFilePath: providers.AzureOIDCTokenFilePath,
		ClientOptions: &azcore.ClientOptions{Cloud: cloudConfig},
	})
	if err != nil {
		return err
	}

	blobURL := fmt.Sprintf("https://%s.%s/%s/%s", settings.StorageAccountName, storageSuffix, settings.ContainerName, key)
	client, err := blob.NewClient(blobURL, azcore.TokenCredential(credential), &blob.ClientOptions{
		ClientOptions: azcore.ClientOptions{Cloud: cloudConfig},
	})
	if err != nil {
		return err
	}

	if _, err := client.Delete(ctx, nil); err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound, bloberror.ContainerNotFound) {
			return nil
		}
		return err
	}
	return nil
}

// azureCloudForEnvironment maps an ARM_ENVIRONMENT value to the matching azcore cloud configuration
// and blob storage DNS suffix. The accepted spellings mirror the azurerm backend's own environment
// names so that cleanup and Terraform never disagree about which cloud is in use.
//
// An unrecognised value is an error rather than a fallback to public Azure: deleting from the wrong
// cloud would either fail confusingly or, worse, target an unrelated account name in another cloud.
func azureCloudForEnvironment(environment string) (cloud.Configuration, string, error) {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "", "public", "azurepubliccloud", "azurecloud":
		return cloud.AzurePublic, "blob.core.windows.net", nil
	case "usgovernment", "usgovernmentcloud", "azureusgovernmentcloud", "azureusgovernment":
		return cloud.AzureGovernment, "blob.core.usgovcloudapi.net", nil
	case "china", "chinacloud", "azurechinacloud":
		return cloud.AzureChina, "blob.core.chinacloudapi.cn", nil
	default:
		return cloud.Configuration{}, "", fmt.Errorf("unsupported ARM_ENVIRONMENT %q for azurerm backend state cleanup", environment)
	}
}

// awsCredentialProvider returns the configured provider, or builds the default UCP-backed one.
func (e *executor) awsCredentialProvider() (credentials.CredentialProvider[credentials.AWSCredential], error) {
	if e.awsCredentials != nil {
		return e.awsCredentials, nil
	}
	return credentials.NewAWSCredentialProvider(e.secretProvider, e.ucpConn, &tokencredentials.AnonymousCredential{})
}

// azureCredentialProvider returns the configured provider, or builds the default UCP-backed one.
func (e *executor) azureCredentialProvider() (credentials.CredentialProvider[credentials.AzureCredential], error) {
	if e.azureCredentials != nil {
		return e.azureCredentials, nil
	}
	return credentials.NewAzureCredentialProvider(e.secretProvider, e.ucpConn, &tokencredentials.AnonymousCredential{})
}
