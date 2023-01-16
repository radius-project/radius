// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestGatewayConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("gatewayresource.json")
	r := &GatewayResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	ct := dm.(*datamodel.Gateway)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", ct.ID)
	require.Equal(t, "gateway0", ct.Name)
	require.Equal(t, "Applications.Core/gateways", ct.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", ct.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", ct.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", ct.Properties.Hostname.Prefix)
	require.Equal(t, "mydestination", ct.Properties.Routes[0].Destination)
	require.Equal(t, "mypath", ct.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", ct.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", ct.Properties.URL)
	require.Equal(t, []outputresource.OutputResource(nil), ct.Properties.Status.OutputResources)
	require.Equal(t, true, ct.Properties.TLS.SSLPassthrough)
	require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)
}

func TestGatewayConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("gatewayresourcedatamodel.json")
	r := &datamodel.Gateway{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &GatewayResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", r.ID)
	require.Equal(t, "gateway0", r.Name)
	require.Equal(t, "Applications.Core/gateways", r.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", r.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", r.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", r.Properties.Hostname.Prefix)
	require.Equal(t, "myreplaceprefix", r.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, false, r.Properties.TLS.SSLPassthrough)
	require.Equal(t, "mypath", r.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", r.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", r.Properties.URL)
	require.Equal(t, "Deployment", versioned.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "kubernetes", versioned.Properties.Status.OutputResources[0]["Provider"])
}

func TestGatewayConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &GatewayResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
