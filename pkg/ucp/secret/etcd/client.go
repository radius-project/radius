// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcd

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/secret"
	etcdclient "go.etcd.io/etcd/client/v3"
)

const (
	secretResourcePrefix = "secret|"
)

var _ secret.Client = (*Client)(nil)

// Client represents radius secret client to manage radius secret.
type Client struct {
	ETCDClient *etcdclient.Client
}

// Save saves the secret in the data store.
func (c *Client) Save(ctx context.Context, name string, value []byte) error {
	if name == "" {
		return &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if value == nil {
		return &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}
	}
	secretName := generateSecretResourceName(name)

	// We don't care about response while save, only if the operation is successful or not
	_, err := c.ETCDClient.Put(ctx, secretName, string(value))
	if err != nil {
		return err
	}
	return nil
}

// Delete deletes the secrets corresponding to the name in etcd.
func (c *Client) Delete(ctx context.Context, name string) error {
	secretName := generateSecretResourceName(name)
	resp, err := c.ETCDClient.Delete(ctx, secretName)
	if err != nil {
		return err
	}
	if resp.Deleted == 0 {
		return &secret.ErrNotFound{}
	}
	return nil
}

// Get returns the secret object if it exists or returns an error.
func (c *Client) Get(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}
	secretName := generateSecretResourceName(name)
	response, err := c.ETCDClient.Get(ctx, secretName)
	if err != nil {
		return nil, err
	}
	if response.Count == 0 {
		return nil, &secret.ErrNotFound{}
	}
	return response.Kvs[0].Value, nil
}

func generateSecretResourceName(name string) string {
	return secretResourcePrefix + name
}
