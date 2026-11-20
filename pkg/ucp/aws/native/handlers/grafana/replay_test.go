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

package grafana

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_WorkspaceServiceAccount makes a service account in a workspace and deletes it, against AWS's responses
// as its API reference documents them. Every property is create-only, so there is no update step.
func Test_Replay_WorkspaceServiceAccount(t *testing.T) {
	s := awsreplay.New(t, "testdata/workspaceserviceaccount.json")
	properties := map[string]any{
		"WorkspaceId": s.Input("RADIUS_REC_GRAFANA_WORKSPACE_ID"),
		"Name":        "radius-rec-viewer",
		"GrafanaRole": "VIEWER",
	}
	nativetest.Lifecycle{
		Handler:    NewWorkspaceServiceAccount(grafana.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Want:       with(properties, map[string]any{"IsDisabled": "false"}),
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_WorkspaceServiceAccountErrors runs AWS's errors through the SDK: a create AWS answers with 409
// ConflictException while the workspace is changing must be a conflict, a create in a workspace that doesn't exist,
// which AWS answers with 404, and one AWS answers with 400 ValidationException must be rejected, so UCP answers 400,
// and a list of a workspace that doesn't exist reads as not found.
func Test_Replay_WorkspaceServiceAccountErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/workspaceserviceaccounterrors.json")
	h := NewWorkspaceServiceAccount(grafana.NewFromConfig(s.Config(t.Context())))
	properties := map[string]any{"WorkspaceId": s.Input("RADIUS_REC_GRAFANA_WORKSPACE_ID"), "Name": "radius-rec-viewer", "GrafanaRole": "VIEWER"}

	_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: properties})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)

	for _, want := range []string{"doesn't exist", "ValidationException"} {
		_, err = h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: properties})
		require.ErrorAs(t, err, &failure, want)
		require.False(t, failure.Conflict, want)
		require.True(t, failure.ClientError, want)
		require.Equal(t, native.OutcomeRejected, failure.Outcome, want)
		require.Contains(t, failure.Message, want)
		require.NotErrorIs(t, err, native.ErrNotFound, want)
	}

	_, err = h.Read(t.Context(), awsreplay.Region, properties["WorkspaceId"].(string)+"|2")
	require.ErrorIs(t, err, native.ErrNotFound)
}
