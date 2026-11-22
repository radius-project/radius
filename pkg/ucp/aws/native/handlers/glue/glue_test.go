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

package glue

import (
	"errors"
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// partitionIndex returns a template for an index named name on sales.orders over keys.
func partitionIndex(name string, keys ...any) map[string]any {
	return map[string]any{
		"CatalogId":      account,
		"DatabaseName":   "sales",
		"TableName":      "orders",
		"PartitionIndex": map[string]any{"IndexName": name, "Keys": keys},
	}
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	maps.Copy(out, changes)
	return out
}

func Test_Suite_PartitionIndex(t *testing.T) {
	properties := partitionIndex("by_date", "year", "month")
	nativetest.Run(t, nativetest.Suite{
		TypeName:    partitionIndexType,
		Schema:      readFile(t, "aws-glue-partitionindex.json"),
		Metadata:    readFile(t, "aws-glue-partitionindex.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewPartitionIndex(newFakeAPI(store))
		},
		Properties: properties,
		Identifier: account + "|sales|orders|by_date",
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: partitionIndex("by_date", "year"),
		OtherForm:        map[string]any{"DatabaseName": "Sales", "TableName": "ORDERS"},
		SameResource: []map[string]any{
			with(properties, map[string]any{"DatabaseName": "Sales", "TableName": "ORDERS"}),
		},
		SameResourceNames: []string{account + "|SALES|Orders|by_date"},
		Invalid: []map[string]any{
			partitionIndex("by|date", "year"),
			with(properties, map[string]any{"TableName": "orders|archive"}),
			partitionIndex("by_date", "year", 42, "month"),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseID(identifier)
			require.NoError(t, err)
			return map[string]any{
				"CatalogId":      id.catalog,
				"DatabaseName":   id.database,
				"TableName":      id.table,
				"PartitionIndex": map[string]any{"IndexName": id.name, "Keys": []any{"year", "month"}},
			}
		},
		API: partitionIndexAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := glue.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h := NewPartitionIndex(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.Identifier(with(partitionIndex("By_Date", "year"), map[string]any{"DatabaseName": "Sales", "TableName": "Orders"}))
	require.NoError(t, err)
	require.Equal(t, account+"|sales|orders|By_Date", id, "Glue stores database and table names in lowercase, and not the index name")

	id, err = h.Identifier(map[string]any{"CatalogId": account, "DatabaseName": "sales", "TableName": "orders"})
	require.NoError(t, err)
	require.Empty(t, id, "an index without a name has no identifier")

	_, err = h.Identifier(partitionIndex("a|b", "year"))
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"CatalogId": account}})
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("a|b", "year")})
	require.ErrorIs(t, err, badName)
}

func Test_CreateRejected(t *testing.T) {
	h := NewPartitionIndex(newFakeAPI(awsfake.NewCloudControl()))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_price", "price")})
	require.ErrorContains(t, err, invalidInputCode, "AWS's error is returned")
}

func Test_ResolveIdentifier(t *testing.T) {
	h := NewPartitionIndex(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.ResolveIdentifier(t.Context(), region, account+"|Sales|ORDERS|By_Date")
	require.NoError(t, err)
	require.Equal(t, account+"|sales|orders|By_Date", id)
	_, err = h.ResolveIdentifier(t.Context(), region, "sales|orders")
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
}

func Test_BadIdentifier(t *testing.T) {
	h := NewPartitionIndex(newFakeAPI(awsfake.NewCloudControl()))
	for _, identifier := range []string{"sales|orders|by_date", "a|b|c|d|e", account + "||orders|by_date"} {
		_, err := h.Read(t.Context(), region, identifier)
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, identifier)
		require.True(t, failure.ClientError)
		require.ErrorAs(t, h.Delete(t.Context(), region, identifier), &failure)
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		require.ErrorAs(t, err, &failure)
	}
}

func Test_ReadPages(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewPartitionIndex(api)
	for i, key := range []string{"year", "month", "region"} {
		name := "i" + string(rune('0'+i))
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex(name, key)})
		require.NoError(t, err)
		if i < 2 {
			_, err = h.Progress(t.Context(), region, account+"|sales|orders|"+name, native.OperationCreate)
			require.NoError(t, err, "Glue builds one index on a table at a time")
		}
	}
	properties, err := h.Read(t.Context(), region, account+"|sales|orders|i2")
	require.NoError(t, err, "the third index is on the second page")
	require.Equal(t, map[string]any{"IndexName": "i2", "Keys": []any{"region"}}, properties["PartitionIndex"])
	require.Equal(t, "CREATING", properties["IndexStatus"])

	_, err = h.Read(t.Context(), region, account+"|sales|orders|missing")
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), region, account+"|sales|gone|i0")
	require.ErrorIs(t, err, native.ErrNotFound, "a table that doesn't exist has no index")

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}

// Test_CreateFails covers an index AWS fails to build. The create fails as rejected, so UCP creates the index again on
// the next deploy. AWS keeps the failed index listed, can't delete it, and lets a create reuse its name, so Read,
// Delete, and the progress of a delete treat it as absent.
func Test_CreateFails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewPartitionIndex(api)
	identifier := account + "|sales|secret|by_year"
	properties := with(partitionIndex("by_year", "year"), map[string]any{"TableName": "secret"})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)

	progress, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, progress.Done)
	require.Equal(t, "ACTIVE", progress.Waiting)

	progress, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.NotNil(t, progress.Failure)
	require.True(t, progress.Failure.ClientError)
	require.Equal(t, native.OutcomeRejected, progress.Failure.Outcome, "no index was made, so the next deploy creates it again")
	require.Equal(t, `AWS Glue failed to build the partition index "by_year" from the table's partitions (ENCRYPTED_PARTITION_ERROR). AWS keeps the failed index in the table's history. Fix the partitions, then deploy again to create the index again`, progress.Failure.Message)

	_, err = h.Read(t.Context(), region, identifier)
	require.ErrorIs(t, err, native.ErrNotFound, "a failed index reads as absent")
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound, "a delete removes only the record")
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "a delete doesn't wait for a failed index")

	// The next deploy creates the index again, with the same name, beside the failed one.
	api.holdCreating = true
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, "CREATING", got["IndexStatus"], "Read skips the failed index listed first")
	progress, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, progress.Done, "the create waits for the new index, not the failed one")
}

func Test_backfillFailure(t *testing.T) {
	require.Equal(t, `AWS Glue failed to build the partition index "i" from the table's partitions. AWS keeps the failed index in the table's history. Fix the partitions, then deploy again to create the index again`, backfillFailure("i", nil))
	require.Equal(t, `AWS Glue failed to build the partition index "i" from the table's partitions (MISSING_PARTITION_VALUE_ERROR, INTERNAL_ERROR). AWS keeps the failed index in the table's history. Fix the partitions, then deploy again to create the index again`,
		backfillFailure("i", []types.BackfillError{
			{Code: types.BackfillErrorCodeMissingPartitionValueError},
			{Code: types.BackfillErrorCodeInternalError},
			{Code: types.BackfillErrorCodeMissingPartitionValueError},
			{},
		}), "each code once, in order")
}

func Test_Delete(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewPartitionIndex(api)
	identifier := account + "|sales|orders|by_date"
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound)

	api.holdCreating = true
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
	require.NoError(t, err)
	require.ErrorContains(t, h.Delete(t.Context(), region, identifier), invalidInputCode, "AWS doesn't delete an index it is creating")

	api.holdCreating = false
	_, err = h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.NoError(t, h.Delete(t.Context(), region, identifier))
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "DELETING", Waiting: "deleted"}, progress)
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("other", "year")})
	require.NoError(t, err)
	_, err = h.Read(t.Context(), region, account+"|sales|orders|other")
	require.NoError(t, err)
	require.NoError(t, api.deleteDirect("other"))
	calls := api.deletes
	require.NoError(t, h.Delete(t.Context(), region, account+"|sales|orders|other"), "an index AWS is deleting had a delete reach AWS")
	require.Equal(t, calls, api.deletes, "no delete is sent for an index AWS is deleting")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("third", "year")})
	require.NoError(t, err)
	_, err = h.Read(t.Context(), region, account+"|sales|orders|third")
	require.NoError(t, err)
	api.errs["DeletePartitionIndex"] = fault("EntityNotFoundException")
	require.ErrorIs(t, h.Delete(t.Context(), region, account+"|sales|orders|third"), native.ErrNotFound, "an index deleted since the read is not found")
}

// Test_TableBusy covers a create or delete while another index on the table is being built or deleted. Glue builds one
// index on a table at a time, so the handler answers a conflict, which UCP answers with 409, and sends nothing.
func Test_TableBusy(t *testing.T) {
	conflict := func(t *testing.T, err error, message string) {
		t.Helper()
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.Conflict)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome, "nothing was made, so UCP drops the record")
		require.Equal(t, message, failure.Message)
	}
	const byMonth = `AWS Glue is building or deleting the partition index "by_month" on the table "sales.orders", and builds one index on a table at a time, so it changed nothing. ` +
		"Deploy again once that index is ACTIVE or deleted, and give partition indexes on one table dependsOn on each other."
	const another = `AWS Glue is building or deleting another partition index on the table "sales.orders", and builds one index on a table at a time, so it changed nothing. ` +
		"Deploy again once that index is ACTIVE or deleted, and give partition indexes on one table dependsOn on each other."

	for _, status := range []string{"CREATING", "DELETING"} {
		t.Run("create while another index is "+status, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			api.holdCreating = status == "CREATING"
			h := NewPartitionIndex(api)
			_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_month", "month")})
			require.NoError(t, err)
			if status == "DELETING" {
				_, err = h.Read(t.Context(), region, account+"|sales|orders|by_month")
				require.NoError(t, err)
				require.NoError(t, h.Delete(t.Context(), region, account+"|sales|orders|by_month"))
			}

			calls := api.creates
			_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
			conflict(t, err, byMonth)
			require.Equal(t, calls, api.creates, "no create is sent while the table is busy")
		})
	}

	t.Run("create the read didn't see as busy", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["CreatePartitionIndex"] = faultWith("ResourceNumberLimitExceededException", "Index by_month is in CREATING state. Only 1 index can be created or deleted simultaneously per table.")
		_, err := NewPartitionIndex(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
		conflict(t, err, another)
	})

	t.Run("delete while another index is CREATING", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewPartitionIndex(api)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
		require.NoError(t, err)
		_, err = h.Read(t.Context(), region, account+"|sales|orders|by_date")
		require.NoError(t, err)
		api.holdCreating = true
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_month", "month")})
		require.NoError(t, err)
		conflict(t, h.Delete(t.Context(), region, account+"|sales|orders|by_date"), another)
		got, err := h.Read(t.Context(), region, account+"|sales|orders|by_date")
		require.NoError(t, err)
		require.Equal(t, "ACTIVE", got["IndexStatus"], "the index is kept")
	})

	t.Run("the limit of 3 indexes isn't a conflict", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["CreatePartitionIndex"] = faultWith("ResourceNumberLimitExceededException", "Maximum number of indexes per table exceeded")
		_, err := NewPartitionIndex(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
		require.ErrorContains(t, err, "Maximum number of indexes")
		var failure *native.Failure
		require.False(t, errors.As(err, &failure) && failure.Conflict)
	})

	t.Run("a read that fails", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["GetPartitionIndexes"] = fault("ThrottlingException")
		_, err := NewPartitionIndex(api).Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
		require.ErrorContains(t, err, "ThrottlingException")
		require.Zero(t, api.creates)
	})
}

// Test_SameName checks that a database or table name AWS Glue returns in lowercase is the same name, so a template
// that sets it in another case is no change.
func Test_SameName(t *testing.T) {
	equivalent := NewPartitionIndex(nil).Options().Equivalent
	require.True(t, equivalent("DatabaseName", "Sales", "sales"))
	require.True(t, equivalent("TableName", "ORDERS", "orders"))
	require.False(t, equivalent("TableName", "orders", "orders_2"))
	require.False(t, equivalent("CatalogId", "A", "a"))
}

func Test_Update(t *testing.T) {
	h := NewPartitionIndex(newFakeAPI(awsfake.NewCloudControl()))
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region}))
	progress, err := h.Progress(t.Context(), region, "anything", native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_ReadFails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewPartitionIndex(api)
	api.errs["GetPartitionIndexes"] = fault("ThrottlingException")
	_, err := h.Read(t.Context(), region, account+"|sales|orders|by_date")
	require.ErrorContains(t, err, "ThrottlingException")
	require.NotErrorIs(t, err, native.ErrNotFound)
}

func Test_CreateAlreadyExists(t *testing.T) {
	h := NewPartitionIndex(newFakeAPI(awsfake.NewCloudControl()))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
	require.NoError(t, err)
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: partitionIndex("by_date", "year")})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, "the SDK sent the create once, so the index isn't one this deploy made")
	require.Empty(t, id, "a failed recording doesn't delete an index it didn't make")
	require.Contains(t, failure.Message, "AlreadyExistsException")
}

// Test_FailedHistory checks the fake's history of failed indexes: a page whose read fails an index that evicts another
// index of the same page, and a failed index deleted and failed again.
func Test_FailedHistory(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewPartitionIndex(api)
	for i := range 11 {
		name := "f" + string(rune('a'+i))
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(partitionIndex(name, "year"), map[string]any{"TableName": "secret"})})
		require.NoError(t, err)
		for range 2 {
			_, err = h.Progress(t.Context(), region, account+"|sales|secret|"+name, native.OperationCreate)
			require.NoError(t, err)
		}
	}
	history := api.failed[account+"|sales|secret"]
	require.Len(t, history, 10, "a table keeps its last 10 failed indexes")
	require.Equal(t, "fb", aws.ToString(history[0].IndexName), "the oldest is dropped")
}

// Test_ValueTypes checks that a value of the wrong type is rejected before any AWS call, instead of being sent as a
// shorter list or an empty name.
func Test_ValueTypes(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewPartitionIndex(api)
	for name, properties := range map[string]map[string]any{
		"a key that isn't a string":         partitionIndex("by_date", "year", 42, "month"),
		"keys that aren't a list":           with(partitionIndex("by_date"), map[string]any{"PartitionIndex": map[string]any{"IndexName": "by_date", "Keys": "year"}}),
		"an index that isn't an object":     with(partitionIndex("by_date", "year"), map[string]any{"PartitionIndex": "by_date"}),
		"an index name that isn't a string": with(partitionIndex("by_date", "year"), map[string]any{"PartitionIndex": map[string]any{"IndexName": 7, "Keys": []any{"year"}}}),
		"a catalog that isn't a string":     with(partitionIndex("by_date", "year"), map[string]any{"CatalogId": 123456789012}),
		"a database that isn't a string":    with(partitionIndex("by_date", "year"), map[string]any{"DatabaseName": true}),
		"a table that isn't a string":       with(partitionIndex("by_date", "year"), map[string]any{"TableName": []any{"orders"}}),
	} {
		t.Run(name, func(t *testing.T) {
			var failure *native.Failure
			_, err := h.Identifier(properties)
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
			require.ErrorAs(t, err, &failure)
			require.Zero(t, api.store.Count(partitionIndexType), "nothing is created")
		})
	}
}
