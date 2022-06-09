// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestHandlers(t *testing.T) {
	handlerTests := []struct {
		url    string
		method string
	}{
		{
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/testrg/providers/applications.connector/mongodatabases?api-version=2022-03-15-privatepreview",
			method: http.MethodGet,
		}, {
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo0?api-version=2022-03-15-privatepreview",
			method: http.MethodPut,
		}, {
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo0?api-version=2022-03-15-privatepreview",
			method: http.MethodPatch,
		}, {
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo0?api-version=2022-03-15-privatepreview",
			method: http.MethodDelete,
		}, {
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo0?api-version=2022-03-15-privatepreview",
			method: http.MethodDelete,
		}, {
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo0/listsecrets?api-version=2022-03-15-privatepreview",
			method: http.MethodPost,
		}, {
			url:    "/providers/applications.connector/operations?api-version=2022-03-15-privatepreview",
			method: http.MethodGet,
		}, {
			url:    "/subscriptions/00000000-0000-0000-0000-000000000000?api-version=2.0",
			method: http.MethodPut,
		},
	}

	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSC := store.NewMockStorageClient(mctrl)

	mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(&store.Object{
		Data: map[string]interface{}{
			"name": "mongo0",
			"properties": map[string]interface{}{
				"provisioningState": "Updating",
			},
		},
	}, nil).AnyTimes()
	mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).AnyTimes()

	r := mux.NewRouter()
	AddRoutes(context.Background(), mockSP, r, "", true)

	for _, tt := range handlerTests {
		t.Run(tt.url, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), tt.method, "http://localhost"+tt.url, nil)
			var match mux.RouteMatch
			require.True(t, r.Match(req, &match))
		})
	}
}
