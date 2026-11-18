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

package rds

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_ActivityStream starts the activity stream of an Aurora PostgreSQL cluster and stops it, against AWS's
// responses. Every property is create-only, so there is no update step. To record it, create the cluster with a
// supported instance class and a customer managed KMS key, and export their ARN and key ID.
func Test_Replay_ActivityStream(t *testing.T) {
	s := awsreplay.New(t, "testdata/activitystream.json")
	properties := map[string]any{
		"ResourceArn": s.Input("RADIUS_REC_AURORA_CLUSTER_ARN"),
		"Mode":        "async",
		"KmsKeyId":    s.Input("RADIUS_REC_KMS_KEY_ID"),
	}
	nativetest.Lifecycle{
		Handler:    NewActivityStream(rds.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Poll:       time.Minute,
	}.Run(t)
}

// Test_Replay_TenantDatabase creates a tenant database in an RDS for Oracle CDB instance in the multi-tenant
// configuration, changes its password and tags, and deletes it, against AWS's responses. Deleting it takes a final
// snapshot named with the time of the delete, which Radius never deletes, so delete it by hand after recording. To
// record it, create the instance and export its identifier.
func Test_Replay_TenantDatabase(t *testing.T) {
	s := awsreplay.New(t, "testdata/tenantdatabase.json")
	run := s.Input("RADIUS_REC_RUN")
	password, newPassword := "Rec-"+run+"-Pw1", "Rec-"+run+"-Pw2"
	s.Scrub(password, newPassword)

	h := NewTenantDatabase(rds.NewFromConfig(s.Config(t.Context())))
	if !s.Recording() {
		// The time the cassette was written. A recording keeps the real clock, so each run's final snapshot gets
		// its own name.
		h.now = func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }
	}
	properties := map[string]any{
		"DBInstanceIdentifier": s.Input("RADIUS_REC_ORACLE_CDB_INSTANCE"),
		"TenantDBName":         "pdbrec",
		"MasterUsername":       "admin",
		"MasterUserPassword":   password,
		"Tags":                 tags("radius-recording", run),
	}
	nativetest.Lifecycle{
		Handler:     h,
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"MasterUserPassword": nil, "ManageMasterUserPassword": false, "CharacterSetName": "AL32UTF8"}),
		Update:      with(properties, map[string]any{"MasterUserPassword": newPassword, "Tags": tags("radius-recording", run, "team", "a")}),
		WantUpdated: with(properties, map[string]any{"MasterUserPassword": nil, "Tags": tags("radius-recording", run, "team", "a")}),
		Poll:        time.Minute,
	}.Run(t)
}
