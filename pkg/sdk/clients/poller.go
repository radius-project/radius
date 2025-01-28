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

package clients

import (
	"context"
	"net/http"

	azcoreruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

// Poller is an interface for polling an operation.
// This uses the same functions of the Poller struct from the Azure SDK for Go.
// We use this interface to allow mocking the Poller struct in tests.
type Poller[T any] interface {
	Done() bool
	Poll(ctx context.Context) (*http.Response, error)
	PollUntilDone(ctx context.Context, options *PollUntilDoneOptions) (T, error)
	Result(ctx context.Context) (T, error)
	ResumeToken() (string, error)
}

type PollUntilDoneOptions = azcoreruntime.PollUntilDoneOptions

// OperationState represents the state of an operation.
// This is a simplified version of the real OperationState that we use in testing.
type OperationState struct {
	Complete bool
	Value    any
	// Ideally we'd use azcore.ResponseError here, but it's tricky to set up in tests.
	Err        error
	ResourceID string
	Kind       string
}
