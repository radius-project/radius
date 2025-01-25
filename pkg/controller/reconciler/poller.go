/*
Copyright 2024 The Radius Authors.

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

package reconciler

import (
	"context"
	"net/http"

	azcoreruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
)

type Poller[T any] interface {
	Done() bool
	Poll(ctx context.Context) (*http.Response, error)
	Result(ctx context.Context) (T, error)
	ResumeToken() (string, error)
}

var _ Poller[sdkclients.ClientCreateOrUpdateResponse] = (*azcoreruntime.Poller[sdkclients.ClientCreateOrUpdateResponse])(nil)
