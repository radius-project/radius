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

// Package secret materializes recipe secret outputs into Radius-managed Radius.Security/secrets
// resources so that secret values are never persisted on the owning resource. Consumers bind to the
// managed secret by name via a container's env valueFrom.secretKeyRef.
package secret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// SecuritySecretsResourceType is the fully-qualified resource type of the managed secret.
	SecuritySecretsResourceType = "Radius.Security/secrets"

	// securitySecretsAPIVersion is the only API version Radius.Security/secrets supports. The shared
	// generic resources client is generated against 2023-10-01-preview, so the API version is overridden
	// per request via ClientOptions.APIVersion.
	securitySecretsAPIVersion = "2025-08-01-preview"

	// managedSecretNameSuffix is appended to an owner resource name to derive its managed secret name.
	managedSecretNameSuffix = "-secrets"

	// managedSecretNameHashLength is the number of hex characters of the owner-ID hash included in the
	// managed secret name to disambiguate owners that share a name but differ in resource type or scope.
	managedSecretNameHashLength = 8
)

// Request describes the managed secret to create or update for an owner resource.
type Request struct {
	// OwnerResourceID is the fully-qualified resource ID of the resource that declared the secrets block.
	OwnerResourceID string
	// EnvironmentID is the owner's environment ID, copied onto the managed secret.
	EnvironmentID string
	// ApplicationID is the owner's application ID, copied onto the managed secret when non-empty.
	ApplicationID string
	// Data maps declared secret keys to their plaintext values. The values are only ever held in memory
	// and passed through to the managed secret's sensitive input; they are never persisted on the owner.
	Data map[string]string
}

// Result identifies the managed secret backing an owner resource.
type Result struct {
	// ID is the fully-qualified resource ID of the managed Radius.Security/secrets resource.
	ID string
	// Name is the name of the managed Radius.Security/secrets resource. Consumers reference this via a
	// container env valueFrom.secretKeyRef.secretName.
	Name string
}

// Materializer creates, updates and deletes the Radius.Security/secrets resource that backs a resource's
// declared recipe secret outputs.
type Materializer interface {
	// Materialize creates or updates the managed secret for the owner described by req and returns its
	// identity. It is idempotent: the managed secret name is derived deterministically from the owner.
	Materialize(ctx context.Context, req Request) (Result, error)

	// Delete removes the managed secret backing the given owner resource. Deleting a non-existent managed
	// secret is not an error.
	Delete(ctx context.Context, ownerResourceID string) error
}

// clientMaterializer implements Materializer using the generic (dynamic) resources client against UCP.
type clientMaterializer struct {
	armClientOptions *arm.ClientOptions
}

// NewMaterializer creates a Materializer that talks to UCP using the provided ARM client options.
func NewMaterializer(armClientOptions *arm.ClientOptions) Materializer {
	return &clientMaterializer{armClientOptions: armClientOptions}
}

// ManagedSecretName derives the deterministic name of the managed Radius.Security/secrets resource for an
// owner resource. It combines the owner's name with a short hash of the owner's fully-qualified resource ID
// so that owners that share a name but differ in resource type or scope — which is allowed within a single
// resource group — never collide on the same managed secret. The name is stable for a given owner, so
// Materialize and Delete derive the same value.
func ManagedSecretName(ownerID resources.ID) string {
	sum := sha256.Sum256([]byte(strings.ToLower(ownerID.String())))
	hash := hex.EncodeToString(sum[:])[:managedSecretNameHashLength]
	return fmt.Sprintf("%s-%s%s", ownerID.Name(), hash, managedSecretNameSuffix)
}

// Materialize creates or updates the managed Radius.Security/secrets resource for the owner and returns its
// identity. It does not wait for the managed secret's own provisioning to complete: the PUT is accepted and
// persisted synchronously, and Kubernetes consumers bind lazily once the backing Kubernetes Secret exists.
func (m *clientMaterializer) Materialize(ctx context.Context, req Request) (Result, error) {
	ownerID, err := resources.ParseResource(req.OwnerResourceID)
	if err != nil {
		return Result{}, err
	}

	secretName := ManagedSecretName(ownerID)
	secretID := fmt.Sprintf("%s/providers/%s/%s", ownerID.RootScope(), SecuritySecretsResourceType, secretName)

	data := make(map[string]any, len(req.Data))
	for key, value := range req.Data {
		data[key] = map[string]any{"value": value}
	}

	properties := map[string]any{
		"environment": req.EnvironmentID,
		"data":        data,
	}
	if req.ApplicationID != "" {
		properties["application"] = req.ApplicationID
	}

	client, err := generated.NewGenericResourcesClient(SecuritySecretsResourceType, ownerID.RootScope(), &aztoken.AnonymousCredential{}, m.clientOptions())
	if err != nil {
		return Result{}, err
	}

	// BeginCreateOrUpdate performs the accepted PUT synchronously (dynamic-rp persists the resource and
	// queues its own async provisioning). We intentionally do not poll to completion to avoid holding a
	// worker while the managed secret's own async operation waits for one.
	if _, err := client.BeginCreateOrUpdate(ctx, secretName, generated.GenericResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: properties,
	}, nil); err != nil {
		return Result{}, fmt.Errorf("failed to materialize managed secret '%s': %w", secretID, err)
	}

	return Result{ID: secretID, Name: secretName}, nil
}

// Delete removes the managed secret backing the given owner resource. A not-found managed secret is treated
// as already deleted.
func (m *clientMaterializer) Delete(ctx context.Context, ownerResourceID string) error {
	ownerID, err := resources.ParseResource(ownerResourceID)
	if err != nil {
		return err
	}

	secretName := ManagedSecretName(ownerID)
	client, err := generated.NewGenericResourcesClient(SecuritySecretsResourceType, ownerID.RootScope(), &aztoken.AnonymousCredential{}, m.clientOptions())
	if err != nil {
		return err
	}

	if _, err := client.BeginDelete(ctx, secretName, nil); err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete managed secret '%s': %w", secretName, err)
	}

	return nil
}

// clientOptions returns a shallow copy of the configured ARM client options with the API version overridden
// to the version Radius.Security/secrets supports, so the shared options are not mutated.
func (m *clientMaterializer) clientOptions() *arm.ClientOptions {
	options := &arm.ClientOptions{}
	if m.armClientOptions != nil {
		options = &arm.ClientOptions{ClientOptions: m.armClientOptions.ClientOptions}
	}
	options.APIVersion = securitySecretsAPIVersion
	return options
}
