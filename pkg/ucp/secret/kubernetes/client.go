/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package kubernetes

import (
	"context"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/secret"
	corev1 "k8s.io/api/core/v1"
	k8s_error "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SecretKey       = "ucp_secret"
	RadiusNamespace = "radius-system"
)

var _ secret.Client = (*Client)(nil)

// Client implements secret storage for k8s.
type Client struct {
	K8sClient controller_runtime.Client
}

// Save saves the secret as a k8s secret resource.
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

	secretObjectKey := controller_runtime.ObjectKey{
		Name:      name,
		Namespace: RadiusNamespace,
	}

	// build secret object
	data := make(map[string][]byte)
	data[SecretKey] = value
	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: RadiusNamespace,
		},
		Data: data,
	}
	// check if secret already exists or not
	res := &corev1.Secret{}
	err := c.K8sClient.Get(ctx, secretObjectKey, res)
	if err != nil {
		if k8s_error.IsNotFound(err) {
			return c.K8sClient.Create(ctx, secret)
		}
		return err
	}
	return c.K8sClient.Update(ctx, secret)
}

// Delete deletes the secret resource with id.
func (c *Client) Delete(ctx context.Context, name string) error {
	if name == "" {
		return &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if !kubernetes.IsValidObjectName(name) {
		return &secret.ErrInvalid{Message: "invalid name: " + name}
	}

	secretObject := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: RadiusNamespace,
		},
	}
	err := c.K8sClient.Delete(ctx, secretObject)
	if err != nil {
		if k8s_error.IsNotFound(err) {
			return &secret.ErrNotFound{}
		}
		return err
	}
	return nil
}

// Get returns the id if secret exists otherwise error.
func (c *Client) Get(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, &secret.ErrInvalid{Message: "invalid argument. 'name' is required"}
	}

	if !kubernetes.IsValidObjectName(name) {
		return nil, &secret.ErrInvalid{Message: "invalid name: " + name}
	}

	res := &corev1.Secret{}
	secretObjectKey := controller_runtime.ObjectKey{
		Name:      name,
		Namespace: RadiusNamespace,
	}
	err := c.K8sClient.Get(ctx, secretObjectKey, res)
	if err != nil {
		if k8s_error.IsNotFound(err) {
			return nil, &secret.ErrNotFound{}
		}
		return nil, err
	}
	return res.Data[SecretKey], nil
}
