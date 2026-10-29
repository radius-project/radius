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

package util

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"github.com/distribution/reference"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/recipes"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/retry"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// ReadFromRegistry reads data from an OCI compliant registry and stores it in a map. It returns an error if the path is invalid,
// if the manifest fails to be fetched, if the bytes fail to be fetched, or if the data fails to be unmarshalled.
func ReadFromRegistry(ctx context.Context, definition recipes.EnvironmentDefinition, data *map[string]any, client remote.Client) error {
	ref, err := parseTemplatePath(definition.TemplatePath)
	if err != nil {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid path %s", err.Error()))
	}

	repo := &remote.Repository{Reference: ref, Client: client, PlainHTTP: definition.PlainHTTP}

	bytes, err := fetchRecipeWithRetry(ctx, repo, ref.ReferenceOrDefault())
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeLanguageFailure, fmt.Sprintf("failed to fetch repository from the path %q: %s", definition.TemplatePath, err.Error()), recipes_util.RecipeSetupError, nil)
	}

	err = json.Unmarshal(bytes, data)
	if err != nil {
		return err
	}

	return nil
}

// registryFetchBackoff returns the backoff used to retry transient registry
// failures. A new backoff is created per fetch because backoffs are stateful.
// It is a variable so tests can shorten the delays.
var registryFetchBackoff = retry.DefaultBackoffStrategy

// fetchRecipeWithRetry downloads the recipe layer, retrying the whole manifest and
// blob exchange when it fails with a transient network or registry error.
//
// The ORAS client already retries individual round trips, but it does not cover
// failures while reading a response body (for example a connection reset partway
// through a blob download from a CDN) and its retry window is only a few seconds,
// which is shorter than typical DNS or network blips.
func fetchRecipeWithRetry(ctx context.Context, repo *remote.Repository, ref string) ([]byte, error) {
	var result []byte
	retryer := retry.NewRetryer(&retry.RetryConfig{BackoffStrategy: registryFetchBackoff()})
	err := retryer.RetryFunc(ctx, func(ctx context.Context) error {
		digest, err := getDigestFromManifest(ctx, repo, ref)
		if err == nil {
			result, err = getBytes(ctx, repo, digest)
		}

		if err != nil && isTransientRegistryError(err) {
			return retry.RetryableError(err)
		}

		return err
	})

	return result, err
}

// isTransientRegistryError reports whether err is a network or server-side
// failure that is likely to succeed on retry. Client errors such as a missing
// tag or an authentication failure are not retried.
//
// context.DeadlineExceeded is intentionally not rejected here: an http.Client
// timeout wraps it while also being a net.Error timeout that is worth retrying.
// When the caller's own deadline has expired, RetryFunc stops on the context
// before making another attempt.
func isTransientRegistryError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}

	if errResp, ok := errors.AsType[*errcode.ErrorResponse](err); ok {
		return errResp.StatusCode >= http.StatusInternalServerError ||
			errResp.StatusCode == http.StatusTooManyRequests ||
			errResp.StatusCode == http.StatusRequestTimeout
	}

	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
		return dnsErr.IsTimeout || dnsErr.IsTemporary
	}

	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}

	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// getDigestFromManifest gets the layers digest from the manifest that ref, a tag or digest, resolves to.
func getDigestFromManifest(ctx context.Context, repo *remote.Repository, ref string) (string, error) {
	descriptor, err := repo.Resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	// get the manifest data
	rc, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	manifestBlob, err := content.ReadAll(rc, descriptor)
	if err != nil {
		return "", err
	}
	// create the manifest map to get the digest of the layer
	var manifest map[string]any
	err = json.Unmarshal(manifestBlob, &manifest)
	if err != nil {
		return "", err
	}

	// get the layers digest to fetch the blob
	layer, ok := manifest["layers"].([]any)[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("failed to decode the layers from manifest")
	}
	layerDigest, ok := layer["digest"].(string)
	if !ok {
		return "", fmt.Errorf("failed to decode the layers digest from manifest")
	}
	return layerDigest, nil
}

// getBytes fetches the recipe ARM JSON using the layers digest
func getBytes(ctx context.Context, repo *remote.Repository, layerDigest string) ([]byte, error) {
	// resolves a layer blob descriptor with a digest reference
	descriptor, err := repo.Blobs().Resolve(ctx, layerDigest)
	if err != nil {
		return nil, err
	}
	// get the layer data
	rc, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	pulledBlob, err := content.ReadAll(rc, descriptor)
	if err != nil {
		return nil, err
	}
	return pulledBlob, nil
}

// parseTemplatePath parses a recipe template path into a fully qualified reference.
// Recipe template paths may include an http(s):// scheme even though OCI references do
// not, so a leading lowercase "https://" or "http://" is ignored. PlainHTTP, not the
// scheme, selects the transport. distribution/reference applies Docker normalization,
// so Docker Hub short names and index.docker.io become docker.io. ORAS then splits the
// normalized name. For repository:tag@digest the digest wins, and a path with neither
// resolves the "latest" tag through ReferenceOrDefault. It returns the parser error if
// the path without its scheme is not a valid reference.
func parseTemplatePath(templatePath string) (registry.Reference, error) {
	path := strings.TrimPrefix(templatePath, "https://")
	path = strings.TrimPrefix(path, "http://")

	named, err := reference.ParseNormalizedNamed(path)
	if err != nil {
		return registry.Reference{}, err
	}

	return registry.ParseReference(named.String())
}

// RegistryName returns the registry name of a recipe template path, which is the key used
// to look up registry credentials. It is the name, not the network host: for Docker Hub it
// returns "docker.io", and ORAS sends the requests to registry-1.docker.io and maps
// docker.io credentials to that host. The only error is a path that does not parse.
func RegistryName(templatePath string) (string, error) {
	ref, err := parseTemplatePath(templatePath)
	if err != nil {
		return "", err
	}

	return ref.Registry, nil
}

// GetRegistrySecrets returns the secret data for the registry of a recipe template path.
// It looks up the registry name in the Bicep registry authentication of the recipe
// configuration, then returns the secret data for the secret ID that the entry points to.
// It returns empty secret data if no entry matches or the template path does not parse.
// ReadFromRegistry rejects a path that does not parse with BadRequest before it sends a
// request, so empty secret data for that path does not hide the error.
func GetRegistrySecrets(definition recipes.Configuration, templatePath string, secrets map[string]recipes.SecretData) recipes.SecretData {
	registryName, err := RegistryName(templatePath)
	if err != nil {
		return recipes.SecretData{}
	}

	authConfig := definition.RecipeConfig.Bicep.Authentication[registryName]
	secretData := secrets[authConfig.Secret]

	// When the environment's bicepSettings specify an explicit authentication method, it is the
	// source of truth for selecting the registry auth client (see authclient.GetNewRegistryAuthClient).
	// This lets a Radius.Security/secrets resource carry only the credential data without a kind that
	// matches the auth scheme. The legacy Applications.Core/secretStores path leaves the method empty,
	// so the secret store's own type is used unchanged.
	if t := secretTypeForAuthMethod(authConfig.AuthenticationMethod); t != "" {
		secretData.Type = t
	}

	return secretData
}

// secretTypeForAuthMethod maps a bicepSettings authenticationMethod to the SecretData.Type expected
// by authclient.GetNewRegistryAuthClient. It returns "" when the method is unset or unrecognized, in
// which case the secret's own type is used.
func secretTypeForAuthMethod(method string) string {
	switch method {
	case "BasicAuth":
		return "basicAuthentication"
	case "AzureWI":
		return "azureWorkloadIdentity"
	case "AwsIrsa":
		return "awsIRSA"
	default:
		return ""
	}
}
