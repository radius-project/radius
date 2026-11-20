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

package redshift

import (
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const partnerTestRegion = "us-west-2"

// partnerSchema returns the hand-written schema of AWS::Redshift::PartnerIntegration.
func partnerSchema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/aws-redshift-partnerintegration.json")
	require.NoError(t, err)
	return string(b)
}

// partnerProps returns the properties of an integration of Fivetran with the database dev of the fake's cluster,
// with changes, where a nil value removes the property.
func partnerProps(changes map[string]any) map[string]any {
	properties := map[string]any{
		"AccountId":         partnerAccount,
		"ClusterIdentifier": partnerCluster,
		"DatabaseName":      "dev",
		"PartnerName":       "Fivetran",
	}
	for k, v := range changes {
		if v == nil {
			delete(properties, k)
		} else {
			properties[k] = v
		}
	}
	return properties
}

const partnerTestID = partnerAccount + "|" + partnerCluster + "|dev|Fivetran"

func newPartnerAPI() *partnerFake {
	store := awsfake.NewCloudControl()
	addPartnerClusters(store)
	return newPartnerFake(store)
}

func Test_Suite_PartnerIntegration(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    partnerType,
		Schema:      partnerSchema(t),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			addPartnerClusters(store)
			return NewPartnerIntegration(newPartnerFake(store))
		},
		Properties:       partnerProps(nil),
		Identifier:       partnerTestID,
		Changed:          partnerProps(nil),
		CreateOnlyChange: partnerProps(map[string]any{"PartnerName": "Etleap"}),
		IdentifierProperties: func(identifier string) map[string]any {
			parts := strings.Split(identifier, "|")
			return map[string]any{"AccountId": parts[0], "ClusterIdentifier": parts[1], "DatabaseName": parts[2], "PartnerName": parts[3]}
		},
		Invalid: []map[string]any{
			partnerProps(map[string]any{"AccountId": "12345"}),
			partnerProps(map[string]any{"ClusterIdentifier": "Analytics"}),
			partnerProps(map[string]any{"ClusterIdentifier": "1analytics"}),
			partnerProps(map[string]any{"DatabaseName": "1dev"}),
			partnerProps(map[string]any{"DatabaseName": strings.Repeat("d", 128)}),
			partnerProps(map[string]any{"PartnerName": "Five tran"}),
			partnerProps(map[string]any{"PartnerName": 5}),
		},
		API: partnerAPI,
	})
}

func Test_PartnerIntegration_ReadStatus(t *testing.T) {
	api := newPartnerAPI()
	h := NewPartnerIntegration(api)
	id, err := h.Create(t.Context(), native.CreateRequest{Region: partnerTestRegion, Properties: partnerProps(nil)})
	require.NoError(t, err)
	require.Equal(t, partnerTestID, id)
	properties, err := h.Read(t.Context(), partnerTestRegion, id)
	require.NoError(t, err)
	require.Equal(t, partnerProps(nil), properties, "AWS returns no status until the partner reports one")

	// The partner reports a status with UpdatePartnerStatus, which the handler never calls.
	stored := maps.Clone(api.store.Resource(partnerType, id))
	stored["Status"], stored["StatusMessage"] = "RuntimeFailure", "the sync failed"
	api.store.Put(partnerType, id, stored)
	properties, err = h.Read(t.Context(), partnerTestRegion, id)
	require.NoError(t, err)
	require.Equal(t, partnerProps(map[string]any{"Status": "RuntimeFailure", "StatusMessage": "the sync failed"}), properties)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: partnerTestRegion, Identifier: id, Desired: partnerProps(nil)}))
	require.Equal(t, 1, api.calls["AddPartner"], "an update sends nothing")
}

// Test_PartnerIntegration_OtherIntegrations checks that Read finds only the integration the identifier names, when
// the cluster has others of the same database or partner.
func Test_PartnerIntegration_OtherIntegrations(t *testing.T) {
	api := newPartnerAPI()
	h := NewPartnerIntegration(api)
	for _, properties := range []map[string]any{
		partnerProps(map[string]any{"PartnerName": "Etleap"}),
		partnerProps(map[string]any{"DatabaseName": "sales"}),
		partnerProps(map[string]any{"ClusterIdentifier": "reports"}),
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: partnerTestRegion, Properties: properties})
		require.NoError(t, err)
	}
	_, err := h.Read(t.Context(), partnerTestRegion, partnerTestID)
	require.ErrorIs(t, err, native.ErrNotFound)
	progress, err := h.Progress(t.Context(), partnerTestRegion, partnerTestID, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "a create AWS doesn't list is gone: %v", progress)
}

func Test_PartnerIntegration_Errors(t *testing.T) {
	api := newPartnerAPI()
	h := NewPartnerIntegration(api)

	require.ErrorIs(t, h.Delete(t.Context(), partnerTestRegion, partnerTestID), native.ErrNotFound)
	missingCluster := partnerAccount + "|missing|dev|Fivetran"
	_, err := h.Read(t.Context(), partnerTestRegion, missingCluster)
	require.ErrorIs(t, err, native.ErrNotFound, "a cluster AWS doesn't have has no integrations")
	require.ErrorIs(t, h.Delete(t.Context(), partnerTestRegion, missingCluster), native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: partnerTestRegion, Properties: partnerProps(map[string]any{"PartnerName": "Sisu"})})
	require.ErrorContains(t, err, partnerNotFound, "AWS's error is returned as it is; the real SDK's is a 404, which awsapi.Rejected reports as rejected")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: partnerTestRegion, Properties: partnerProps(map[string]any{"PartnerName": "Five tran"})})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, 1, api.calls["AddPartner"], "an invalid value is never sent")
	id, err := h.Identifier(partnerProps(map[string]any{"DatabaseName": nil}))
	require.NoError(t, err)
	require.Empty(t, id, "a template without every value gives no identifier")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: partnerTestRegion, Properties: partnerProps(map[string]any{"DatabaseName": nil})})
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, 1, api.calls["AddPartner"])

	for _, identifier := range []string{"", "a|b", partnerAccount + "|Analytics|dev|Fivetran", partnerTestID + "|extra"} {
		_, err = h.Read(t.Context(), partnerTestRegion, identifier)
		require.ErrorAs(t, err, &failure, identifier)
		require.ErrorAs(t, h.Delete(t.Context(), partnerTestRegion, identifier), &failure, identifier)
		_, err = h.Progress(t.Context(), partnerTestRegion, identifier, native.OperationCreate)
		require.ErrorAs(t, err, &failure, identifier)
	}

	_, err = h.Create(t.Context(), native.CreateRequest{Region: partnerTestRegion, Properties: partnerProps(nil)})
	require.NoError(t, err)
	require.NoError(t, h.Delete(t.Context(), partnerTestRegion, partnerTestID))
	progress, err := h.Progress(t.Context(), partnerTestRegion, partnerTestID, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "listed", Waiting: "deleted"}, progress, "AWS may still list a deleted integration")
	_, err = h.Progress(t.Context(), partnerTestRegion, partnerTestID, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	api.errs["DescribePartners"] = partnerFault("Throttling", "slow down")
	_, err = h.Progress(t.Context(), partnerTestRegion, partnerTestID, native.OperationCreate)
	require.ErrorContains(t, err, "Throttling")
	_, err = h.List(t.Context(), partnerTestRegion)
	require.Error(t, err, "AWS lists integrations only per cluster")
}
