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
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_DBInstanceState stops an available RDS for PostgreSQL DB instance and starts it again, as UCP does once
// it took the DB instance over, against responses written by hand from RDS's API reference. A delete changes nothing
// in AWS, so there is no delete step. To record it, create a single-AZ DB instance without read replicas, wait until
// it is available, export its identifier, and delete it after recording.
func Test_Replay_DBInstanceState(t *testing.T) {
	s := awsreplay.New(t, "testdata/dbinstancestate.json")
	id := s.Input("RADIUS_REC_RDS_INSTANCE")
	h := NewDBInstanceState(rds.NewFromConfig(s.Config(t.Context())))
	for _, step := range []struct{ state, status string }{{"stopped", "stopped"}, {"running", "available"}} {
		state := step.state
		properties := map[string]any{"DBInstanceIdentifier": id, "State": state}
		before, err := h.UpdateState(t.Context(), awsreplay.Region, id)
		require.NoError(t, err)
		require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: awsreplay.Region, Identifier: id, Desired: properties, State: before}))
		for polls := 0; ; polls++ {
			require.Less(t, polls, 240, "the DB instance didn't get to %s", state)
			progress, err := h.UpdateProgress(t.Context(), awsreplay.Region, id, before)
			require.NoError(t, err)
			require.Nil(t, progress.Failure)
			if progress.Done {
				break
			}
			s.Wait(30 * time.Second)
		}
		got, err := h.Read(t.Context(), awsreplay.Region, id)
		require.NoError(t, err)
		nativetest.RequireSubset(t, map[string]any{"DBInstanceIdentifier": id, "State": state, "DBInstanceStatus": step.status}, got)
	}
}
