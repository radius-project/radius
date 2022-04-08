// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	v20220315 "github.com/project-radius/radius/pkg/corerp/api/v20220315"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/stretchr/testify/require"
)

func TestRunWith20220315PrivatePreview(t *testing.T) {
	// arrange
	op, _ := NewGetOperations(nil, nil)
	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		APIVersion: v20220315.Version,
	})

	// act
	resp, _ := op.Run(ctx, nil)

	// assert
	switch v := resp.(type) {
	case *rest.OKResponse:
		pagination, ok := v.Body.(*armrpcv1.PaginatedList)
		require.True(t, ok)
		require.Equal(t, 7, len(pagination.Value))
	default:
		require.Truef(t, false, "should not return error")
	}
}

func TestRunWithUnsupportedAPIVersion(t *testing.T) {
	// arrange
	op, _ := NewGetOperations(nil, nil)
	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		APIVersion: "unknownversion",
	})

	// act
	resp, _ := op.Run(ctx, nil)

	// assert
	switch v := resp.(type) {
	case *rest.NotFoundResponse:
		armerr := v.Body
		require.Equal(t, armerrors.InvalidResourceType, armerr.Error.Code)
	default:
		require.Truef(t, false, "should not return error")
	}
}
