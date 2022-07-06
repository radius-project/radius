// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/stretchr/testify/require"
)

func TestRunWith20220315PrivatePreview(t *testing.T) {
	// arrange
	opts := ctrl.Options{}
	op, err := NewGetOperations(opts)
	require.NoError(t, err)
	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		APIVersion: v20220315privatepreview.Version,
	})

	// act
	resp, err := op.Run(ctx, nil)

	// assert
	require.NoError(t, err)
	switch v := resp.(type) {
	case *rest.OKResponse:
		pagination, ok := v.Body.(*v1.PaginatedList)
		require.True(t, ok)
		require.Equal(t, 19, len(pagination.Value))
	default:
		require.Truef(t, false, "should not return error")
	}
}

func TestRunWithUnsupportedAPIVersion(t *testing.T) {
	// arrange
	opts := ctrl.Options{}
	op, err := NewGetOperations(opts)
	require.NoError(t, err)
	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		APIVersion: "unknownversion",
	})

	// act
	resp, err := op.Run(ctx, nil)

	// assert
	require.NoError(t, err)
	switch v := resp.(type) {
	case *rest.NotFoundResponse:
		armerr := v.Body
		require.Equal(t, armerrors.InvalidResourceType, armerr.Error.Code)
	default:
		require.Truef(t, false, "should not return error")
	}
}
