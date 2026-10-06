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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	azcredential "github.com/radius-project/radius/pkg/azure/credential"
	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	ucpaws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// errStateModifiedDuringCleanup reports that the state object changed between the end of destroy and
// the conditional delete, so another writer now owns it and cleanup deliberately left it in place.
// This is an expected outcome rather than a failure, and is logged differently from a cleanup error.
var errStateModifiedDuringCleanup = errors.New("state object changed after destroy completed")

// errStateNotEmptyDuringCleanup reports that the state read back after destroy positively tracks
// resources or outputs, so another writer committed live state before cleanup read it. Like a failed
// precondition this leaves the object in place and is an expected outcome rather than a failure.
var errStateNotEmptyDuringCleanup = errors.New("state object is not empty after destroy completed")

// errStateUnverifiableDuringCleanup reports that cleanup could not establish what the object holds,
// because it was unreadable, unparseable, implausibly large, or written in a state format Radius does
// not recognize. This is distinct from errStateNotEmptyDuringCleanup: Radius has not shown the object
// is live, only that it cannot show it is empty. Deleting on that basis is what would destroy state
// Radius does not own, so the object is kept and the operator is told to inspect it rather than to
// remove it.
var errStateUnverifiableDuringCleanup = errors.New("state object could not be verified as empty after destroy completed")

// errStateAlreadyAbsent reports that there was no object at the state key. Nothing was deleted, so it
// must not be logged as a deletion, but it is the expected outcome when a destroy left no state or an
// earlier cleanup already removed it.
var errStateAlreadyAbsent = errors.New("state object is already absent")

// maxEmptyStateBytes bounds how much of the state object cleanup reads back. A state Terraform has
// just emptied is well under a kilobyte, so anything larger cannot be one and is rejected without
// being buffered.
const maxEmptyStateBytes = 64 * 1024

// emptyStateFormatVersion is the state format Radius's pinned Terraform writes. Cleanup deletes only
// this version, because "empty" is decided from fields whose absence means something different in
// other formats; see verifyStateIsEmpty.
const emptyStateFormatVersion = 4

// terraformState is the subset of Terraform's state format cleanup needs. The format version
// identifies the document, and the tracked resources and recorded outputs decide whether it still
// represents live infrastructure.
type terraformState struct {
	Version   *int                       `json:"version"`
	Resources []json.RawMessage          `json:"resources"`
	Outputs   map[string]json.RawMessage `json:"outputs"`
}

// verifyStateIsEmpty reads the state object's body and reports whether it is safe to delete.
//
// Reading the content, rather than only its metadata, is what makes cleanup safe against a writer
// that committed live state after destroy released its lock but before cleanup looked at the object.
// An entity tag read alone cannot distinguish that state from the empty one destroy left: it would
// capture the new version and then delete it, because the conditional delete only covers the window
// between the metadata read and the delete itself. Verifying the version that was read is empty, and
// deleting only that version, closes both halves of the race.
//
// Emptiness is established positively, not by the absence of keys. The document must declare the
// state format Radius's Terraform writes before `resources` and `outputs` mean what this code assumes:
// a v3 state keeps live resources under a top-level `modules` array and has no top-level `resources`
// key at all, so treating a missing key as empty would delete live infrastructure's state. Unrelated
// JSON documents, `{}`, and `null` fail the same check.
//
// Anything cleanup cannot confirm is the empty state destroy wrote is left in place. Those cases are
// reported as errStateUnverifiableDuringCleanup rather than errStateNotEmptyDuringCleanup, because
// Radius has not observed live content and must not claim it has.
func verifyStateIsEmpty(body io.Reader) error {
	// Read one byte past the limit so an oversized state is detected without buffering it.
	data, err := io.ReadAll(io.LimitReader(body, maxEmptyStateBytes+1))
	if err != nil {
		return fmt.Errorf("%w: unable to read it: %w", errStateUnverifiableDuringCleanup, err)
	}
	if len(data) > maxEmptyStateBytes {
		return fmt.Errorf("%w: it is larger than %d bytes, so it cannot be the state destroy left", errStateUnverifiableDuringCleanup, maxEmptyStateBytes)
	}

	var state terraformState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("%w: unable to parse it: %w", errStateUnverifiableDuringCleanup, err)
	}
	if state.Version == nil || *state.Version != emptyStateFormatVersion {
		return fmt.Errorf("%w: it does not declare Terraform state format version %d", errStateUnverifiableDuringCleanup, emptyStateFormatVersion)
	}
	if len(state.Resources) > 0 || len(state.Outputs) > 0 {
		return errStateNotEmptyDuringCleanup
	}
	return nil
}

// deleteCloudState removes the state object Terraform leaves behind after destroy, so that deleting a
// Radius resource does not accumulate empty objects in the user's bucket or container.
//
// Cleanup runs after destroy has released the Terraform state lock. It deliberately does not retake
// that lock; it instead reads the state object back, deletes only a recognized state format that
// tracks no resources or outputs, and makes the delete conditional on the entity tag of the version it
// read. A writer that committed live state before the read is caught by the content check, and one
// that commits between the read and the delete is caught by the precondition. In both cases the object
// is left in place, as it is whenever cleanup cannot establish what the object holds.
//
// It is best effort by design. The resources it tracked are already destroyed by the time this runs, so
// a cleanup failure is an operator cleanup task rather than a reason to fail the Radius delete and leave
// the resource undeletable. Every outcome is logged with the state key, and the log distinguishes an
// object Radius chose to keep from one it failed to delete, because only the latter is safe to remove
// without inspecting it first.
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
			logger.Info(fmt.Sprintf("Recovered from panic while deleting Terraform %s state object %q after destroy; inspect the object before removing it manually: %v", settings.Type, key, r))
		}
	}()

	del := e.deleteStateObject
	if del == nil {
		del = e.deleteCloudStateObject
	}

	err := del(ctx, settings, auth, key)
	switch {
	case err == nil:
		logger.Info(fmt.Sprintf("Deleted Terraform %s state object %q after destroy", settings.Type, key))
	case errors.Is(err, errStateAlreadyAbsent):
		logger.Info(fmt.Sprintf("No Terraform %s state object %q to delete after destroy; it was already absent", settings.Type, key))
	case errors.Is(err, errStateModifiedDuringCleanup):
		logger.Info(fmt.Sprintf("Left Terraform %s state object %q in place after destroy because it changed since destroy completed; another writer owns it now", settings.Type, key))
	case errors.Is(err, errStateNotEmptyDuringCleanup):
		logger.Info(fmt.Sprintf("Left Terraform %s state object %q in place after destroy because it still tracks resources or outputs; another writer owns it now", settings.Type, key))
	case errors.Is(err, errStateUnverifiableDuringCleanup):
		// Radius has not shown this object is live, only that it cannot show it is empty, so the
		// operator is told to inspect it rather than to remove it.
		logger.Info(fmt.Sprintf("Left Terraform %s state object %q in place after destroy because Radius could not confirm it is empty; inspect it before removing it manually: %s", settings.Type, key, err.Error()))
	default:
		logger.Info(fmt.Sprintf("Unable to delete Terraform %s state object %q after destroy; the resources it tracked were destroyed and the object can be removed manually: %s", settings.Type, key, err.Error()))
	}
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
	options := s3.Options{
		Region:      settings.Region,
		Credentials: aws.NewCredentialsCache(ucpaws.NewUCPCredentialProvider(provider, 0)),
	}
	if e.stateEndpointOverride != "" {
		options.BaseEndpoint = aws.String(e.stateEndpointOverride)
		options.UsePathStyle = true
	}
	client := s3.New(options)

	// Read the object itself, not just its metadata. Destroy has already released the Terraform lock,
	// so the only way to know this version is the empty state destroy wrote, rather than live state a
	// concurrent operation committed since, is to look at its contents. The entity tag from this same
	// response then scopes the delete to the exact version that was verified.
	object, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(settings.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return errStateAlreadyAbsent
		}
		return err
	}
	defer object.Body.Close()

	if err := verifyStateIsEmpty(object.Body); err != nil {
		return err
	}
	if object.ETag == nil || *object.ETag == "" {
		return fmt.Errorf("%w: it has no entity tag, so the version just verified cannot be identified", errStateUnverifiableDuringCleanup)
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket:  aws.String(settings.Bucket),
		Key:     aws.String(key),
		IfMatch: object.ETag,
	})
	if err != nil {
		// S3 DeleteObject is idempotent, but treat an explicit not-found as success for clarity.
		if isS3NotFound(err) {
			return errStateAlreadyAbsent
		}
		if isS3PreconditionFailed(err) {
			return errStateModifiedDuringCleanup
		}
		return err
	}
	return nil
}

// isS3NotFound reports whether the error means there was no object at the state key.
//
// A coded error is authoritative. Falling through to the HTTP status would classify NoSuchBucket,
// which is a misconfiguration the operator needs to see, as a missing object and report cleanup as
// having nothing to do.
func isS3NotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound"
	}
	return s3ResponseStatus(err) == 404
}

func isS3PreconditionFailed(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return true
	}
	return s3ResponseStatus(err) == 412
}

// s3ResponseStatus returns the HTTP status behind an S3 error, or 0. Some S3 errors arrive without a
// body to model an error code from, so the status is the only reliable signal for a missing object.
func s3ResponseStatus(err error) int {
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.Response != nil {
		return respErr.HTTPStatusCode()
	}
	return 0
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

	clientOptions := azcore.ClientOptions{Cloud: cloudConfig}
	if e.stateTransport != nil {
		clientOptions.Transport = e.stateTransport
	}

	// TokenFilePath must match the path rendered into the backend block. Without it the credential
	// falls back to AZURE_FEDERATED_TOKEN_FILE, which Radius no longer sets for Workload Identity.
	credential, err := azcredential.NewUCPCredential(azcredential.UCPCredentialOptions{
		Provider:      provider,
		TokenFilePath: providers.AzureOIDCTokenFilePath,
		ClientOptions: &clientOptions,
	})
	if err != nil {
		return err
	}

	blobURL := fmt.Sprintf("https://%s.%s/%s/%s", settings.StorageAccountName, storageSuffix, settings.ContainerName, key)
	if e.stateEndpointOverride != "" {
		blobURL = fmt.Sprintf("%s/%s/%s/%s", strings.TrimSuffix(e.stateEndpointOverride, "/"), settings.StorageAccountName, settings.ContainerName, key)
	}
	client, err := blob.NewClient(blobURL, azcore.TokenCredential(credential), &blob.ClientOptions{
		ClientOptions: clientOptions,
	})
	if err != nil {
		return err
	}

	// Download the blob rather than only its properties. Destroy has already released the Terraform
	// lock, so the contents are the only way to tell the empty state destroy wrote from live state a
	// concurrent operation committed since. The entity tag from this same response then scopes the
	// delete to the exact version that was verified.
	download, err := client.DownloadStream(ctx, nil)
	if err != nil {
		// Only a missing blob means there is nothing to clean up. A missing container is a
		// misconfiguration the operator needs to see, so it propagates as a cleanup failure.
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return errStateAlreadyAbsent
		}
		return err
	}
	defer download.Body.Close()

	if err := verifyStateIsEmpty(download.Body); err != nil {
		return err
	}
	if download.ETag == nil || *download.ETag == "" {
		return fmt.Errorf("%w: it has no entity tag, so the version just verified cannot be identified", errStateUnverifiableDuringCleanup)
	}

	options := &blob.DeleteOptions{
		AccessConditions: &blob.AccessConditions{
			ModifiedAccessConditions: &blob.ModifiedAccessConditions{IfMatch: download.ETag},
		},
	}
	if _, err := client.Delete(ctx, options); err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return errStateAlreadyAbsent
		}
		if bloberror.HasCode(err, bloberror.ConditionNotMet) {
			return errStateModifiedDuringCleanup
		}
		return err
	}
	return nil
}

// azureCloudForEnvironment maps an ARM_ENVIRONMENT value to the matching azcore cloud configuration
// and blob storage DNS suffix. The accepted spellings mirror the azurerm backend's own environment
// names so that cleanup and Terraform never disagree about which cloud is in use.
//
// An unrecognized value is an error rather than a fallback to public Azure: deleting from the wrong
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
