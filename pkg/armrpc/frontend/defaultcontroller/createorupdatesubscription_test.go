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
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
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
		rawReq := radiustesting.ReadFixture(tc.infile)
		expected := &v1.Subscription{}
		_ = json.Unmarshal(rawReq, expected)

		req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPost, subscriptionHeaderfile, expected)

		// arrange
		op, _ := NewCreateOrUpdateSubscription(ctrl.Options{})
		ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
			APIVersion: v1.SubscriptionAPIVersion,
		})

		// act
		resp, err := op.Run(ctx, req)

		// assert
		require.NoError(t, err)

		w := httptest.NewRecorder()
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
	}

}

func TestSubscriptionsRunWithUnsupportedAPIVersion(t *testing.T) {
	// arrange
	op, _ := NewCreateOrUpdateSubscription(ctrl.Options{})
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
