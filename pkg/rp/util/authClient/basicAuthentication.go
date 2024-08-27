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

package authClient

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

func NewBasicAuthentication(username string, password string) AuthClient {
	return &basicAuthentication{username: username, password: password}
}

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
