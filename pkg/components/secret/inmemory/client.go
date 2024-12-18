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

package inmemory

import (
	"context"
	"sync"

	"github.com/radius-project/radius/pkg/components/secret"
	"github.com/radius-project/radius/pkg/kubernetes"
)

var _ secret.Client = (*Client)(nil)

// Client implements an in-memory secret client.
//
// The in-memory client is suitable for testing and development.
type Client struct {
	lock sync.Mutex
	data map[string][]byte
}

// Save saves the secret data.
func (c *Client) Save(ctx context.Context, name string, value []byte) error {
	if name == "" {
		return &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if value == nil {
		return &secret.ErrInvalid{Message: "invalid argument. 'value' is required"}
	}

	if !kubernetes.IsValidObjectName(name) {
		return &secret.ErrInvalid{Message: "invalid name: " + name}
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.data == nil {
		c.data = map[string][]byte{}
	}

	c.data[name] = value

	return nil
}

// Delete deletes the secret data if it is present in the store, otherwise returns an ErrNotFound.
func (c *Client) Delete(ctx context.Context, name string) error {
	if name == "" {
		return &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if !kubernetes.IsValidObjectName(name) {
		return &secret.ErrInvalid{Message: "invalid name: " + name}
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.data == nil {
		c.data = map[string][]byte{}
	}

	_, ok := c.data[name]
	if !ok {
		return &secret.ErrNotFound{}
	}

	delete(c.data, name)

	return nil
}

// Get returns the secret data if it is found, otherwise returns an ErrNotFound.
func (c *Client) Get(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if !kubernetes.IsValidObjectName(name) {
		return nil, &secret.ErrInvalid{Message: "invalid name: " + name}
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	data, ok := c.data[name]
	if !ok {
		return nil, &secret.ErrNotFound{}
	}

	return data, nil
}
