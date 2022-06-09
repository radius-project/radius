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

var handlerTests = []struct {
	url        string
	method     string
	isAzureAPI bool
}{
	{
		url:        "/resourcegroups/testrg/providers/applications.connector/mongodatabases?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPut,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodPatch,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo?api-version=2022-03-15-privatepreview",
		method:     http.MethodDelete,
		isAzureAPI: false,
	}, {
		url:        "/resourcegroups/testrg/providers/applications.connector/mongodatabases/mongo/listsecrets?api-version=2022-03-15-privatepreview",
		method:     http.MethodPost,
		isAzureAPI: false,
	}, {
		url:        "/providers/applications.connector/operations?api-version=2022-03-15-privatepreview",
		method:     http.MethodGet,
		isAzureAPI: true,
	}, {
		url:        "/subscriptions/00000000-0000-0000-0000-000000000000?api-version=2.0",
		method:     http.MethodPut,
		isAzureAPI: true,
	},
}

func TestHandlers(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSC := store.NewMockStorageClient(mctrl)

	mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(&store.Object{}, nil).AnyTimes()
	mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).AnyTimes()

	assertRouters(t, "", true, mockSP)
	assertRouters(t, "/api.ucp.dev", false, mockSP)
}

func assertRouters(t *testing.T, pathBase string, isARM bool, mockSP *dataprovider.MockDataStorageProvider) {
	r := mux.NewRouter()
	err := AddRoutes(context.Background(), mockSP, r, pathBase, isARM)
	require.NoError(t, err)

	for _, tt := range handlerTests {
		if !isARM && tt.isAzureAPI {
			continue
		}

		uri := "http://localhost" + pathBase + tt.url
		if isARM && !tt.isAzureAPI {
			uri = "http://localhost" + pathBase + "/subscriptions/00000000-0000-0000-0000-000000000000" + tt.url
		}

		t.Run(uri, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), tt.method, uri, nil)
			var match mux.RouteMatch
			require.True(t, r.Match(req, &match))
		})
	}
}
