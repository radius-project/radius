// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
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
		name    string
		req     *Request
		want    *v1.ARMRequestContext
		wantErr bool
	}{
		{
			name:    "empty request",
			req:     &Request{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid id",
			req: &Request{
				ResourceID: "invalid",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "happy path",
			req: &Request{
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
			want: &v1.ARMRequestContext{
				ResourceID:     parsedResourceID,
				CorrelationID:  "test-correlation-id",
				OperationID:    opID,
				OperationType:  "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
				Traceparent:    "test-traceparent-id",
				HomeTenantID:   "test-home-tenant-id",
				ClientObjectID: "test-client-object-id",
				APIVersion:     "2021-01-01",
				AcceptLanguage: "en-US",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.ARMRequestContext()
			if (err != nil) != tt.wantErr {
				t.Errorf("Request.ARMRequestContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Request.ARMRequestContext() = %v, want %v", got, tt.want)
			}
		})
	}
}
