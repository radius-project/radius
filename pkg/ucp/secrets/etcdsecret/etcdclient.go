// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcdsecret

import "github.com/project-radius/radius/pkg/ucp/secrets"

var _ secrets.Interface = (*Client)(nil)

type Client struct {
}

func(c *Client) CreateSecrets(name string) {

}

func(c *Client) DeleteSecrets(name string) {

}

func(c *Client) GetSecrets(name string) {

}

func(c *Client) ListSecrets() {

}
