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
	"errors"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-cloudwatch-anomalydetector.json")
	require.NoError(t, err)
	return string(b)
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func dims(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Name": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

func single(stat string, dimensions []any) map[string]any {
	return map[string]any{"Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": stat, "Dimensions": dimensions}
}

func config(zone string) map[string]any {
	return map[string]any{
		"MetricTimeZone":     zone,
		"ExcludedTimeRanges": []any{map[string]any{"StartTime": "2026-01-01T00:00:00", "EndTime": "2026-01-02T00:00:00"}},
	}
}

// identifierOf returns the identifier of properties, failing the test on an error.
func identifierOf(t *testing.T, properties map[string]any) string {
	id, err := NewAnomalyDetector(nil).Identifier(properties)
	require.NoError(t, err)
	return id
}

func Test_Suite_AnomalyDetector(t *testing.T) {
	detector := map[string]any{
		"SingleMetricAnomalyDetector": single("Average", dims("InstanceId", "i-1", "AutoScalingGroupName", "asg")),
		"Configuration":               config("UTC"),
		"MetricCharacteristics":       map[string]any{"PeriodicSpikes": true},
	}
	identifier := identifierOf(t, detector)
	reordered := single("Average", dims("AutoScalingGroupName", "asg", "InstanceId", "i-1"))
	nativetest.Run(t, nativetest.Suite{
		TypeName: anomalyDetectorType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewAnomalyDetector(newFakeAPI(store))
		},
		IdentifierProperty: "Id",
		Properties:         detector,
		Identifier:         identifier,
		Changed:            with(detector, map[string]any{"Configuration": config("America/New_York")}),
		Removable:          "Configuration",
		CreateOnlyChange:   with(detector, map[string]any{"SingleMetricAnomalyDetector": single("p90", dims("InstanceId", "i-1", "AutoScalingGroupName", "asg"))}),
		OtherForm:          map[string]any{"SingleMetricAnomalyDetector": reordered},
	})
}

func Test_Identifier(t *testing.T) {
	legacy := map[string]any{"Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average", "Dimensions": dims("b", "2", "a", "1")}
	modern := map[string]any{"SingleMetricAnomalyDetector": single("Average", dims("a", "1", "b", "2"))}
	require.Equal(t, identifierOf(t, legacy), identifierOf(t, modern), "both forms and any dimension order give one identifier")
	require.JSONEq(t, `{"Namespace":"AWS/EC2","MetricName":"CPUUtilization","Dimensions":[{"Name":"a","Value":"1"},{"Name":"b","Value":"2"}],"Stat":"Average"}`, identifierOf(t, legacy))

	math := map[string]any{"MetricMathAnomalyDetector": mathDetector("m1")}
	require.JSONEq(t, `{"MetricDataQueries":[{"Id":"m1","MetricStat":{"Metric":{"Namespace":"AWS/EC2","MetricName":"CPUUtilization","Dimensions":[{"Name":"a","Value":"1"},{"Name":"b","Value":"2"}]},"Period":300,"Stat":"Average"},"ReturnData":true}]}`, identifierOf(t, math))

	require.Empty(t, identifierOf(t, map[string]any{"Configuration": config("UTC")}), "no metric, no identifier")

	_, err := NewAnomalyDetector(nil).Identifier(map[string]any{"Stat": 5})
	require.Error(t, err)
}

func mathDetector(id string) map[string]any {
	return map[string]any{"MetricDataQueries": []any{map[string]any{
		"Id":         id,
		"ReturnData": true,
		"MetricStat": map[string]any{
			"Period": 300,
			"Stat":   "Average",
			"Metric": map[string]any{"Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Dimensions": dims("b", "2", "a", "1")},
		},
	}}}
}

func Test_LegacyForm(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAnomalyDetector(api)
	legacy := map[string]any{"Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average", "Dimensions": dims("a", "1")}

	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: legacy})
	require.NoError(t, err)
	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	nativetest.RequireSubset(t, legacy, got)
	nativetest.RequireSubset(t, map[string]any{"Id": identifier, "SingleMetricAnomalyDetector": single("Average", dims("a", "1"))}, got)

	modern := map[string]any{"SingleMetricAnomalyDetector": single("Average", dims("a", "1"))}
	other, err := h.Identifier(modern)
	require.NoError(t, err)
	require.Equal(t, identifier, other, "the other form names the same detector")

	require.NoError(t, h.Delete(t.Context(), region, identifier))
	_, err = h.Read(t.Context(), region, identifier)
	require.ErrorIs(t, err, native.ErrNotFound)
	for _, call := range api.calls {
		require.True(t, strings.HasSuffix(call, " "+region), "every call is sent to the request's region: %s", call)
	}
}

func Test_MetricMath(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAnomalyDetector(api)
	properties := map[string]any{"MetricMathAnomalyDetector": mathDetector("m1"), "Configuration": config("UTC")}

	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"MetricMathAnomalyDetector": mathDetector("m2")}})
	require.NoError(t, err)

	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, identifier, got["Id"])
	require.Equal(t, "m1", got["MetricMathAnomalyDetector"].(map[string]any)["MetricDataQueries"].([]any)[0].(map[string]any)["Id"])
	require.Equal(t, config("UTC"), got["Configuration"])
	require.NotContains(t, got, "Namespace")

	progress, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	progress, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "present", Waiting: "deleted"}, progress)

	require.NoError(t, h.Delete(t.Context(), region, identifier))
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound)
}

func Test_ReadMatchesExactly(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAnomalyDetector(api)
	// A detector with more dimensions matches the dimension filter, but is another detector.
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"SingleMetricAnomalyDetector": single("Average", dims("a", "1", "b", "2"))}})
	require.NoError(t, err)

	_, err = h.Read(t.Context(), region, identifierOf(t, map[string]any{"SingleMetricAnomalyDetector": single("Average", dims("a", "1"))}))
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), region, identifierOf(t, map[string]any{"SingleMetricAnomalyDetector": single("Sum", dims("a", "1", "b", "2"))}))
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_AccountID(t *testing.T) {
	id := identity{Namespace: "AWS/EC2", MetricName: "CPUUtilization", Stat: "Average"}
	withAccount := id
	withAccount.AccountId = "123456789012"
	otherAccount := id
	otherAccount.AccountId = "210987654321"
	require.True(t, id.same(withAccount), "AWS may add the account of a detector the template created without one")
	require.False(t, withAccount.same(otherAccount))

	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAnomalyDetector(api)
	properties := map[string]any{"SingleMetricAnomalyDetector": map[string]any{"AccountId": "123456789012", "Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average"}}
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Contains(t, identifier, "123456789012")
	require.NoError(t, h.Delete(t.Context(), region, identifier))
	require.Empty(t, api.store.List(anomalyDetectorType))
}

func Test_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	api.pageSize = 1
	h := NewAnomalyDetector(api)
	for _, p := range []map[string]any{
		{"SingleMetricAnomalyDetector": single("Average", dims("a", "1"))},
		{"SingleMetricAnomalyDetector": single("Sum", nil)},
		{"MetricMathAnomalyDetector": mathDetector("m1")},
	} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: p})
		require.NoError(t, err)
	}
	list, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, list, 3)
	for _, r := range list {
		require.Equal(t, r.Identifier, r.Properties["Id"])
		got, err := h.Read(t.Context(), region, r.Identifier)
		require.NoError(t, err)
		require.Equal(t, r.Properties, got)
	}
}

func Test_Update(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewAnomalyDetector(api)
	properties := map[string]any{"SingleMetricAnomalyDetector": single("Average", nil), "Configuration": config("UTC"), "MetricCharacteristics": map[string]any{"PeriodicSpikes": true}}
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)

	desired := with(properties, map[string]any{"Configuration": nil})
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Desired: desired}))
	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.NotContains(t, got, "Configuration", "a configuration the template removed goes back to AWS's default")
	require.Equal(t, map[string]any{"PeriodicSpikes": true}, got["MetricCharacteristics"])
}

func Test_Errors(t *testing.T) {
	failure := &native.Failure{}

	h := NewAnomalyDetector(newFakeAPI(awsfake.NewCloudControl()))
	for _, identifier := range []string{"not-json", `{"Stat":"Average"}`} {
		_, err := h.Read(t.Context(), region, identifier)
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.ErrorAs(t, h.Delete(t.Context(), region, identifier), &failure)
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		require.ErrorAs(t, err, &failure)
	}

	badTime := map[string]any{
		"SingleMetricAnomalyDetector": single("Average", nil),
		"Configuration":               map[string]any{"ExcludedTimeRanges": []any{map[string]any{"StartTime": "yesterday", "EndTime": "2026-01-02T00:00:00"}}},
	}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: badTime})
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.NotContains(t, failure.Message, "yesterday", "the message names the property, never its value")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"Stat": 5}})
	require.Error(t, err)

	for _, both := range []map[string]any{
		{"SingleMetricAnomalyDetector": single("Average", nil), "Namespace": "AWS/EC2"},
		{"MetricMathAnomalyDetector": mathDetector("m1"), "Stat": "Average"},
		{"MetricMathAnomalyDetector": mathDetector("m1"), "SingleMetricAnomalyDetector": single("Average", nil)},
	} {
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: both})
		require.ErrorAs(t, err, &failure, "AWS takes one form of a detector")
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	}

	require.Error(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Desired: map[string]any{"Stat": 5}}))
	require.Error(t, h.Delete(t.Context(), region, `{"MetricDataQueries":[{"Id":"m1","Period":"often"}]}`))

	api := newFakeAPI(awsfake.NewCloudControl())
	h = NewAnomalyDetector(api)
	boom := errors.New("boom")
	api.errs["DescribeAnomalyDetectors"] = boom
	identifier := identifierOf(t, map[string]any{"SingleMetricAnomalyDetector": single("Average", nil)})
	_, err = h.Read(t.Context(), region, identifier)
	require.ErrorIs(t, err, boom)
	_, err = h.List(t.Context(), region)
	require.ErrorIs(t, err, boom)
	_, err = h.Progress(t.Context(), region, identifier, native.OperationUpdate)
	require.ErrorIs(t, err, boom)
}

func Test_Equivalent(t *testing.T) {
	ranges := func(start string, end string) []any {
		return []any{map[string]any{"StartTime": start, "EndTime": end}}
	}
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"Configuration/ExcludedTimeRanges", ranges("2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"), ranges("2026-01-01T00:00:00", "2026-01-02T00:00:00"), true},
		{"Configuration/ExcludedTimeRanges", ranges("2026-01-01T01:00:00+01:00", "2026-01-02T00:00:00"), ranges("2026-01-01T00:00:00", "2026-01-02T00:00:00"), true},
		{"Configuration/ExcludedTimeRanges", ranges("2026-01-01T00:00:01", "2026-01-02T00:00:00"), ranges("2026-01-01T00:00:00", "2026-01-02T00:00:00"), false},
		{"Configuration/ExcludedTimeRanges", ranges("soon", "2026-01-02T00:00:00"), ranges("2026-01-01T00:00:00", "2026-01-02T00:00:00"), false},
		{"Configuration/ExcludedTimeRanges", ranges("2026-01-01T00:00:00", "2026-01-02T00:00:00"), []any{}, false},
		{"Configuration/ExcludedTimeRanges", "x", "x", false},
		{"Configuration/ExcludedTimeRanges", []any{map[string]any{"StartTime": 1, "EndTime": 2}}, ranges("2026-01-01T00:00:00", "2026-01-02T00:00:00"), false},
		{"Configuration/MetricTimeZone", "UTC", "utc", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

func Test_ConfigurationProperties(t *testing.T) {
	require.Nil(t, configurationProperties(nil))
	got := configurationProperties(&types.AnomalyDetectorConfiguration{MetricTimezone: aws.String("UTC")})
	require.Equal(t, "UTC", got["MetricTimeZone"])
}
