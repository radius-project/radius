// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/stretchr/testify/require"
)

func loadTestData(testfile string) []byte {
	d, err := ioutil.ReadFile(testfile)
	if err != nil {
		return nil
	}
	return d
}

func TestSubscriptionsRunWith20220315PrivatePreview(t *testing.T) {
	testDataFile := "./testdata/subscriptiontestdata.json"
	testData := loadTestData(testDataFile)
	req, _ := http.NewRequest("POST", "fakeurl.com", bytes.NewBuffer(testData))
    req.Header.Set("X-Custom-Header", "myvalue")
    req.Header.Set("Content-Type", "application/json")

	// arrange
	op, _ := NewCreateOrUpdateSubscription(nil, nil)
	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		APIVersion: v20220315privatepreview.Version,
	})

	// act
	resp, _ := op.Run(ctx, req)

	// assert
	switch v := resp.(type) {
	case *rest.OKResponse:
		pagination, ok := v.Body.(*armrpcv1.PaginatedList)
		require.True(t, ok)
		require.Equal(t, 1, len(pagination.Value))
	default:
		require.Truef(t, false, "should not return error")
	}
}

func TestSubscriptionsRunWithUnsupportedAPIVersion(t *testing.T) {
	// arrange
	op, _ := NewCreateOrUpdateSubscription(nil, nil)
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
