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

package controller

import (
	"errors"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestNewFailedResult(t *testing.T) {
	ctx := testcontext.New(t)
	err := errors.New("test error")
	errorDetails := v1.ErrorDetails{
		Message: "test error message",
	}

	result := NewFailedResult(ctx, err, errorDetails)
	require.Equal(t, errorDetails, *result.Error)
	require.Equal(t, false, result.Requeue)
	require.Equal(t, v1.ProvisioningStateFailed, *result.state)
}

func TestSetFailed(t *testing.T) {
	err := v1.ErrorDetails{
		Message: "test error message",
	}

	result := &Result{}
	result.SetFailed(testcontext.New(t), errors.New("test error"), err, true)
	require.Equal(t, err, *result.Error)
	require.Equal(t, true, result.Requeue)
}
