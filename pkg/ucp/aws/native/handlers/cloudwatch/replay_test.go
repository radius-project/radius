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
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
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

// Test_Replay_MetricMathAnomalyDetector creates a metric math detector with the fewest fields a template sets, lists
// it, changes its configuration, and deletes it, against AWS's recorded responses. AWS returns the expression query
// with ReturnData true, which the template leaves out.
func Test_Replay_MetricMathAnomalyDetector(t *testing.T) {
	s := awsreplay.New(t, "testdata/metric-math-anomaly-detector.json")
	cfg := s.Config(t.Context())
	h := NewAnomalyDetector(cloudwatch.NewFromConfig(cfg), sts.NewFromConfig(cfg))
	run := s.Input("RADIUS_REC_RUN")

	properties := map[string]any{"MetricMathAnomalyDetector": map[string]any{"MetricDataQueries": []any{
		map[string]any{
			"Id": "m1",
			"MetricStat": map[string]any{
				"Metric": map[string]any{"Namespace": "Radius/Recording", "MetricName": "radius-rec-" + run, "Dimensions": dims("radius-recording", run)},
				"Period": 300,
				"Stat":   "Average",
			},
			"ReturnData": false,
		},
		map[string]any{"Id": "e1", "Expression": "m1 * 2"},
	}}}
	identifier := identifierOf(t, properties)
	nativetest.Lifecycle{
		Handler:     h,
		Session:     s,
		Properties:  properties,
		Want:        map[string]any{"Id": identifier},
		Update:      with(properties, map[string]any{"Configuration": config("UTC")}),
		WantUpdated: map[string]any{"Id": identifier, "Configuration": config("UTC")},
		Poll:        5 * time.Second,
		Between: func() {
			list, err := h.List(t.Context(), awsreplay.Region)
			require.NoError(t, err)
			identifiers := []string{}
			for _, r := range list {
				identifiers = append(identifiers, r.Identifier)
			}
			require.Contains(t, identifiers, identifier, "List gives the detector the template's identifier")
		},
	}.Run(t)
}

// Test_Replay_AnomalyDetectorConfigurationRemoved creates a detector with a configuration, deploys the template
// without it, and checks that AWS has no excluded time ranges or time zone left, against AWS's recorded responses.
func Test_Replay_AnomalyDetectorConfigurationRemoved(t *testing.T) {
	s := awsreplay.New(t, "testdata/anomaly-detector-configuration-removed.json")
	cfg := s.Config(t.Context())
	h := NewAnomalyDetector(cloudwatch.NewFromConfig(cfg), sts.NewFromConfig(cfg))
	run := s.Input("RADIUS_REC_RUN")

	properties := map[string]any{
		"SingleMetricAnomalyDetector": map[string]any{
			"Namespace":  "Radius/Recording",
			"MetricName": "radius-rec-" + run,
			"Stat":       "Average",
			"Dimensions": dims("radius-recording", run),
		},
		"Configuration": config("America/New_York"),
	}
	update := with(properties, map[string]any{"Configuration": nil})
	identifier := identifierOf(t, properties)
	if s.Recording() {
		t.Cleanup(func() {
			if !t.Failed() {
				return
			}
			// t.Context() ends before cleanups run.
			err := h.Delete(context.WithoutCancel(t.Context()), awsreplay.Region, identifier)
			if err != nil && !errors.Is(err, native.ErrNotFound) {
				t.Errorf("failed to delete the anomaly detector %s: %v", identifier, err)
			}
		})
	}
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: properties,
		Update:     update,
		Poll:       5 * time.Second,
		SkipDelete: true,
	}.Run(t)

	got, err := h.Read(t.Context(), awsreplay.Region, identifier)
	require.NoError(t, err)
	require.NotContains(t, got, "Configuration", "AWS has no excluded time ranges or time zone left")
	require.NoError(t, h.Delete(t.Context(), awsreplay.Region, identifier))
	_, err = h.Progress(t.Context(), awsreplay.Region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}
