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

package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	v20220315privatepreview "github.com/radius-project/radius/pkg/portableresources/api/v20220315privatepreview"
	"github.com/stretchr/testify/require"
)

func TestRunWith20220315PrivatePreview(t *testing.T) {
	// arrange
	opts := ctrl.Options{}
	op, err := NewGetOperations(opts)
	require.NoError(t, err)
	ctx := v1.WithARMRequestContext(context.Background(), &v1.ARMRequestContext{
		APIVersion: v20220315privatepreview.Version,
	})
	w := httptest.NewRecorder()

	// act
	resp, err := op.Run(ctx, w, nil)

	// assert
	require.NoError(t, err)
	switch v := resp.(type) {
	case *rest.OKResponse:
		pagination, ok := v.Body.(*v1.PaginatedList)
		require.True(t, ok)
		require.Equal(t, 33, len(pagination.Value))
	default:
		require.Truef(t, false, "should not return error")
	}
}

func TestRunWithUnsupportedAPIVersion(t *testing.T) {
	// arrange
	opts := ctrl.Options{}
	op, err := NewGetOperations(opts)
	require.NoError(t, err)
	ctx := v1.WithARMRequestContext(context.Background(), &v1.ARMRequestContext{
		APIVersion: "unknownversion",
	})
	w := httptest.NewRecorder()

	// act
	resp, err := op.Run(ctx, w, nil)

	// assert
	require.NoError(t, err)
	switch v := resp.(type) {
	case *rest.NotFoundResponse:
		armerr := v.Body
		require.Equal(t, v1.CodeInvalidResourceType, armerr.Error.Code)
	default:
		require.Truef(t, false, "should not return error")
	}
}
