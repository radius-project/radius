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

package sagemaker

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sagemaker"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_TrialComponent creates a trial component, updates its display name, status, end time, and tags, and
// deletes it, against AWS's recorded responses.
func Test_Replay_TrialComponent(t *testing.T) {
	s := awsreplay.New(t, "testdata/trialcomponent.json")
	api := sagemaker.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")

	properties := map[string]any{
		"TrialComponentName": "radius-rec-" + run + "-tc",
		"DisplayName":        "radius-recording",
		"StartTime":          "2026-01-02T03:04:05Z",
		"Status":             map[string]any{"PrimaryStatus": "InProgress"},
		"MetadataProperties": map[string]any{"ProjectId": "radius-rec"},
		"Tags":               tags("radius-recording", run),
	}
	updated := with(properties, map[string]any{
		"DisplayName": "radius-recording-updated",
		"EndTime":     "2026-01-02T04:04:05Z",
		"Status":      map[string]any{"PrimaryStatus": "Completed"},
		"Tags":        tags("radius-recording", run, "step", "update"),
	})
	nativetest.Lifecycle{
		Handler:    NewTrialComponent(api),
		Session:    s,
		Properties: properties,
		Update:     updated,
		Poll:       5 * time.Second,
	}.Run(t)
}
