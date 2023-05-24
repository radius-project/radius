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

package etcd

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/util"
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
	resp, err := c.ETCDClient.Get(ctx, secretName)
	if err != nil {
		return nil, err
	}
	if resp.Count == 0 {
		return nil, &secret.ErrNotFound{}
	}
	return resp.Kvs[0].Value, nil
}

func generateSecretResourceName(name string) string {
	return secretResourcePrefix + util.NormalizeStringToLower(name)
}
