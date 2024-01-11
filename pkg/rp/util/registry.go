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
	"fmt"

	dockerParser "github.com/novln/docker-parser"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/recipes"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
)

// ReadFromRegistry reads data from an OCI compliant registry and stores it in a map. It returns an error if the path is invalid,
// if the client to the registry fails to be created, if the manifest fails to be fetched, if the bytes fail to be fetched, or if
// the data fails to be unmarshalled.
func ReadFromRegistry(ctx context.Context, definition recipes.EnvironmentDefinition, data *map[string]any, client remote.Client) error {
	registryRepo, tag, err := parsePath(definition.TemplatePath)
	if err != nil {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid path %s", err.Error()))
	}

	repo, err := remote.NewRepository(registryRepo)
	if err != nil {
		return fmt.Errorf("failed to create client to registry %s", err.Error())
	}
	repo.Client = client

	if definition.PlainHTTP {
		repo.PlainHTTP = true
	}

	digest, err := getDigestFromManifest(ctx, repo, tag)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeLanguageFailure, fmt.Sprintf("failed to fetch repository from the path %q: %s", definition.TemplatePath, err.Error()), recipes_util.RecipeSetupError, nil)
	}

	bytes, err := getBytes(ctx, repo, digest)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeLanguageFailure, fmt.Sprintf("failed to fetch repository from the path %q: %s", definition.TemplatePath, err.Error()), recipes_util.RecipeSetupError, nil)
	}

	err = json.Unmarshal(bytes, data)
	if err != nil {
		return err
	}

	return nil
}

// getDigestFromManifest gets the layers digest from the manifest
func getDigestFromManifest(ctx context.Context, repo *remote.Repository, tag string) (string, error) {
	// resolves a manifest descriptor with a Tag reference
	descriptor, err := repo.Resolve(ctx, tag)
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

// parsePath parses a path in the form of registry/repository:tag
func parsePath(path string) (repository string, tag string, err error) {
	reference, err := dockerParser.Parse(path)
	if err != nil {
		return "", "", err
	}

	repository = reference.Repository()
	tag = reference.Tag()
	return
}
