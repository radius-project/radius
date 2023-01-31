// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"context"
	"encoding/json"
	"fmt"

	dockerParser "github.com/novln/docker-parser"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
)

// ReadFromRegistry reads content from an OCI compliant registry.
func ReadFromRegistry(ctx context.Context, path string, data *map[string]any) error {
	registryRepo, tag, err := ParsePath(path)
	if err != nil {
		return fmt.Errorf("invalid path %s", err.Error())
	}

	// get the data from ACR
	// client to the ACR repository in the path
	repo, err := remote.NewRepository(registryRepo)
	if err != nil {
		return fmt.Errorf("failed to create client to registry %s", err.Error())
	}

	digest, err := getDigestFromManifest(ctx, repo, tag)
	if err != nil {
		return err
	}

	bytes, err := getBytes(ctx, repo, digest)
	if err != nil {
		return err
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

// ParsePath parses a path in the form of registry/repository:tag
func ParsePath(path string) (repository string, tag string, err error) {
	reference, err := dockerParser.Parse(path)
	if err != nil {
		return "", "", err
	}

	repository = reference.Repository()
	tag = reference.Tag()
	return
}
