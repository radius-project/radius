// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package etcd

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/ucp/secret"
	etcdclient "go.etcd.io/etcd/client/v3"
)

var _ secret.Client = (*Client)(nil)

// Client represents radius secret client to manage radius secret.
type Client struct {
	ETCDClient ETCDV3Client
}

// Save saves the secret in the data store.
func (c *Client) Save(ctx context.Context, name string, value []byte) error {
	if name == "" {
		return &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if value == nil {
		return &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}
	}

	err := c.ETCDClient.Save(ctx, name, string(value))
	if err != nil {
		return err
	}
	return nil
}

// Delete deletes the secrets corresponding to the name in etcd.
func (c *Client) Delete(ctx context.Context, name string) error {
	err := c.ETCDClient.Delete(ctx, name)
	if err != nil {
		return err
	}
	return nil
}

// Get returns the secret object if it exists or returns an error.
func (c *Client) Get(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, &secret.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	response, err := c.ETCDClient.Get(ctx, name)
	if err != nil {
		if errors.Is(err, &secret.ErrNotFound{}) {
			return nil, &secret.ErrNotFound{}
		}
		return nil, err
	}
	return response, nil
}

//go:generate mockgen -destination=./mock_etcdv3client.go -package=etcd -self_package github.com/project-radius/radius/pkg/ucp/secret/etcd github.com/project-radius/radius/pkg/ucp/secret/etcd ETCDV3Client

// ETCDV3Client is an interface to implement etcd v3 vanilla client operations.
type ETCDV3Client interface {
	Save(ctx context.Context, name string, value string) error
	Delete(ctx context.Context, name string) error
	Get(ctx context.Context, name string) ([]byte, error)
}

var _ ETCDV3Client = (*ETCDV3ClientImpl)(nil)

// ETCDV3ClientImpl is a struct to implement vanilla etcd v3 client.
type ETCDV3ClientImpl struct {
	ETCDClient *etcdclient.Client
}

// Save saves the object with key as 'id' in etcd store.
func (ec *ETCDV3ClientImpl) Save(ctx context.Context, name string, value string) error {
	// We only care about the success or failure of the operation.
	_, err := ec.ETCDClient.Put(ctx, name, value)
	if err != nil {
		return err
	}
	return nil
}

// Delete deletes the object with key as 'id' from etcd store.
func (ec *ETCDV3ClientImpl) Delete(ctx context.Context, name string) error {
	// We only care about the success or failure of the operation.
	_, err := ec.ETCDClient.Delete(ctx, name)
	if err != nil {
		return err
	}
	return nil
}

// Get gets value for object with key as 'id' from etcd store.
func (ec *ETCDV3ClientImpl) Get(ctx context.Context, name string) ([]byte, error) {
	result, err := ec.ETCDClient.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return result.Kvs[0].Value, nil
}
