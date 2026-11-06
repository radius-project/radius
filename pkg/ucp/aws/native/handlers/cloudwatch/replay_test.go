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

package cloudwatch

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_AnomalyDetector creates a detector for a custom metric, changes its configuration, and deletes it,
// against AWS's recorded responses. The metric needs no data points, and detectors aren't taggable.
func Test_Replay_AnomalyDetector(t *testing.T) {
	s := awsreplay.New(t, "testdata/anomaly-detector.json")
	cfg := s.Config(t.Context())
	api := cloudwatch.NewFromConfig(cfg)
	run := s.Input("RADIUS_REC_RUN")

	properties := map[string]any{
		"SingleMetricAnomalyDetector": map[string]any{
			"Namespace":  "Radius/Recording",
			"MetricName": "radius-rec-" + run,
			"Stat":       "Average",
			"Dimensions": dims("radius-recording", run),
		},
		"Configuration": config("UTC"),
	}
	nativetest.Lifecycle{
		Handler:    NewAnomalyDetector(api, sts.NewFromConfig(cfg)),
		Session:    s,
		Properties: properties,
		Update:     with(properties, map[string]any{"Configuration": config("America/New_York")}),
		Poll:       5 * time.Second,
	}.Run(t)
}
