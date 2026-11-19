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

// Test_Replay_DBInstanceState stops an available RDS for PostgreSQL DB instance and starts it again, against
// responses written by hand from RDS's API reference. A delete changes nothing in AWS, so there is no delete step. To
// record it, create a single-AZ DB instance without read replicas, wait until it is available, export its identifier,
// and delete it after recording.
func Test_Replay_DBInstanceState(t *testing.T) {
	s := awsreplay.New(t, "testdata/dbinstancestate.json")
	id := s.Input("RADIUS_REC_RDS_INSTANCE")
	nativetest.Lifecycle{
		Handler:     NewDBInstanceState(rds.NewFromConfig(s.Config(t.Context()))),
		Session:     s,
		Properties:  map[string]any{"DBInstanceIdentifier": id, "State": "stopped"},
		Want:        map[string]any{"DBInstanceIdentifier": id, "State": "stopped", "DBInstanceStatus": "stopped"},
		Update:      map[string]any{"DBInstanceIdentifier": id, "State": "running"},
		WantUpdated: map[string]any{"DBInstanceIdentifier": id, "State": "running", "DBInstanceStatus": "available"},
		Poll:        time.Minute,
		SkipDelete:  true,
	}.Run(t)
}
