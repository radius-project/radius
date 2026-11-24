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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_PartitionIndex creates a partition index on a table with the partition keys year and month, and deletes
// it, against AWS's responses. Every property is create-only, so there is no update step.
func Test_Replay_PartitionIndex(t *testing.T) {
	s := awsreplay.New(t, "testdata/partitionindex.json")
	api := glue.NewFromConfig(s.Config(t.Context()))

	properties := map[string]any{
		"CatalogId":      s.Input("RADIUS_REC_ACCOUNT_ID"),
		"DatabaseName":   s.Input("RADIUS_REC_GLUE_DATABASE"),
		"TableName":      s.Input("RADIUS_REC_GLUE_TABLE"),
		"PartitionIndex": map[string]any{"IndexName": "radius_rec_by_date", "Keys": []any{"year", "month"}},
	}
	nativetest.Lifecycle{
		Handler:    NewPartitionIndex(api),
		Session:    s,
		Properties: properties,
		Want:       with(properties, map[string]any{"IndexStatus": "ACTIVE"}),
		Poll:       10 * time.Second,
	}.Run(t)
}

// Test_Replay_TableBusy creates an index while Glue is deleting another index on the table, which it started after the
// handler read the table. Glue answers 400 with ResourceNumberLimitExceededException, and the handler must report a
// conflict, so UCP answers 409, not the limit of 3 indexes Glue uses the same code for.
func Test_Replay_TableBusy(t *testing.T) {
	s := awsreplay.New(t, "testdata/tablebusy.json")
	api := glue.NewFromConfig(s.Config(t.Context()))
	_, err := NewPartitionIndex(api).Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{
		"CatalogId":      s.Input("RADIUS_REC_ACCOUNT_ID"),
		"DatabaseName":   s.Input("RADIUS_REC_GLUE_DATABASE"),
		"TableName":      s.Input("RADIUS_REC_GLUE_TABLE"),
		"PartitionIndex": map[string]any{"IndexName": "radius_rec_by_date", "Keys": []any{"year", "month"}},
	}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
}
