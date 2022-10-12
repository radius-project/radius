// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcdsecrets

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secrets"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ secrets.Interface = (*Client)(nil)

type Client struct {
	SecretsStorageClient store.StorageClient
}

// CreateSecrets saves the secrets in etcd
func (c *Client) CreateSecrets(ctx context.Context, id string, secrets interface{}) error {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: secrets,
	}
	return c.SecretsStorageClient.Save(ctx, nr)
}

// DeleteSecrets deletes the secrets corresponsing to the name in etcd
func (c *Client) DeleteSecrets(ctx context.Context, id string) error {
	return c.SecretsStorageClient.Delete(ctx, id)
}

// GetSecrets returns the name of the secrets if it exists or returns an error
func (c *Client) GetSecrets(ctx context.Context, id string) (string, error) {
	// ignore secret values because we don't want to expose the values
	_, err := c.SecretsStorageClient.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ListSecrets lists the ids of the secrets existing in a plane
func (c *Client) ListSecrets(ctx context.Context, planeType string, planeName string, scope string) ([]string, error) {
	secrets := []string{}
	var query store.Query
	query.RootScope = resources.SegmentSeparator + resources.PlanesSegment + resources.SegmentSeparator + planeType + resources.SegmentSeparator + planeName
	query.IsScopeQuery = false
	query.ResourceType = "system.azure/credentials"

	result, err := c.SecretsStorageClient.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, item := range result.Items {
		secrets = append(secrets, item.Metadata.ID)
	}
	return secrets, nil
}
