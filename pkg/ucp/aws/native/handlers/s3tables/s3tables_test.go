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

package s3tables

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region       = "us-west-2"
	tableBucket  = "arn:aws:s3tables:us-west-2:123456789012:bucket/sales"
	tableArn     = tableBucket + "/table/orders"
	role         = "arn:aws:iam::123456789012:role/S3TablesReplicationRole"
	destination1 = "arn:aws:s3tables:us-east-1:123456789012:bucket/sales-dr"
	destination2 = "arn:aws:s3tables:eu-west-1:123456789012:bucket/sales-eu"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// replication returns the properties of the replication of table to destinations.
func replication(table string, destinations ...string) map[string]any {
	list := []any{}
	for _, d := range destinations {
		list = append(list, map[string]any{"DestinationTableBucketARN": d})
	}
	return map[string]any{
		"TableArn": table,
		"Configuration": map[string]any{
			"Role":  role,
			"Rules": []any{map[string]any{"Destinations": list}},
		},
	}
}

func Test_Suite_TableReplication(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    tableReplicationType,
		Schema:      readFile(t, "aws-s3tables-tablereplication.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewTableReplication(newFakeAPI(store))
		},
		IdentifierProperty: "TableArn",
		Properties:         replication(tableArn, destination1, destination2),
		Identifier:         tableArn,
		Changed:            replication(tableArn, destination2),
		CreateOnlyChange:   replication(tableBucket+"/table/returns", destination1),
		// AWS may return the destinations in another order.
		OtherForm: map[string]any{"Configuration": replication(tableArn, destination2, destination1)["Configuration"]},
		API:       tableReplicationAPI,
	})
}

func Test_Update_SendsTheVersionToken(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTableReplication(api)
	ctx := t.Context()
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: replication(tableArn, destination1)})
	require.NoError(t, err)

	require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: tableArn, Desired: replication(tableArn, destination2)}))
	props, err := h.Read(ctx, region, tableArn)
	require.NoError(t, err)
	require.Equal(t, replication(tableArn, destination2), props)
	require.Equal(t, []string{"PutTableReplication", "GetTableReplication", "PutTableReplication", "GetTableReplication"}, api.calls)

	t.Run("a configuration someone changed after the read is not replaced", func(t *testing.T) {
		stale := &staleAPI{fakeAPI: api}
		err := NewTableReplication(stale).Update(ctx, native.UpdateRequest{Region: region, Identifier: tableArn, Desired: replication(tableArn, destination1)})
		require.ErrorContains(t, err, conflictCode)
		props, err := h.Read(ctx, region, tableArn)
		require.NoError(t, err)
		require.Equal(t, replication(tableArn, destination2), props)
	})
}

// staleAPI changes the configuration right after each read, as someone outside Radius could.
type staleAPI struct {
	*fakeAPI
}

func (s *staleAPI) GetTableReplication(ctx context.Context, in *s3tables.GetTableReplicationInput, optFns ...func(*s3tables.Options)) (*s3tables.GetTableReplicationOutput, error) {
	out, err := s.fakeAPI.GetTableReplication(ctx, in, optFns...)
	if err != nil {
		return nil, err
	}
	_, err = s.fakeAPI.PutTableReplication(ctx, &s3tables.PutTableReplicationInput{TableArn: in.TableArn, Configuration: out.Configuration, VersionToken: out.VersionToken})
	return out, err
}

func Test_Create_RejectsAMissingConfiguration(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTableReplication(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"TableArn": tableArn}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Empty(t, api.calls, "nothing reaches AWS")

	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: tableArn, Desired: map[string]any{"TableArn": tableArn, "Configuration": map[string]any{"Rules": "all"}}})
	require.ErrorAs(t, err, &failure)
	require.Contains(t, failure.Message, "Configuration")
	require.Empty(t, api.calls, "nothing reaches AWS")
}

func Test_Fake_RequiredParameters(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	ctx := t.Context()
	var params smithy.InvalidParamsError
	_, err := api.GetTableReplication(ctx, &s3tables.GetTableReplicationInput{})
	require.ErrorAs(t, err, &params)
	_, err = api.PutTableReplication(ctx, &s3tables.PutTableReplicationInput{TableArn: aws.String(tableArn)})
	require.ErrorAs(t, err, &params)
	_, err = api.DeleteTableReplication(ctx, &s3tables.DeleteTableReplicationInput{TableArn: aws.String(tableArn)})
	require.ErrorAs(t, err, &params)
}

func Test_Update_ConfigurationWithoutRequiredValues(t *testing.T) {
	ctx := t.Context()
	for name, configuration := range map[string]map[string]any{
		"no role":         {"Rules": []any{map[string]any{"Destinations": []any{map[string]any{"DestinationTableBucketARN": destination1}}}}},
		"no rules":        {"Role": role},
		"no destinations": {"Role": role, "Rules": []any{map[string]any{}}},
		"no bucket ARN":   {"Role": role, "Rules": []any{map[string]any{"Destinations": []any{map[string]any{}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			h := NewTableReplication(newFakeAPI(awsfake.NewCloudControl()))
			_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"TableArn": tableArn, "Configuration": configuration}})
			var failure *native.Failure
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
		})
	}
}

func Test_NotFound(t *testing.T) {
	h := NewTableReplication(newFakeAPI(awsfake.NewCloudControl()))
	ctx := t.Context()
	_, err := h.Read(ctx, region, tableArn)
	require.ErrorIs(t, err, native.ErrNotFound, "a table with no configuration")
	require.ErrorIs(t, h.Delete(ctx, region, tableArn), native.ErrNotFound)
	_, err = h.Read(ctx, region, tableBucket+"/table/"+missingPrefix)
	require.ErrorIs(t, err, native.ErrNotFound, "a table AWS doesn't have")

	err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: tableArn, Desired: replication(tableArn, destination1)})
	require.ErrorIs(t, err, native.ErrNotFound, "an update of a configuration deleted since the read")
}

func Test_Read_WithoutConfiguration(t *testing.T) {
	props, err := NewTableReplication(emptyAPI{}).Read(t.Context(), region, tableArn)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"TableArn": tableArn}, props)
}

// emptyAPI returns no configuration.
type emptyAPI struct {
	API
}

func (emptyAPI) GetTableReplication(ctx context.Context, in *s3tables.GetTableReplicationInput, _ ...func(*s3tables.Options)) (*s3tables.GetTableReplicationOutput, error) {
	return &s3tables.GetTableReplicationOutput{}, nil
}

func Test_Delete_ConfigurationDeletedAfterTheRead(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTableReplication(api)
	ctx := t.Context()
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: replication(tableArn, destination1)})
	require.NoError(t, err)
	api.errs["DeleteTableReplication"] = fault(notFoundCode)
	require.ErrorIs(t, h.Delete(ctx, region, tableArn), native.ErrNotFound)
}

func Test_ListAndProgress(t *testing.T) {
	h := NewTableReplication(newFakeAPI(awsfake.NewCloudControl()))
	_, err := h.List(t.Context(), region)
	require.ErrorContains(t, err, "can't be listed")
	progress, err := h.Progress(t.Context(), region, tableArn, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_InRegion(t *testing.T) {
	opts := s3tables.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}
