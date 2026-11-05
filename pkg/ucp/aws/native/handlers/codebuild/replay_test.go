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

package codebuild

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/codebuild"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Project creates a project that runs an inline build spec with no source and no artifacts, updates it,
// and deletes it, against AWS's recorded responses. A project costs nothing until it runs a build.
func Test_Replay_Project(t *testing.T) {
	s := awsreplay.New(t, "testdata/project.json")
	api := codebuild.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")
	role := s.Input("RADIUS_REC_CODEBUILD_ROLE")

	properties := map[string]any{
		"Name":        "radius-rec-" + run,
		"Description": "Radius recording",
		"ServiceRole": role,
		"Source":      map[string]any{"Type": "NO_SOURCE", "BuildSpec": "version: 0.2\nphases:\n  build:\n    commands:\n      - echo radius\n"},
		"Artifacts":   map[string]any{"Type": "NO_ARTIFACTS"},
		"Environment": map[string]any{
			"Type":        "LINUX_CONTAINER",
			"ComputeType": "BUILD_GENERAL1_SMALL",
			"Image":       "aws/codebuild/amazonlinux-x86_64-standard:5.0",
		},
		"TimeoutInMinutes": 10,
		"Tags":             tags("radius-recording", run),
	}
	// AWS fills defaults into the nested objects, so only the top-level values are compared.
	want := func(p map[string]any) map[string]any {
		return map[string]any{
			"Name":             p["Name"],
			"Description":      p["Description"],
			"ServiceRole":      p["ServiceRole"],
			"TimeoutInMinutes": p["TimeoutInMinutes"],
			"Tags":             p["Tags"],
		}
	}
	updated := with(properties, map[string]any{"Description": "Radius recording, updated", "TimeoutInMinutes": 15})
	nativetest.Lifecycle{
		Handler:     NewProject(api),
		Session:     s,
		Properties:  properties,
		Want:        want(properties),
		Update:      updated,
		WantUpdated: want(updated),
		Poll:        2 * time.Second,
	}.Run(t)
}
