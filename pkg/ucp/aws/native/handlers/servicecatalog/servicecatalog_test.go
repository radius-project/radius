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

package servicecatalog

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

var share = map[string]any{"PortfolioId": "port-abc123", "AcceptLanguage": "zh"}

func Test_Suite_AcceptedPortfolioShare(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName: acceptedPortfolioShareType,
		Schema:   schema(t, "aws-servicecatalog-acceptedportfolioshare.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewAcceptedPortfolioShare(newFakeAPI(store))
		},
		IdentifierProperty: "PortfolioId",
		WriteOnly:          []string{"AcceptLanguage"},
		Properties:         share,
		Identifier:         "port-abc123",
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          share,
		CreateOnlyChange: map[string]any{"PortfolioId": "port-def456", "AcceptLanguage": "zh"},
		API:              acceptedPortfolioShareAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := servicecatalog.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Create(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAcceptedPortfolioShare(api)

	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"PortfolioId": "port-abc123"}})
	require.NoError(t, err)
	require.Equal(t, "port-abc123", id)
	require.Equal(t, map[string]any{"PortfolioId": "port-abc123"}, api.store.Resource(acceptedPortfolioShareType, id), "no language is sent when the template sets none")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"PortfolioId": "port-abc123"}})
	require.NoError(t, err, "accepting a share again changes nothing")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"PortfolioId": notShared}})
	require.ErrorContains(t, err, "ResourceNotFoundException")
}

func Test_Update(t *testing.T) {
	h := NewAcceptedPortfolioShare(newFakeAPI(awsfake.NewCloudControl()))
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "port-abc123", Desired: share}))
}

func Test_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAcceptedPortfolioShare(api)
	require.ErrorIs(t, h.Delete(t.Context(), region, "port-abc123"), native.ErrNotFound)

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: share})
	require.NoError(t, err)
	api.errs["RejectPortfolioShare"] = fault("ResourceNotFoundException")
	require.ErrorIs(t, h.Delete(t.Context(), region, "port-abc123"), native.ErrNotFound, "a share rejected since the read is not found")
}

func Test_ListAndProgress(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAcceptedPortfolioShare(api)
	for _, id := range []string{"port-1", "port-2", "port-3"} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"PortfolioId": id}})
		require.NoError(t, err)
	}
	require.NoError(t, h.Delete(t.Context(), region, "port-2"))
	progress, err := h.Progress(t.Context(), region, "port-2", native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "accepted", Waiting: "deleted"}, progress, "AWS lists a rejected share once more")
	_, err = h.Progress(t.Context(), region, "port-2", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	progress, err = h.Progress(t.Context(), region, "port-1", native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Equal(t, []native.Resource{
		{Identifier: "port-1", Properties: map[string]any{"PortfolioId": "port-1"}},
		{Identifier: "port-3", Properties: map[string]any{"PortfolioId": "port-3"}},
	}, resources)

	api.errs["ListAcceptedPortfolioShares"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
	_, err = h.Read(t.Context(), region, "port-1")
	require.ErrorContains(t, err, "ThrottlingException")
}
