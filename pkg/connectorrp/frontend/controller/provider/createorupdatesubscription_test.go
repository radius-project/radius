// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
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
		reqBody := &armrpcv1.Subscription{}
		_ = json.Unmarshal(rawReq, reqBody)

		req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodPost, subscriptionHeaderfile, reqBody)

		ctrl, _ := NewCreateOrUpdateSubscription(nil, nil)
		ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
			APIVersion: armrpcv1.SubscriptionAPIVersion,
		})

		resp, err := ctrl.Run(ctx, req)

		require.NoError(t, err)

		w := httptest.NewRecorder()
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
	}
}

func TestSubscriptionsRunWithUnsupportedAPIVersion(t *testing.T) {
	ctrl, _ := NewCreateOrUpdateSubscription(nil, nil)
	ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
		APIVersion: "unknown",
	})

	resp, _ := ctrl.Run(ctx, nil)

	switch respType := resp.(type) {
	case *rest.NotFoundResponse:
		armerr := respType.Body
		require.Equal(t, armerrors.InvalidResourceType, armerr.Error.Code)
	default:
		require.Truef(t, false, "should not return error")
	}
}
