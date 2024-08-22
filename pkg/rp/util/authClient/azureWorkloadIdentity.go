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
)

var _ AuthClient = (*azureWorkloadIdentity)(nil)

type azureWorkloadIdentity struct {
	clientID string
	tenantID string
}

func NewAzureWorkloadIdentity(clientID string, tenantID string) AuthClient {
	return &azureWorkloadIdentity{clientID: clientID, tenantID: tenantID}
}

func (b *azureWorkloadIdentity) GetAuthClient(ctx context.Context) (remote.Client, error) {
	// To Do
	return &auth.Client{}, nil
}
