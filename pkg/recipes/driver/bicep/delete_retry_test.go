/*
Copyright 2026 The Radius Authors.

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

package bicep

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_Bicep_Delete_RetryPolicy(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		retries     int
		failures    int
		wantCalls   int
		wantElapsed time.Duration
		wantFailure bool
	}{
		{name: "authorization failure", err: &azcore.ResponseError{StatusCode: http.StatusForbidden, ErrorCode: "AuthorizationFailed"}, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "wrapped authorization failure", err: &processors.ResourceError{ID: "test", Inner: fmt.Errorf("delete failed: %w", &azcore.ResponseError{StatusCode: http.StatusForbidden, ErrorCode: "AuthorizationFailed"})}, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "authentication failure", err: &azcore.ResponseError{StatusCode: http.StatusUnauthorized}, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "authorization failure from failed long-running operation", err: &azcore.ResponseError{StatusCode: http.StatusOK, ErrorCode: "AuthorizationFailed"}, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "linked authorization failure", err: &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "LinkedAuthorizationFailed"}, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "invalid authentication token", err: &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "InvalidAuthenticationToken"}, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "kubernetes unauthorized", err: apierrors.NewUnauthorized("denied"), retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "kubernetes forbidden", err: fmt.Errorf("delete failed: %w", apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "test", errors.New("denied"))), retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "canceled request", err: fmt.Errorf("delete failed: %w", context.Canceled), retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "request deadline", err: context.DeadlineExceeded, retries: 20, failures: 21, wantCalls: 1, wantFailure: true},
		{name: "dependency conflict then success", err: &azcore.ResponseError{StatusCode: http.StatusConflict}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "wrapped dependency conflict then success", err: &processors.ResourceError{ID: "test", Inner: &azcore.ResponseError{StatusCode: http.StatusConflict}}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "kubernetes conflict then success", err: apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, "test", errors.New("conflict")), retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "timeout then success", err: &azcore.ResponseError{StatusCode: http.StatusRequestTimeout}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "throttled then success", err: &azcore.ResponseError{StatusCode: http.StatusTooManyRequests}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "server error then success", err: &azcore.ResponseError{StatusCode: http.StatusServiceUnavailable}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "internal error then success", err: &azcore.ResponseError{StatusCode: http.StatusInternalServerError}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "bad gateway then success", err: &azcore.ResponseError{StatusCode: http.StatusBadGateway}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "gateway timeout then success", err: &azcore.ResponseError{StatusCode: http.StatusGatewayTimeout}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "azure in-use dependency then success", err: &azcore.ResponseError{StatusCode: http.StatusBadRequest, ErrorCode: "InUseSubnetCannotBeDeleted"}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "failed long-running operation then success", err: &processors.ResourceError{ID: "test", Inner: &azcore.ResponseError{StatusCode: http.StatusOK, ErrorCode: "Conflict"}}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "invalid request then success", err: &azcore.ResponseError{StatusCode: http.StatusBadRequest}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "unhandled not found then success", err: &azcore.ResponseError{StatusCode: http.StatusNotFound}, retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "unclassified error then success", err: errors.New("connection reset"), retries: 2, failures: 1, wantCalls: 2, wantElapsed: time.Minute},
		{name: "retry limit", err: &azcore.ResponseError{StatusCode: http.StatusConflict}, retries: 2, failures: 3, wantCalls: 3, wantElapsed: 2 * time.Minute, wantFailure: true},
		{name: "default retry limit", err: &azcore.ResponseError{StatusCode: http.StatusConflict}, retries: 20, failures: 21, wantCalls: 21, wantElapsed: 20 * time.Minute, wantFailure: true},
		{name: "no retries", err: &azcore.ResponseError{StatusCode: http.StatusConflict}, failures: 1, wantCalls: 1, wantFailure: true},
		{name: "success", retries: 20, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				d, client, _, _ := setupDeleteInputs(t)
				d.options = BicepOptions{DeleteRetryCount: tt.retries, DeleteRetryDelaySeconds: 60}
				id := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-account"
				calls := 0
				client.EXPECT().Delete(gomock.Any(), id).DoAndReturn(func(context.Context, string) error {
					calls++
					if calls <= tt.failures {
						return tt.err
					}
					return nil
				}).Times(tt.wantCalls)

				start := time.Now()
				err := d.Delete(t.Context(), driver.DeleteOptions{OutputResources: []rpv1.OutputResource{
					{ID: resources.MustParse(id), RadiusManaged: new(true)},
				}})
				require.Equal(t, tt.wantElapsed, time.Since(start))
				if !tt.wantFailure {
					require.NoError(t, err)
					return
				}
				var recipeError *recipes.RecipeError
				require.ErrorAs(t, err, &recipeError)
				require.Equal(t, recipes.RecipeDeletionFailed, recipeError.ErrorDetails.Code)
				require.Contains(t, recipeError.ErrorDetails.Message, fmt.Sprintf("after %d attempt(s)", tt.wantCalls))
				require.Contains(t, recipeError.ErrorDetails.Message, tt.err.Error())
			})
		})
	}
}

func Test_Bicep_Delete_CanceledDuringRetryDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d, client, _, _ := setupDeleteInputs(t)
		d.options = BicepOptions{DeleteRetryCount: 20, DeleteRetryDelaySeconds: 60}
		id := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-account"
		client.EXPECT().Delete(gomock.Any(), id).Return(&azcore.ResponseError{StatusCode: http.StatusConflict}).Times(1)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()

		start := time.Now()
		err := d.Delete(ctx, driver.DeleteOptions{OutputResources: []rpv1.OutputResource{
			{ID: resources.MustParse(id), RadiusManaged: new(true)},
		}})
		require.ErrorContains(t, err, context.Canceled.Error())
		require.ErrorContains(t, err, "after 1 attempt(s)")
		require.Equal(t, time.Second, time.Since(start))
	})
}

func Test_Bicep_Delete_CanceledBeforeFirstAttempt(t *testing.T) {
	d, client, _, _ := setupDeleteInputs(t)
	d.options = BicepOptions{DeleteRetryCount: 20, DeleteRetryDelaySeconds: 60}
	id := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-account"
	client.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := d.Delete(ctx, driver.DeleteOptions{OutputResources: []rpv1.OutputResource{
		{ID: resources.MustParse(id), RadiusManaged: new(true)},
	}})
	require.ErrorContains(t, err, context.Canceled.Error())
	require.ErrorContains(t, err, "after 0 attempt(s)")
}

// Test_Bicep_Delete_PermanentFailureDoesNotStopOtherRetries checks that a permanent failure on one resource does not
// stop another resource from using its own retries.
func Test_Bicep_Delete_PermanentFailureDoesNotStopOtherRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d, client, _, _ := setupDeleteInputs(t)
		d.options = BicepOptions{DeleteRetryCount: 20, DeleteRetryDelaySeconds: 60}
		parent := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-account"
		lock := parent + "/providers/Microsoft.Authorization/locks/test-lock"
		conflictSeen := make(chan struct{})
		gomock.InOrder(
			client.EXPECT().Delete(gomock.Any(), parent).DoAndReturn(func(context.Context, string) error {
				close(conflictSeen)
				return &azcore.ResponseError{StatusCode: http.StatusConflict}
			}),
			client.EXPECT().Delete(gomock.Any(), parent).Return(nil),
		)
		client.EXPECT().Delete(gomock.Any(), lock).DoAndReturn(func(context.Context, string) error {
			<-conflictSeen
			time.Sleep(time.Second)
			return &azcore.ResponseError{StatusCode: http.StatusForbidden, ErrorCode: "AuthorizationFailed"}
		}).Times(1)

		start := time.Now()
		err := d.Delete(t.Context(), driver.DeleteOptions{OutputResources: []rpv1.OutputResource{
			{ID: resources.MustParse(parent), RadiusManaged: new(true)},
			{ID: resources.MustParse(lock), RadiusManaged: new(true)},
		}})
		require.ErrorContains(t, err, "AuthorizationFailed")
		require.ErrorContains(t, err, fmt.Sprintf("failed to delete resource %q after 1 attempt(s)", lock))
		require.NotContains(t, err.Error(), fmt.Sprintf("%q after", parent))
		require.Equal(t, time.Minute, time.Since(start))
	})
}
