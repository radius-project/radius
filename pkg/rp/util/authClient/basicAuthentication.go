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

package authclient

import (
	"context"

	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

var _ AuthClient = (*basicAuthentication)(nil)

type basicAuthentication struct {
	username string
	password string
}

// NewBasicAuthentication creates a new basicAuthentication instance.
func NewBasicAuthentication(username string, password string) AuthClient {
	return &basicAuthentication{username: username, password: password}
}

// GetAuthClient creates and returns an authentication client for accessing a private bicep registry
// using basic authentication. It returns an auth.Client configured with the
// provided username and password for the registry.
func (b *basicAuthentication) GetAuthClient(ctx context.Context, templatePath string) (remote.Client, error) {
	registry, err := getRegistryHostname(templatePath)
	if err != nil {
		return nil, err
	}

	return &auth.Client{
		Client: retry.DefaultClient,
		Credential: auth.StaticCredential(registry, auth.Credential{
			Username: b.username,
			Password: b.password,
		}),
	}, nil
}
