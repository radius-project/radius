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

package resourceproviders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	productmanifest "github.com/radius-project/radius/deploy/manifest"
)

const (
	testIconPlaneName        = "local"
	testIconResourceProvider = "MyCompany.Resources"
	testIconResourceType     = "testResources"
	testIconSVG              = `<svg xmlns="http://www.w3.org/2000/svg"><circle/></svg>`
	// testIconHash is sha256(testIconSVG) hex-encoded — must match the
	// bytes because the icon endpoint now performs a content-integrity
	// check on the served body before writing the response.
	testIconHash           = "7f9d93106668218aa422f68b0570c4d8676fb271a89d27a7e561da7517f3d9f5"
	testIconResourceTypeID = "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources/resourceTypes/testResources"
)

func newIconRequest(t *testing.T, hash string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "http://ucp/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/System.Resources/resourceproviders/MyCompany.Resources/resourcetypes/testResources/icons/"+hash, nil)
	require.NoError(t, err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("planeName", testIconPlaneName)
	rctx.URLParams.Add("resourceProviderName", testIconResourceProvider)
	rctx.URLParams.Add("resourceTypeName", testIconResourceType)
	rctx.URLParams.Add("hash", hash)

	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func newGetIconController(t *testing.T) (*GetIcon, *database.MockClient) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockDB := database.NewMockClient(ctrl)

	c, err := NewGetIcon(armrpc_controller.Options{DatabaseClient: mockDB})
	require.NoError(t, err)

	return c.(*GetIcon), mockDB
}

func TestGetIcon_Success(t *testing.T) {
	ctrl, mockDB := newGetIconController(t)

	rt := &datamodel.ResourceType{
		Properties: datamodel.ResourceTypeProperties{
			Icon:     to.Ptr(testIconSVG),
			IconHash: to.Ptr(testIconHash),
		},
	}
	mockDB.EXPECT().Get(gomock.Any(), testIconResourceTypeID).Return(&database.Object{Data: rt}, nil)

	req := newIconRequest(t, testIconHash)
	resp, err := ctrl.Run(context.Background(), nil, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	rec := httptest.NewRecorder()
	require.NoError(t, resp.Apply(context.Background(), rec, req))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/svg+xml; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "default-src 'none'; style-src 'unsafe-inline'; sandbox", rec.Header().Get("Content-Security-Policy"))
	require.Equal(t, testIconSVG, rec.Body.String())
}

func TestGetIcon_HashMismatch(t *testing.T) {
	ctrl, mockDB := newGetIconController(t)

	rt := &datamodel.ResourceType{
		Properties: datamodel.ResourceTypeProperties{
			Icon:     to.Ptr(testIconSVG),
			IconHash: to.Ptr(testIconHash),
		},
	}
	mockDB.EXPECT().Get(gomock.Any(), testIconResourceTypeID).Return(&database.Object{Data: rt}, nil)

	req := newIconRequest(t, "deadbeef")
	resp, err := ctrl.Run(context.Background(), nil, req)
	require.NoError(t, err)

	notFound, ok := resp.(*armrpc_rest.NotFoundResponse)
	require.True(t, ok, "expected NotFoundResponse, got %T", resp)
	require.Equal(t, v1.CodeNotFound, notFound.Body.Error.Code)
	require.Contains(t, notFound.Body.Error.Message, `icon with hash "deadbeef" was not found`)
}

// TestGetIcon_IntegrityCheckFails covers the content-addressed integrity
// contract: even when the URL-path hash matches the record's advertised
// IconHash, the endpoint re-computes sha256(body) before writing and
// returns 404 with cause "icon integrity check failed" if the body does
// not hash back to the requested hash. Simulates a data-tier corruption
// where the advertised hash and the stored bytes have diverged.
func TestGetIcon_IntegrityCheckFails(t *testing.T) {
	ctrl, mockDB := newGetIconController(t)

	// Record advertises testIconHash but the stored bytes are unrelated
	// content — sha256 of those bytes will not match testIconHash.
	rt := &datamodel.ResourceType{
		Properties: datamodel.ResourceTypeProperties{
			Icon:     to.Ptr(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`),
			IconHash: to.Ptr(testIconHash),
		},
	}
	mockDB.EXPECT().Get(gomock.Any(), testIconResourceTypeID).Return(&database.Object{Data: rt}, nil)

	req := newIconRequest(t, testIconHash)
	resp, err := ctrl.Run(context.Background(), nil, req)
	require.NoError(t, err)

	notFound, ok := resp.(*armrpc_rest.NotFoundResponse)
	require.True(t, ok, "expected NotFoundResponse, got %T", resp)
	require.Equal(t, v1.CodeNotFound, notFound.Body.Error.Code)
	require.Contains(t, notFound.Body.Error.Message, "icon integrity check failed")
}

func TestGetIcon_NoIcon(t *testing.T) {
	tests := []struct {
		name string
		icon *string
		hash *string
	}{
		// A record with no hash always 404's, regardless of what bytes exist.
		// After registration-time hash substitution this shape shouldn't occur
		// for freshly registered types, but the endpoint still rejects it
		// defensively.
		{name: "both nil", icon: nil, hash: nil},
		{name: "hash nil", icon: to.Ptr(testIconSVG), hash: nil},
		// Hash set but no bytes and the hash is NOT the product default —
		// nothing to serve, so still 404. The default-hash case is exercised
		// by TestGetIcon_DefaultHashServesEmbeddedBytes below.
		{name: "icon nil, non-default hash", icon: nil, hash: to.Ptr(testIconHash)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl, mockDB := newGetIconController(t)

			rt := &datamodel.ResourceType{
				Properties: datamodel.ResourceTypeProperties{
					Icon:     tc.icon,
					IconHash: tc.hash,
				},
			}
			mockDB.EXPECT().Get(gomock.Any(), testIconResourceTypeID).Return(&database.Object{Data: rt}, nil)

			req := newIconRequest(t, testIconHash)
			resp, err := ctrl.Run(context.Background(), nil, req)
			require.NoError(t, err)

			notFound, ok := resp.(*armrpc_rest.NotFoundResponse)
			require.True(t, ok, "expected NotFoundResponse, got %T", resp)
			require.Contains(t, notFound.Body.Error.Message, "resource type has no icon")
		})
	}
}

// TestGetIcon_DefaultHashServesEmbeddedBytes exercises the default-hash
// fallback: resource types registered without an icon are stored with the
// product default's hash but no bytes on the record; a GET for the default
// hash on such a record must still succeed by serving the embedded default
// SVG.
func TestGetIcon_DefaultHashServesEmbeddedBytes(t *testing.T) {
	defaultIcon := productmanifest.Default()

	ctrl, mockDB := newGetIconController(t)

	rt := &datamodel.ResourceType{
		Properties: datamodel.ResourceTypeProperties{
			// Icon stays nil — registration-time substitution stores only the
			// hash, not the bytes.
			Icon:     nil,
			IconHash: to.Ptr(defaultIcon.Hash),
		},
	}
	mockDB.EXPECT().Get(gomock.Any(), testIconResourceTypeID).Return(&database.Object{Data: rt}, nil)

	req := newIconRequest(t, defaultIcon.Hash)
	resp, err := ctrl.Run(context.Background(), nil, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	rec := httptest.NewRecorder()
	require.NoError(t, resp.Apply(context.Background(), rec, req))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/svg+xml; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Equal(t, string(defaultIcon.Bytes), rec.Body.String())
}

func TestGetIcon_ResourceTypeNotFound(t *testing.T) {
	ctrl, mockDB := newGetIconController(t)

	mockDB.EXPECT().Get(gomock.Any(), testIconResourceTypeID).Return(nil, &database.ErrNotFound{ID: testIconResourceTypeID})

	req := newIconRequest(t, testIconHash)
	resp, err := ctrl.Run(context.Background(), nil, req)
	require.NoError(t, err)

	notFound, ok := resp.(*armrpc_rest.NotFoundResponse)
	require.True(t, ok, "expected NotFoundResponse, got %T", resp)
	require.Equal(t, v1.CodeNotFound, notFound.Body.Error.Code)
	// The plain NotFoundResponse (no cause) is used when the resource type itself
	// doesn't exist, so the message should NOT contain the "resource type has no icon"
	// or "icon with hash" causes we produce when the resource does exist.
	require.NotContains(t, notFound.Body.Error.Message, "resource type has no icon")
	require.NotContains(t, notFound.Body.Error.Message, "icon with hash")
}
