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
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

var dataShareSalesID = dataShareSales + "|" + dataShareConsumerAccount

// dataShareUpdate updates the resource with the identifier from lastApplied to desired, with the patch UCP computes.
func dataShareUpdate(t *testing.T, h native.Handler, file string, identifier string, lastApplied map[string]any, desired map[string]any) error {
	t.Helper()
	current, err := h.Read(t.Context(), dataShareRegionName, identifier)
	require.NoError(t, err)
	c, err := json.Marshal(current)
	require.NoError(t, err)
	d, err := json.Marshal(desired)
	require.NoError(t, err)
	removed := []string{}
	for k := range lastApplied {
		if _, ok := desired[k]; !ok {
			removed = append(removed, k)
		}
	}
	patch, err := awsoperations.GeneratePatchWithOptions(c, d, []byte(dataShareSchema(t, file)), awsoperations.PatchOptions{LastApplied: lastApplied, Removed: removed})
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: dataShareRegionName, Identifier: identifier, Current: current, Desired: desired, Patch: patch})
}

// dataShareRequireFailure requires err to be a client failure with the outcome, and returns its message.
func dataShareRequireFailure(t *testing.T, err error, outcome native.Outcome) string {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, outcome, failure.Outcome)
	require.False(t, failure.Conflict)
	return failure.Message
}

// newDataShareProducer returns the authorization handler and its fake, as the producer account.
func newDataShareProducer() (*DataShareAuthorization, *dataShareFake) {
	f := newDataShareFake(awsfake.NewCloudControl(), dataShareProducer)
	return NewDataShareAuthorization(f), f
}

// dataShareAuthorized creates the authorization the properties give and waits until AWS reports it.
func dataShareAuthorized(t *testing.T, h *DataShareAuthorization, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: properties})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	return id
}

func Test_DataShareAuthorization_Identifier(t *testing.T) {
	h, _ := newDataShareProducer()
	id, err := h.Identifier(dataShareAuthorization)
	require.NoError(t, err)
	require.Equal(t, dataShareSalesID, id)
	id, err = h.Identifier(map[string]any{"DataShareArn": dataShareSales})
	require.NoError(t, err)
	require.Empty(t, id, "a template without the consumer doesn't give the identifier")
	// Only an account ID is verified to read back as sent, so the keywords AWS also takes are rejected before any call.
	for _, consumer := range []string{"ADX", "DataCatalog/" + dataShareConsumerAccount, "xxxxxxxxxxxx", "12345678901", "1234567890123"} {
		_, err := h.Identifier(dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": consumer}))
		require.Contains(t, dataShareRequireFailure(t, err, native.OutcomeRejected), "a 12-digit AWS account ID", consumer)
	}

	for _, bad := range []string{"", dataShareSales, "|" + dataShareConsumerAccount, dataShareSales + "|", "arn|" + dataShareConsumerAccount} {
		_, err := h.Read(t.Context(), dataShareRegionName, bad)
		dataShareRequireFailure(t, err, native.OutcomeRejected)
		dataShareRequireFailure(t, h.Delete(t.Context(), dataShareRegionName, bad), native.OutcomeRejected)
		_, err = h.Progress(t.Context(), dataShareRegionName, bad, native.OperationCreate)
		dataShareRequireFailure(t, err, native.OutcomeRejected)
	}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: map[string]any{"DataShareArn": dataShareSales}})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": "yes"})})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
	err = h.Update(t.Context(), native.UpdateRequest{Region: dataShareRegionName, Identifier: "bad", Desired: dataShareAuthorization,
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AllowWrites", Value: true}}})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
	err = h.Update(t.Context(), native.UpdateRequest{Region: dataShareRegionName, Identifier: dataShareSalesID, Desired: dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": "yes"})})
	dataShareRequireFailure(t, err, native.OutcomeRejected)
}

func Test_DataShareAuthorization_Create(t *testing.T) {
	h, f := newDataShareProducer()
	_, err := h.Read(t.Context(), dataShareRegionName, dataShareSalesID)
	require.ErrorIs(t, err, native.ErrNotFound, "a grant the producer hasn't authorized (PENDING_AUTHORIZATION) isn't an authorization")
	_, err = h.Progress(t.Context(), dataShareRegionName, dataShareSalesID, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "a create that was never sent doesn't wait for PENDING_AUTHORIZATION")

	id := dataShareAuthorized(t, h, dataShareAuthorization)
	properties, err := h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"DataShareArn": dataShareSales, "ConsumerIdentifier": dataShareConsumerAccount, "AllowWrites": false,
		"ProducerArn": dataShareProducerArn, "Status": "AUTHORIZED",
	}, properties, "the create is done without the consumer")

	for _, properties := range []map[string]any{
		dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": "111122223333"}),
		dataShareWith(dataShareAuthorization, map[string]any{"DataShareArn": dataShareUngranted}),
	} {
		_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: properties})
		require.Contains(t, dataShareRequireFailure(t, err, native.OutcomeRejected), "GRANT USAGE ON DATASHARE")
	}

	f.errs["AuthorizeDataShare"] = dataShareError("ThrottlingException", "slow down")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareAuthorization})
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_DataShareAuthorization_Statuses(t *testing.T) {
	h, f := newDataShareProducer()
	id := dataShareAuthorized(t, h, dataShareAuthorization)

	consumer := NewDataShareConsumerAssociation(newDataShareFake(f.store, dataShareConsumerAccount))
	_, err := consumer.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	require.NoError(t, err)
	properties, err := h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", properties["Status"], "an authorization the consumer associated is ACTIVE")

	f.reject(dataShareSales)
	_, err = h.Read(t.Context(), dataShareRegionName, id)
	require.ErrorIs(t, err, native.ErrNotFound, "a REJECTED data share is no longer authorized")
	id = dataShareAuthorized(t, h, dataShareAuthorization)

	require.NoError(t, h.Delete(t.Context(), dataShareRegionName, id))
	_, err = h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err, "AWS still reports the authorization once")
	_, err = h.Read(t.Context(), dataShareRegionName, id)
	require.ErrorIs(t, err, native.ErrNotFound, "a DEAUTHORIZED data share is no longer authorized")

	dataShareMarkInternal(dataShareAuthorizationType)(f.store, id)
	var other *native.OtherProductError
	_, err = h.Read(t.Context(), dataShareRegionName, id)
	require.ErrorAs(t, err, &other)
	require.Equal(t, id, other.Identifier)
	f.calls = map[string]int{}
	require.ErrorAs(t, h.Delete(t.Context(), dataShareRegionName, id), &other)
	err = h.Update(t.Context(), native.UpdateRequest{
		Region: dataShareRegionName, Identifier: id, Desired: dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": true}),
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AllowWrites", Value: true}},
	})
	require.ErrorAs(t, err, &other)
	require.Zero(t, f.calls["DeauthorizeDataShare"]+f.calls["AuthorizeDataShare"], "a data share Redshift made for itself is never changed")
}

func Test_DataShareAuthorization_Update(t *testing.T) {
	const file = "aws-redshift-datashareauthorization.json"
	h, f := newDataShareProducer()
	writes := dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": true})
	id := dataShareAuthorized(t, h, writes)
	consumer := NewDataShareConsumerAssociation(newDataShareFake(f.store, dataShareConsumerAccount))
	_, err := consumer.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	require.NoError(t, err)

	require.NoError(t, dataShareUpdate(t, h, file, id, writes, writes))
	require.Equal(t, 1, f.calls["AuthorizeDataShare"], "an unchanged template sends nothing")

	without := dataShareWith(writes, map[string]any{"AllowWrites": nil})
	require.NoError(t, dataShareUpdate(t, h, file, id, writes, without))
	require.Equal(t, 2, f.calls["AuthorizeDataShare"])
	progress, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	properties, err := h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, false, properties["AllowWrites"], "removing AllowWrites sets it back to AWS's default")
	require.Equal(t, "ACTIVE", properties["Status"], "authorizing again keeps the consumer's association")
}

func Test_DataShareAuthorization_Delete(t *testing.T) {
	h, f := newDataShareProducer()
	require.ErrorIs(t, h.Delete(t.Context(), dataShareRegionName, dataShareUngranted+"|"+dataShareConsumerAccount), native.ErrNotFound)

	id := dataShareAuthorized(t, h, dataShareAuthorization)
	require.NoError(t, h.Delete(t.Context(), dataShareRegionName, id))
	progress, err := h.Progress(t.Context(), dataShareRegionName, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "AUTHORIZED", Waiting: "DEAUTHORIZED"}, progress, "AWS still reports the authorization once")
	_, err = h.Progress(t.Context(), dataShareRegionName, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	require.ErrorIs(t, h.Delete(t.Context(), dataShareRegionName, id), native.ErrNotFound, "an authorization removed outside Radius is gone")

	id = dataShareAuthorized(t, h, dataShareAuthorization)
	f.errs["DeauthorizeDataShare"] = dataShareError(dataShareFault, "There is an error with the datashare.")
	require.ErrorContains(t, h.Delete(t.Context(), dataShareRegionName, id), dataShareFault, "InvalidDataShareFault isn't not found while AWS still has the authorization")
	properties, err := h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, "AUTHORIZED", properties["Status"])
}

// dataShareRacingDeauthorize removes the authorization before the delete's call, as another deploy would.
type dataShareRacingDeauthorize struct {
	*dataShareFake
}

func (r dataShareRacingDeauthorize) DeauthorizeDataShare(ctx context.Context, in *redshift.DeauthorizeDataShareInput, opts ...func(*redshift.Options)) (*redshift.DeauthorizeDataShareOutput, error) {
	if _, err := r.dataShareFake.DeauthorizeDataShare(ctx, in, opts...); err != nil {
		return nil, err
	}
	_, _ = r.dataShareFake.DescribeDataShares(ctx, &redshift.DescribeDataSharesInput{DataShareArn: in.DataShareArn})
	return r.dataShareFake.DeauthorizeDataShare(ctx, in, opts...)
}

func Test_DataShareAuthorization_DeleteRace(t *testing.T) {
	_, f := newDataShareProducer()
	h := NewDataShareAuthorization(dataShareRacingDeauthorize{f})
	id := dataShareAuthorized(t, h, dataShareAuthorization)
	require.ErrorIs(t, h.Delete(t.Context(), dataShareRegionName, id), native.ErrNotFound, "an authorization another deploy removed first is gone")
}

func Test_DataShareAuthorization_List(t *testing.T) {
	h, f := newDataShareProducer()
	f.grants[dataShareUngranted] = []string{dataShareConsumerAccount}
	for _, arn := range []string{dataShareSales, dataShareOrders, dataShareUngranted} {
		dataShareAuthorized(t, h, dataShareWith(dataShareAuthorization, map[string]any{"DataShareArn": arn}))
	}
	dataShareAuthorized(t, h, dataShareWith(dataShareAuthorization, map[string]any{"ConsumerIdentifier": dataShareOtherConsumer}))
	f.reject(dataShareOrders)

	resources, err := h.List(t.Context(), dataShareRegionName)
	require.NoError(t, err)
	ids := []string{}
	for _, r := range resources {
		ids = append(ids, r.Identifier)
	}
	require.ElementsMatch(t, []string{dataShareSalesID, dataShareSales + "|" + dataShareOtherConsumer, dataShareUngranted + "|" + dataShareConsumerAccount}, ids,
		"the list reads every page, and leaves out a grant that isn't authorized")
	require.Greater(t, f.calls["DescribeDataSharesForProducer"], 1)

	dataShareMarkInternal(dataShareAuthorizationType)(f.store, dataShareSalesID)
	resources, err = h.List(t.Context(), dataShareRegionName)
	require.NoError(t, err)
	require.Len(t, resources, 1, "a list leaves out a data share Redshift made for itself")

	f.errs["DescribeDataSharesForProducer"] = dataShareError("ThrottlingException", "slow down")
	_, err = h.List(t.Context(), dataShareRegionName)
	require.ErrorContains(t, err, "ThrottlingException")
}

// Test_DataShare_AcceptedSeparately follows a data share from the producer's authorization to the consumer's
// association and back: the authorization is done on its own, the consumer accepts it in its own account, and
// removing either side leaves the other as AWS does.
func Test_DataShare_AcceptedSeparately(t *testing.T) {
	producer, f := newDataShareProducer()
	consumerAPI := newDataShareFake(f.store, dataShareConsumerAccount)
	consumer := NewDataShareConsumerAssociation(consumerAPI)

	_, err := consumer.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	require.Contains(t, dataShareRequireFailure(t, err, native.OutcomeRejected), dataShareAuthorizationType, "the consumer can't accept before the producer authorizes")

	id := dataShareAuthorized(t, producer, dataShareAuthorization)
	require.Zero(t, consumerAPI.calls["AssociateDataShareConsumer"], "the authorization finishes without the consumer")

	association, err := consumer.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	require.NoError(t, err)
	progress, err := consumer.Progress(t.Context(), dataShareRegionName, association, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	properties, err := producer.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, string(types.DataShareStatusActive), properties["Status"])

	require.NoError(t, consumer.Delete(t.Context(), dataShareRegionName, association))
	_, _ = consumerAPI.DescribeDataSharesForConsumer(t.Context(), &redshift.DescribeDataSharesForConsumerInput{})
	properties, err = producer.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, string(types.DataShareStatusAuthorized), properties["Status"], "removing the association keeps the authorization")

	_, err = consumer.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	require.NoError(t, err)
	require.NoError(t, producer.Delete(t.Context(), dataShareRegionName, id))
	_, err = consumer.Read(t.Context(), dataShareRegionName, association)
	require.ErrorIs(t, err, native.ErrNotFound, "removing the authorization removes the consumer's association")
}

// Test_DataShareAuthorization_ConsumerAccount checks that the consumer account never takes its own association for
// this account's authorization: it reports a data share it associated with the entire account as ACTIVE too.
func Test_DataShareAuthorization_ConsumerAccount(t *testing.T) {
	producer, f := newDataShareProducer()
	id := dataShareAuthorized(t, producer, dataShareAuthorization)
	consumerAPI := newDataShareFake(f.store, dataShareConsumerAccount)
	association, err := NewDataShareConsumerAssociation(consumerAPI).Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareEntireProperties})
	require.NoError(t, err)

	h := NewDataShareAuthorization(consumerAPI)
	_, err = h.Read(t.Context(), dataShareRegionName, id)
	require.ErrorIs(t, err, native.ErrNotFound, "the consumer account's ACTIVE association isn't this account's authorization")
	require.Equal(t, 1, consumerAPI.calls["DescribeDataSharesForProducer"])
	_, err = h.Read(t.Context(), dataShareRegionName, dataShareOrders+"|"+dataShareConsumerAccount)
	require.ErrorIs(t, err, native.ErrNotFound, "the consumer account sees no data share the producer hasn't authorized for it")

	properties, err := producer.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, "ACTIVE", properties["Status"], "the producer account produces the data share")
	f.errs["DescribeDataSharesForProducer"] = dataShareError("ThrottlingException", "slow down")
	_, err = producer.Read(t.Context(), dataShareRegionName, id)
	require.ErrorContains(t, err, "ThrottlingException")

	_, err = producer.Read(t.Context(), dataShareRegionName, dataShareSales+"|111122223333")
	require.ErrorIs(t, err, native.ErrNotFound, "a consumer the data share isn't granted to has no authorization")
	require.NotEmpty(t, association)
}

// dataShareListsInternal returns the data shares the fake has from DescribeDataSharesForProducer, including any
// Redshift made for itself, which AWS left out in a run.
type dataShareListsInternal struct {
	*dataShareFake
}

func (l dataShareListsInternal) DescribeDataSharesForProducer(ctx context.Context, in *redshift.DescribeDataSharesForProducerInput, _ ...func(*redshift.Options)) (*redshift.DescribeDataSharesForProducerOutput, error) {
	share := l.producerView(dataShareSales)
	return &redshift.DescribeDataSharesForProducerOutput{DataShares: []types.DataShare{share}}, nil
}

func Test_DataShareAuthorization_ListInternal(t *testing.T) {
	_, f := newDataShareProducer()
	dataShareAuthorized(t, NewDataShareAuthorization(f), dataShareAuthorization)
	dataShareMarkInternal(dataShareAuthorizationType)(f.store, dataShareSalesID)
	resources, err := NewDataShareAuthorization(dataShareListsInternal{f}).List(t.Context(), dataShareRegionName)
	require.NoError(t, err)
	require.Empty(t, resources, "a list leaves out a data share Redshift made for itself, even when AWS returns it")
}

// dataShareProducesNone answers DescribeDataSharesForProducer with no data shares.
type dataShareProducesNone struct {
	*dataShareFake
}

func (dataShareProducesNone) DescribeDataSharesForProducer(context.Context, *redshift.DescribeDataSharesForProducerInput, ...func(*redshift.Options)) (*redshift.DescribeDataSharesForProducerOutput, error) {
	return &redshift.DescribeDataSharesForProducerOutput{}, nil
}

func Test_DataShareAuthorization_NotProduced(t *testing.T) {
	_, f := newDataShareProducer()
	id := dataShareAuthorized(t, NewDataShareAuthorization(f), dataShareAuthorization)
	_, err := NewDataShareConsumerAssociation(newDataShareFake(f.store, dataShareConsumerAccount)).Create(t.Context(),
		native.CreateRequest{Region: dataShareRegionName, Properties: dataShareConsumerAssociation})
	require.NoError(t, err)
	_, err = NewDataShareAuthorization(dataShareProducesNone{f}).Read(t.Context(), dataShareRegionName, id)
	require.ErrorIs(t, err, native.ErrNotFound, "an ACTIVE data share the account doesn't produce isn't its authorization")
}

// Test_DataShareAuthorization_ConfirmsWrites checks that a create or an update that changes AllowWrites finishes only
// once a read shows the change, and fails with an unknown outcome when no read does.
func Test_DataShareAuthorization_ConfirmsWrites(t *testing.T) {
	h, f := newDataShareProducer()
	writes := dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": true})
	id, err := h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: writes})
	require.NoError(t, err)
	properties, err := h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, true, properties["AllowWrites"], "the create finishes only once a read shows AllowWrites")

	require.NoError(t, dataShareUpdate(t, h, "aws-redshift-datashareauthorization.json", id, writes, dataShareAuthorization))
	properties, err = h.Read(t.Context(), dataShareRegionName, id)
	require.NoError(t, err)
	require.Equal(t, false, properties["AllowWrites"], "the update finishes only once a read shows AllowWrites")

	f.stuck[dataShareAuthorizationType+"|"+id] = true
	err = dataShareUpdate(t, h, "aws-redshift-datashareauthorization.json", id, dataShareAuthorization, writes)
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "AWS may have applied the change no read showed")
	require.Contains(t, failure.Message, "Deploy again")

	f.errs["DescribeDataShares"] = dataShareError("ThrottlingException", "slow down")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: dataShareRegionName, Properties: dataShareWith(writes, map[string]any{"ConsumerIdentifier": dataShareOtherConsumer})})
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "a throttled read after an accepted create is never reported as rejected")
	require.Contains(t, failure.Message, "ThrottlingException")
}

func Test_DataShareAuthorization_ConfirmStops(t *testing.T) {
	h, f := newDataShareProducer()
	writes := dataShareWith(dataShareAuthorization, map[string]any{"AllowWrites": true})
	id := dataShareAuthorized(t, h, dataShareAuthorization)

	f.errs["AuthorizeDataShare"] = dataShareError(dataShareParameterFault, "bad value")
	require.ErrorContains(t, dataShareUpdate(t, h, "aws-redshift-datashareauthorization.json", id, dataShareAuthorization, writes), dataShareParameterFault)
	delete(f.errs, "AuthorizeDataShare")

	f.stuck[dataShareAuthorizationType+"|"+id] = true
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := h.confirm(ctx, dataShareRegionName, id, true)
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "a deploy that stops waiting can't tell whether AWS applied the change")
	require.Contains(t, failure.Message, context.Canceled.Error())
}

// Test_DataShareConfirm_Canceled checks that a confirmation never reads AWS once its context is canceled, before the
// first read or after any read, and reports the cancellation with an unknown outcome.
func Test_DataShareConfirm_Canceled(t *testing.T) {
	for _, cancelAfter := range []int{0, 1, 3} {
		ctx, cancel := context.WithCancel(t.Context())
		reads := 0
		if cancelAfter == 0 {
			cancel()
		}
		err := dataShareConfirm(ctx, "the authorization", func() (bool, error) {
			reads++
			if reads == cancelAfter {
				cancel()
			}
			return false, nil
		})
		cancel()
		require.Equal(t, cancelAfter, reads, "no read after the context is canceled")
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomeUnknown, failure.Outcome)
		require.Contains(t, failure.Message, context.Canceled.Error())
	}
}
