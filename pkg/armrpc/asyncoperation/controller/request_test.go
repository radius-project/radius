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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestTimeout(t *testing.T) {
	r := Request{}
	require.Equal(t, DefaultAsyncOperationTimeout, r.Timeout())

	testTimeout := time.Duration(200) * time.Minute
	r = Request{OperationTimeout: &testTimeout}
	require.Equal(t, testTimeout, r.Timeout())
}

func TestRequest_ARMRequestContext(t *testing.T) {
	opID := uuid.New()
	subscriptionID := uuid.New()
	resourceGroup := "test-resource-group"
	provider := "applications.core"
	resourceID := fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/%s/environments/test-environment", subscriptionID, resourceGroup, provider)
	parsedResourceID, err := resources.Parse(resourceID)
	require.NoError(t, err)

	tests := []struct {
		name string
		in   *Request
		out  *v1.ARMRequestContext
		err  error
	}{
		{
			name: "empty request",
			in:   &Request{},
			out:  nil,
			err:  errors.New("'' is not a valid resource id"),
		},
		{
			name: "invalid id",
			in: &Request{
				ResourceID: "invalid",
			},
			out: nil,
			err: errors.New("'invalid' is not a valid resource id"),
		},
		{
			name: "invalid operation type",
			in: &Request{
				ResourceID:    resourceID,
				OperationType: "invalid operation type",
			},
			out: nil,
			err: v1.ErrInvalidOperationType,
		},
		{
			name: "valid request",
			in: &Request{
				ResourceID:     resourceID,
				CorrelationID:  "test-correlation-id",
				OperationID:    opID,
				OperationType:  "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
				TraceparentID:  "test-traceparent-id",
				HomeTenantID:   "test-home-tenant-id",
				ClientObjectID: "test-client-object-id",
				APIVersion:     "2021-01-01",
				AcceptLanguage: "en-US",
			},
			out: &v1.ARMRequestContext{
				ResourceID:     parsedResourceID,
				CorrelationID:  "test-correlation-id",
				OperationID:    opID,
				OperationType:  rpctest.MustParseOperationType("APPLICATIONS.CORE/ENVIRONMENTS|PUT"),
				Traceparent:    "test-traceparent-id",
				HomeTenantID:   "test-home-tenant-id",
				ClientObjectID: "test-client-object-id",
				APIVersion:     "2021-01-01",
				AcceptLanguage: "en-US",
			},
			err: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rpcContext, err := tc.in.ARMRequestContext()
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.out, rpcContext)
			}
		})
	}
}
