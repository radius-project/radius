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

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func TestGatewayConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresource.json")
	r := &GatewayResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	gw := dm.(*datamodel.Gateway)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", gw.ID)
	require.Equal(t, "gateway0", gw.Name)
	require.Equal(t, "Applications.Core/gateways", gw.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", gw.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", gw.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", gw.Properties.Hostname.Prefix)
	require.Equal(t, "mydestination", gw.Properties.Routes[0].Destination)
	require.Equal(t, "mypath", gw.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", gw.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", gw.Properties.URL)
	require.Equal(t, []rpv1.OutputResource(nil), gw.Properties.Status.OutputResources)
	require.Equal(t, "2022-03-15-privatepreview", gw.InternalMetadata.UpdatedAPIVersion)
}

func TestGatewayConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresourcedatamodel.json")
	r := &datamodel.Gateway{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &GatewayResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", *versioned.ID)
	require.Equal(t, "gateway0", *versioned.Name)
	require.Equal(t, "Applications.Core/gateways", *versioned.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", *versioned.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", *versioned.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", *versioned.Properties.Hostname.Prefix)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "mypath", *versioned.Properties.Routes[0].Path)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", *versioned.Properties.URL)
	require.Equal(t, "Deployment", versioned.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "kubernetes", versioned.Properties.Status.OutputResources[0]["Provider"])
}

func TestGatewaySSLPassthroughConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresource-with-sslpassthrough.json")
	r := &GatewayResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	gw := dm.(*datamodel.Gateway)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", gw.ID)
	require.Equal(t, "gateway0", gw.Name)
	require.Equal(t, "Applications.Core/gateways", gw.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", gw.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", gw.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", gw.Properties.Hostname.Prefix)
	require.Equal(t, "mydestination", gw.Properties.Routes[0].Destination)
	require.Equal(t, "mypath", gw.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", gw.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", gw.Properties.URL)
	require.Equal(t, []rpv1.OutputResource(nil), gw.Properties.Status.OutputResources)
	require.Equal(t, "2022-03-15-privatepreview", gw.InternalMetadata.UpdatedAPIVersion)
	require.Equal(t, true, gw.Properties.TLS.SSLPassthrough)
}

func TestGatewaySSLPassthroughConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresourcedatamodel-with-sslpassthrough.json")
	r := &datamodel.Gateway{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &GatewayResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", *versioned.ID)
	require.Equal(t, "gateway0", *versioned.Name)
	require.Equal(t, "Applications.Core/gateways", *versioned.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", *versioned.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", *versioned.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", *versioned.Properties.Hostname.Prefix)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "mypath", *versioned.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", *versioned.Properties.URL)
	require.Equal(t, "Deployment", versioned.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "kubernetes", versioned.Properties.Status.OutputResources[0]["Provider"])
	require.Equal(t, true, *versioned.Properties.TLS.SSLPassthrough)
}

func TestGatewayTLSTerminationConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresource-with-tlstermination.json")
	r := &GatewayResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	gw := dm.(*datamodel.Gateway)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", gw.ID)
	require.Equal(t, "gateway0", gw.Name)
	require.Equal(t, "Applications.Core/gateways", gw.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", gw.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", gw.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", gw.Properties.Hostname.Prefix)
	require.Equal(t, "mydestination", gw.Properties.Routes[0].Destination)
	require.Equal(t, "mypath", gw.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", gw.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", gw.Properties.URL)
	require.Equal(t, []rpv1.OutputResource(nil), gw.Properties.Status.OutputResources)
	require.Equal(t, "2022-03-15-privatepreview", gw.InternalMetadata.UpdatedAPIVersion)
	require.Equal(t, "secretname", gw.Properties.TLS.CertificateFrom)
	require.Equal(t, datamodel.TLSMinVersion13, gw.Properties.TLS.MinimumProtocolVersion)
}

func TestGatewayTLSTerminationConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresourcedatamodel-with-tlstermination.json")
	r := &datamodel.Gateway{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &GatewayResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", *versioned.ID)
	require.Equal(t, "gateway0", *versioned.Name)
	require.Equal(t, "Applications.Core/gateways", *versioned.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", *versioned.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", *versioned.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", *versioned.Properties.Hostname.Prefix)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "mypath", *versioned.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", *versioned.Properties.URL)
	require.Equal(t, "Deployment", versioned.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "kubernetes", versioned.Properties.Status.OutputResources[0]["Provider"])
	require.Equal(t, "secretname", *versioned.Properties.TLS.CertificateFrom)
	require.Equal(t, TLSMinVersionOne3, *versioned.Properties.TLS.MinimumProtocolVersion)
}

func TestGatewayTLSTerminationConvertVersionedToDataModel_NoMinProtocolVersion(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresource-with-tlstermination-nominprotocolversion.json")
	r := &GatewayResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	gw := dm.(*datamodel.Gateway)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", gw.ID)
	require.Equal(t, "gateway0", gw.Name)
	require.Equal(t, "Applications.Core/gateways", gw.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", gw.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", gw.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", gw.Properties.Hostname.Prefix)
	require.Equal(t, "mydestination", gw.Properties.Routes[0].Destination)
	require.Equal(t, "mypath", gw.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", gw.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", gw.Properties.URL)
	require.Equal(t, []rpv1.OutputResource(nil), gw.Properties.Status.OutputResources)
	require.Equal(t, "2022-03-15-privatepreview", gw.InternalMetadata.UpdatedAPIVersion)
	require.Equal(t, "secretname", gw.Properties.TLS.CertificateFrom)
	require.Equal(t, datamodel.TLSMinVersion12, gw.Properties.TLS.MinimumProtocolVersion)
}

func TestGatewayTLSTerminationConvertDataModelToVersioned_NoMinProtocolVersion(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("gatewayresourcedatamodel-with-tlstermination-nominprotocolversion.json")
	r := &datamodel.Gateway{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &GatewayResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/gateway0", *versioned.ID)
	require.Equal(t, "gateway0", *versioned.Name)
	require.Equal(t, "Applications.Core/gateways", *versioned.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", *versioned.Properties.Application)
	require.Equal(t, "myapp.mydomain.com", *versioned.Properties.Hostname.FullyQualifiedHostname)
	require.Equal(t, "myprefix", *versioned.Properties.Hostname.Prefix)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "mypath", *versioned.Properties.Routes[0].Path)
	require.Equal(t, "myreplaceprefix", *versioned.Properties.Routes[0].ReplacePrefix)
	require.Equal(t, "http://myprefix.myapp.mydomain.com", *versioned.Properties.URL)
	require.Equal(t, "Deployment", versioned.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "kubernetes", versioned.Properties.Status.OutputResources[0]["Provider"])
	require.Equal(t, "secretname", *versioned.Properties.TLS.CertificateFrom)
	require.Equal(t, TLSMinVersionOne2, *versioned.Properties.TLS.MinimumProtocolVersion)
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
