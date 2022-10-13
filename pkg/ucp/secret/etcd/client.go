// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcd

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ secret.Client = (*Client)(nil)

type Client struct {
	Storage store.StorageClient
}

// CreateSecrets saves the secrets in etcd
func (c *Client) CreateOrUpdate(ctx context.Context, id string, secrets interface{}) error {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: secrets,
	}
	return c.Storage.Save(ctx, nr)
}

// DeleteSecrets deletes the secrets corresponsing to the name in etcd
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.Storage.Delete(ctx, id)
}

// GetSecrets returns the name of the secrets if it exists or returns an error
func (c *Client) Get(ctx context.Context, id string) (string, error) {
	// ignore secret values because we don't want to expose the values
	_, err := c.Storage.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ListSecrets lists the ids of the secrets existing in a plane
func (c *Client) List(ctx context.Context, planeType string, planeName string, scope string) ([]string, error) {
	secrets := []string{}
	var query store.Query
	query.RootScope = resources.SegmentSeparator + resources.PlanesSegment + resources.SegmentSeparator + planeType + resources.SegmentSeparator + planeName
	query.IsScopeQuery = false
	query.ResourceType = "system.azure/credentials"

	result, err := c.Storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, item := range result.Items {
		secrets = append(secrets, item.Metadata.ID)
	}
	return secrets, nil
}
