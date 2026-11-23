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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3tables"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_TableReplication replicates a table to one table bucket, adds a second, and deletes the configuration,
// against AWS's responses.
func Test_Replay_TableReplication(t *testing.T) {
	s := awsreplay.New(t, "testdata/tablereplication.json")
	table := s.Input("RADIUS_REC_S3TABLES_TABLE_ARN")
	replicationRole := s.Input("RADIUS_REC_S3TABLES_REPLICATION_ROLE_ARN")
	first, second := s.Input("RADIUS_REC_S3TABLES_DESTINATION_1"), s.Input("RADIUS_REC_S3TABLES_DESTINATION_2")
	properties := func(destinations ...string) map[string]any {
		p := replication(table, destinations...)
		p["Configuration"].(map[string]any)["Role"] = replicationRole
		return p
	}
	nativetest.Lifecycle{
		Handler:    NewTableReplication(s3tables.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties(first),
		Update:     properties(first, second),
		Poll:       time.Second,
	}.Run(t)
}
