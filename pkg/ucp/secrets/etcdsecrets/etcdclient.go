// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcdsecrets

import (
	"github.com/project-radius/radius/pkg/ucp/secrets"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ secrets.Interface = (*Client)(nil)

type Client struct {
	SecretsStorageClient store.StorageClient
}

func (c *Client) CreateSecrets(name string) {

}

func (c *Client) DeleteSecrets(name string) {

}

func (c *Client) GetSecrets(name string) {

}

func (c *Client) ListSecrets() {

}
