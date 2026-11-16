/*
Copyright 2026 The Radius Authors.

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

package v1alpha3

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeploymentCoordinatorStatusJSON(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"Idle", "PutDispatching", "PutRunning", "DeleteDispatching", "DeleteRunning", "RetryWaiting", "Blocked"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			status := DeploymentTemplateStatus{
				Coordinator: &DeploymentCoordinatorStatus{
					Phase: phase,
					Outputs: []DeploymentOutputStatus{
						{ID: "/outputs/current", State: "Current"},
						{ID: "/outputs/old", State: "PendingDelete"},
					},
					ResourceID:    "/outputs/old",
					SubmittedHash: "template-hash",
					Message:       "request outcome is unknown",
					RetryAfter:    &metav1.Time{Time: time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC)},
				},
				Operation: &ResourceOperation{ResumeToken: "resume-token"},
			}
			data, err := json.Marshal(status)
			require.NoError(t, err)
			var roundTrip DeploymentTemplateStatus
			require.NoError(t, json.Unmarshal(data, &roundTrip))
			require.NotNil(t, roundTrip.Coordinator.RetryAfter)
			roundTrip.Coordinator.RetryAfter.Time = roundTrip.Coordinator.RetryAfter.UTC()
			require.Equal(t, status, roundTrip)
			require.JSONEq(t, `{
				"coordinator": {
					"phase": "`+phase+`",
					"outputs": [{"id":"/outputs/current","state":"Current"},{"id":"/outputs/old","state":"PendingDelete"}],
					"resourceID": "/outputs/old",
					"submittedHash": "template-hash",
					"message": "request outcome is unknown",
					"retryAfter": "2026-11-09T23:00:00Z"
				},
				"operation": {"resumeToken": "resume-token"}
			}`, string(data))
		})
	}
	data, err := json.Marshal(DeploymentTemplateStatus{})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(data))
}

func TestDeploymentTemplateDeepCopyCoordinator(t *testing.T) {
	t.Parallel()
	template := &DeploymentTemplate{
		Status: DeploymentTemplateStatus{
			Coordinator: &DeploymentCoordinatorStatus{
				Phase:      "Idle",
				Outputs:    []DeploymentOutputStatus{{ID: "/outputs/current", State: "Current"}},
				RetryAfter: &metav1.Time{Time: time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC)},
			},
		},
	}
	copied := template.DeepCopy()
	require.Equal(t, template, copied)
	copied.Status.Coordinator.Phase = "DeleteDispatching"
	copied.Status.Coordinator.Outputs[0].State = "PendingDelete"
	copied.Status.Coordinator.RetryAfter.Time = copied.Status.Coordinator.RetryAfter.Add(time.Hour)
	require.Equal(t, "Idle", template.Status.Coordinator.Phase)
	require.Equal(t, "Current", template.Status.Coordinator.Outputs[0].State)
	require.Equal(t, time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC), template.Status.Coordinator.RetryAfter.Time)
	require.Nil(t, (&DeploymentTemplate{}).DeepCopy().Status.Coordinator)
}
