// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"

	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

const (
	subscriptionHeaderfile = "armsubscriptionheaders.json"
)

func TestSubscriptionsRunWithArmV2ApiVersion(t *testing.T) {
	subscriptionTests := []struct {
		infile string
	}{
		{"registeredsubscriptiontestdata.json"},
		{"unregisteredsubscriptiontestdata.json"},
	}

	for _, tc := range subscriptionTests {
		rawReq := testutil.ReadFixture(tc.infile)
		expected := &v1.Subscription{}
		_ = json.Unmarshal(rawReq, expected)

		req, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPost, subscriptionHeaderfile, expected)

		// arrange
		op, _ := NewCreateOrUpdateSubscription(ctrl.Options{})
		ctx := v1.WithARMRequestContext(context.Background(), &v1.ARMRequestContext{
			APIVersion: v1.SubscriptionAPIVersion,
		})
		w := httptest.NewRecorder()

		// act
		resp, err := op.Run(ctx, w, req)

		// assert
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
	}

}

func TestSubscriptionsRunWithUnsupportedAPIVersion(t *testing.T) {
	// arrange
	op, _ := NewCreateOrUpdateSubscription(ctrl.Options{})
	ctx := v1.WithARMRequestContext(context.Background(), &v1.ARMRequestContext{
		APIVersion: "unknownversion",
	})
	w := httptest.NewRecorder()

	// act
	resp, _ := op.Run(ctx, w, nil)

	// assert
	switch v := resp.(type) {
	case *rest.NotFoundResponse:
		armerr := v.Body
		require.Equal(t, v1.CodeInvalidResourceType, armerr.Error.Code)
	default:
		require.Truef(t, false, "should not return error")
	}
}
