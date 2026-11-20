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
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const dataShareConsumerFile = "aws-redshift-datashareconsumerassociation.json"

var (
	dataShareEntireProperties = map[string]any{"DataShareArn": dataShareSales, "AssociateEntireAccount": true}
	dataShareRegionProperties = map[string]any{"DataShareArn": dataShareSales, "ConsumerRegion": "us-east-1"}
)

// newDataShareConsumer returns the association handler and its fake, as the consumer account, with the data shares
// sales and orders authorized for it.
func newDataShareConsumer() (*DataShareConsumerAssociation, *dataShareFake) {
	f := newDataShareConsumerFake(awsfake.NewCloudControl())
	return NewDataShareConsumerAssociation(f), f
}

// dataShareAssociated creates the association the properties give and waits until AWS reports it.
func dataShareAssociated(t *testing.T, h *DataShareConsumerAssociation, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: properties})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	return id
}

func Test_DataShareConsumerAssociation_Identifier(t *testing.T) {
	h, _ := newDataShareConsumer()
	for _, tt := range []struct {
		properties map[string]any
		want       string
	}{
		{dataShareConsumerAssociation, dataShareSales + "|" + dataShareNamespace},
		{dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerArn": dataShareCluster}), dataShareSales + "|" + dataShareCluster},
		{dataShareEntireProperties, dataShareSales + "|entire-account"},
		{dataShareRegionProperties, dataShareSales + "|us-east-1"},
		{dataShareWith(dataShareRegionProperties, map[string]any{"AssociateEntireAccount": false}), dataShareSales + "|us-east-1"},
		{map[string]any{"ConsumerArn": dataShareNamespace}, ""},
	} {
		id, err := h.Identifier(tt.properties)
		require.NoError(t, err)
		require.Equal(t, tt.want, id)
	}

	for _, bad := range []string{"", dataShareSales, dataShareSales + "|", dataShareSales + "|arn:aws:glue:us-west-2:444455556666:catalog", "sales|entire-account"} {
		_, err := h.Read(t.Context(), dataShareRegionName, bad)
		dataShareRequireFailure(t, err, native.OutcomeRejected)
		dataShareRequireFailure(t, h.Delete(t.Context(), dataShareRegionName, bad), native.OutcomeRejected)
		_, err = h.Progress(t.Context(), dataShareRegionName, bad, native.OperationCreate)
		dataShareRequireFailure(t, err, native.OutcomeRejected)
	}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: map[string]any{"AssociateEntireAccount": true}})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareWith(dataShareConsumerAssociation, map[string]any{"ConsumerRegion": "us-east-1"})})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
	err = h.Update(t.Context(), native.UpdateRequest{Region: dataShareRegionName, Identifier: dataShareSales + "|" + dataShareNamespace,
		Desired: dataShareWith(dataShareConsumerAssociation, map[string]any{"AllowWrites": "yes"})})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
	err = h.Update(t.Context(), native.UpdateRequest{Region: dataShareRegionName, Identifier: "bad", Desired: dataShareConsumerAssociation,
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AllowWrites", Value: true}}})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
}

func Test_DataShareConsumerAssociation_Create(t *testing.T) {
	h, f := newDataShareConsumer()
	for _, properties := range []map[string]any{dataShareConsumerAssociation, dataShareEntireProperties, dataShareRegionProperties} {
		id, _ := h.Identifier(properties)
		_, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationCreate)
		require.ErrorIs(t, err, native.ErrNotFound, "a create that was never sent doesn't wait for an AVAILABLE data share")
	}

	id := dataShareAssociated(t, h, dataShareConsumerAssociation)
	properties, err := h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"DataShareArn": dataShareSales, "ConsumerArn": dataShareNamespace, "AssociateEntireAccount": false, "AllowWrites": false, "ProducerArn": dataShareProducerArn,
	}, properties)
	region := dataShareAssociated(t, h, dataShareRegionProperties)
	properties, err = h.Read(t.Context(), dataShareRegionName, region)
	require.NoError(t, err)
	require.Equal(t, "us-east-1", properties["ConsumerRegion"], "a namespace and a Region association keep each other")
	_, err = h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareWith(dataShareEntireProperties, map[string]any{"DataShareArn": dataShareUngranted})})
	require.Contains(t, dataShareRequireFailure(t, err, native.OutcomeRejected), dataShareAuthorizationType)
	require.Equal(t, 2, f.calls["AssociateDataShareConsumer"], "a data share no producer authorized is never sent")

	f.errs["DescribeDataSharesForConsumer"] = dataShareError("ThrottlingException", "slow down")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareEntireProperties})
	require.ErrorContains(t, err, "ThrottlingException")
}

// dataShareRevokingAssociate makes AWS answer InvalidDataShareFault, as when the producer removed its authorization
// after the handler read the data share.
type dataShareRevokingAssociate struct {
	*dataShareFake
}

func (r dataShareRevokingAssociate) AssociateDataShareConsumer(ctx context.Context, in *redshift.AssociateDataShareConsumerInput, opts ...func(*redshift.Options)) (*redshift.AssociateDataShareConsumerOutput, error) {
	r.store.Remove(dataShareAuthorizationType, aws.ToString(in.DataShareArn)+"|"+dataShareConsumerAccount)
	return r.dataShareFake.AssociateDataShareConsumer(ctx, in, opts...)
}

func Test_DataShareConsumerAssociation_CreateRevoked(t *testing.T) {
	_, f := newDataShareConsumer()
	h := NewDataShareConsumerAssociation(dataShareRevokingAssociate{f})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	message := dataShareRequireFailure(t, err, native.OutcomeRejected)
	require.Contains(t, message, dataShareAuthorizationType)
	require.Contains(t, message, dataShareFault)
}

// Test_DataShareConsumerAssociation_Replaced checks that a create never makes AWS replace another association: one
// with the entire account replaces those with a namespace or a Region, and the other way around.
func Test_DataShareConsumerAssociation_Replaced(t *testing.T) {
	h, f := newDataShareConsumer()
	namespace := dataShareAssociated(t, h, dataShareConsumerAssociation)
	region := dataShareAssociated(t, h, dataShareRegionProperties)

	_, err := h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareEntireProperties})
	require.Contains(t, dataShareRequireFailure(t, err, native.OutcomeRejected), "its 2 associations with a namespace or a Region")
	require.Equal(t, 2, f.calls["AssociateDataShareConsumer"], "UCP sends nothing")
	for _, id := range []string{namespace, region} {
		_, err = h.Read(t.Context(), dataShareRegionName, id)
		require.NoError(t, err, "AWS keeps %s", id)
	}

	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareWith(dataShareEntireProperties, map[string]any{"DataShareArn": dataShareOrders})})
	require.NoError(t, err, "another data share's associations don't count")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareWith(dataShareConsumerAssociation, map[string]any{"DataShareArn": dataShareOrders})})
	require.Contains(t, dataShareRequireFailure(t, err, native.OutcomeRejected), "its association with the entire account")
	entire, _ := h.Identifier(dataShareWith(dataShareEntireProperties, map[string]any{"DataShareArn": dataShareOrders}))
	_, err = h.Read(t.Context(), dataShareRegionName, entire)
	require.NoError(t, err, "AWS keeps the association with the entire account")
}

func Test_DataShareConsumerAssociation_Update(t *testing.T) {
	h, f := newDataShareConsumer()
	for _, properties := range []map[string]any{dataShareConsumerAssociation, dataShareEntireProperties, dataShareRegionProperties} {
		if properties["AssociateEntireAccount"] == true {
			properties = dataShareWith(properties, map[string]any{"DataShareArn": dataShareOrders})
		}
		writes := dataShareWith(properties, map[string]any{"AllowWrites": true})
		id := dataShareAssociated(t, h, writes)
		before := f.calls["AssociateDataShareConsumer"]

		require.NoError(t, dataShareUpdate(t, h, dataShareConsumerFile, id, writes, writes))
		require.Equal(t, before, f.calls["AssociateDataShareConsumer"], "an unchanged template sends nothing")

		without := dataShareWith(writes, map[string]any{"AllowWrites": nil})
		require.NoError(t, dataShareUpdate(t, h, dataShareConsumerFile, id, writes, without))
		require.Equal(t, before+1, f.calls["AssociateDataShareConsumer"])
		progress, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationUpdate)
		require.NoError(t, err)
		require.True(t, progress.Done)
		read, err := h.Read(t.Context(), dataShareRegionName, id)
		require.NoError(t, err)
		require.Equal(t, false, read["AllowWrites"], "removing AllowWrites sets it back to AWS's default")
	}
	require.Len(t, f.store.List(dataShareConsumerType), 3, "associating again the same way replaces nothing")

	dataShareMarkInternal(dataShareConsumerType)(f.store, dataShareSales+"|"+dataShareNamespace)
	err := h.Update(t.Context(), native.UpdateRequest{
		Region: dataShareRegionName, Identifier: dataShareSales + "|" + dataShareNamespace, Desired: dataShareConsumerAssociation,
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AllowWrites", Value: true}},
	})
	var other *native.OtherProductError
	require.ErrorAs(t, err, &other)
}

func Test_DataShareConsumerAssociation_Delete(t *testing.T) {
	h, f := newDataShareConsumer()
	require.ErrorIs(t, h.Delete(t.Context(), dataShareRegionName, dataShareUngranted+"|entire-account"), native.ErrNotFound)

	for _, properties := range []map[string]any{dataShareConsumerAssociation, dataShareRegionProperties, dataShareWith(dataShareEntireProperties, map[string]any{"DataShareArn": dataShareOrders})} {
		id := dataShareAssociated(t, h, properties)
		require.NoError(t, h.Delete(t.Context(), dataShareRegionName, id))
		progress, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationDelete)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "ACTIVE", Waiting: "disassociated"}, progress, "AWS still reports the association once")
		_, err = h.Progress(t.Context(), dataShareRegionName, id, native.OperationDelete)
		require.ErrorIs(t, err, native.ErrNotFound)
		require.ErrorIs(t, h.Delete(t.Context(), dataShareRegionName, id), native.ErrNotFound, "an association removed outside Radius is gone")
	}

	id := dataShareAssociated(t, h, dataShareConsumerAssociation)
	f.errs["DisassociateDataShareConsumer"] = dataShareError(dataShareFault, "There is an error with the datashare.")
	require.ErrorContains(t, h.Delete(t.Context(), dataShareRegionName, id), dataShareFault, "InvalidDataShareFault isn't not found while AWS still has the association")

	dataShareMarkInternal(dataShareConsumerType)(f.store, id)
	var other *native.OtherProductError
	delete(f.errs, "DisassociateDataShareConsumer")
	require.ErrorAs(t, h.Delete(t.Context(), dataShareRegionName, id), &other)
	_, err := h.Read(t.Context(), dataShareRegionName, id)
	require.ErrorAs(t, err, &other)
}

// Test_DataShareConsumerAssociation_ProducerAccount checks that the producer account never takes its own
// authorization for a consumer association: it sees no data share as a consumer.
func Test_DataShareConsumerAssociation_ProducerAccount(t *testing.T) {
	store := awsfake.NewCloudControl()
	consumer := NewDataShareConsumerAssociation(newDataShareConsumerFake(store))
	id := dataShareAssociated(t, consumer, dataShareEntireProperties)

	h := NewDataShareConsumerAssociation(newDataShareFake(store, dataShareProducer))
	_, err := h.Read(t.Context(), dataShareRegionName, id)
	require.ErrorIs(t, err, native.ErrNotFound)
	resources, err := h.List(t.Context(), dataShareRegionName)
	require.NoError(t, err)
	require.Empty(t, resources)
}

func Test_DataShareConsumerAssociation_List(t *testing.T) {
	h, f := newDataShareConsumer()
	f.store.Put(dataShareAuthorizationType, dataShareUngranted+"|"+dataShareConsumerAccount, map[string]any{"DataShareArn": dataShareUngranted})
	want := []string{}
	for _, properties := range []map[string]any{
		dataShareConsumerAssociation,
		dataShareRegionProperties,
		dataShareWith(dataShareEntireProperties, map[string]any{"DataShareArn": dataShareOrders}),
		dataShareWith(dataShareConsumerAssociation, map[string]any{"DataShareArn": dataShareUngranted, "ConsumerArn": dataShareCluster}),
	} {
		want = append(want, dataShareAssociated(t, h, properties))
	}
	resources, err := h.List(t.Context(), dataShareRegionName)
	require.NoError(t, err)
	ids := []string{}
	for _, r := range resources {
		ids = append(ids, r.Identifier)
		read, err := h.Read(t.Context(), dataShareRegionName, r.Identifier)
		require.NoError(t, err)
		require.Equal(t, read, r.Properties, "the list and a read agree")
	}
	require.ElementsMatch(t, want, ids, "the list reads every page, and leaves out a data share that is only AVAILABLE")
	require.Greater(t, f.calls["DescribeDataSharesForConsumer"], 1)

	dataShareMarkInternal(dataShareConsumerType)(f.store, want[0])
	resources, err = h.List(t.Context(), dataShareRegionName)
	require.NoError(t, err)
	require.Len(t, resources, 2, "a list leaves out the associations of a data share Redshift made for itself")

	f.errs["DescribeDataSharesForConsumer"] = dataShareError("ThrottlingException", "slow down")
	_, err = h.List(t.Context(), dataShareRegionName)
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_DataShareConsumerAssociation_UnknownConsumer(t *testing.T) {
	for _, id := range []string{"DataCatalog/" + dataShareConsumerAccount, "arn:aws:glue:us-west-2:" + dataShareConsumerAccount + ":catalog", ""} {
		_, ok := dataShareConsumerOf(&types.DataShareAssociation{ConsumerIdentifier: aws.String(id)})
		require.False(t, ok, "%q isn't an association the handler serves", id)
	}
	share := &types.DataShare{DataShareAssociations: []types.DataShareAssociation{{ConsumerIdentifier: aws.String("DataCatalog/" + dataShareConsumerAccount), Status: types.DataShareStatusActive}}}
	require.NoError(t, dataShareReplaced(share, dataShareConsumer{entireAccount: true}), "an association the handler doesn't serve is never one AWS replaces")
}

func Test_DataShareConsumerAssociation_DeleteFails(t *testing.T) {
	h, f := newDataShareConsumer()
	id := dataShareAssociated(t, h, dataShareConsumerAssociation)
	f.errs["DescribeDataSharesForConsumer"] = dataShareError("ThrottlingException", "slow down")
	require.ErrorContains(t, h.Delete(t.Context(), dataShareRegionName, id), "ThrottlingException")
	require.Zero(t, f.calls["DisassociateDataShareConsumer"], "a delete that can't read the data share sends nothing")
}
