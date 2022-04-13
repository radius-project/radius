// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/stretchr/testify/require"
)

const testDataFileDir = "./testdata/"

func loadTestData(testfile string) []byte {
	d, err := ioutil.ReadFile(testfile)
	if err != nil {
		return nil
	}
	return d
}

func TestSubscriptionsRunWithArmV2ApiVersion(t *testing.T) {
	files, _ := ioutil.ReadDir(testDataFileDir)
	for _, file := range files {
		testData := loadTestData(testDataFileDir + file.Name())
		req, _ := http.NewRequest("POST", "fakeurl.com", bytes.NewBuffer(testData))
		req.Header.Set("X-Custom-Header", "myvalue")
		req.Header.Set("Content-Type", "application/json")

		// arrange
		op, _ := NewCreateOrUpdateSubscription(nil, nil)
		ctx := servicecontext.WithARMRequestContext(context.Background(), &servicecontext.ARMRequestContext{
			APIVersion: armrpcv1.SubscriptionAPIVersion,
		})

		// act
		resp, _ := op.Run(ctx, req)

		// assert
		switch v := resp.(type) {
		case *rest.OKResponse:
			subscription, ok := v.Body.(*armrpcv1.Subscription)
			require.True(t, ok)

			expected := armrpcv1.Subscription{}
			_ = json.Unmarshal(testData, &expected)
			require.True(t, reflect.DeepEqual(*subscription, expected))
		default:
			require.Truef(t, false, "should not return error")
		}
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
