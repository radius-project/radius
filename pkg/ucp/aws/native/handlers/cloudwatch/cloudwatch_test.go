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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
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

// newHandler returns the handler with api for both CloudWatch and STS.
func newHandler(api *fakeAPI) *AnomalyDetector {
	return NewAnomalyDetector(api, api)
}

// identifierOf returns the identifier of properties, failing the test on an error.
func identifierOf(t *testing.T, properties map[string]any) string {
	id, err := NewAnomalyDetector(nil, nil).Identifier(properties)
	require.NoError(t, err)
	return id
}

func Test_Suite_AnomalyDetector(t *testing.T) {
	anomalyDetectorSuite(t, single("Average", dims("InstanceId", "i-1", "AutoScalingGroupName", "asg")))
}

// Test_Suite_AnomalyDetector_CurrentAccount runs the suite for a template that names the current account as the
// detector's AccountId, which the identifier leaves out.
func Test_Suite_AnomalyDetector_CurrentAccount(t *testing.T) {
	anomalyDetectorSuite(t, with(single("Average", dims("InstanceId", "i-1", "AutoScalingGroupName", "asg")), map[string]any{"AccountId": fakeAccount}))
}

func anomalyDetectorSuite(t *testing.T, metric map[string]any) {
	withStat := func(stat string) map[string]any { return with(metric, map[string]any{"Stat": stat}) }
	detector := map[string]any{
		"SingleMetricAnomalyDetector": metric,
		"Configuration":               config("UTC"),
		"MetricCharacteristics":       map[string]any{"PeriodicSpikes": true},
	}
	identifier := identifierOf(t, with(detector, map[string]any{"SingleMetricAnomalyDetector": with(metric, map[string]any{"AccountId": nil})}))
	reordered := with(metric, map[string]any{"Dimensions": dims("AutoScalingGroupName", "asg", "InstanceId", "i-1")})
	other := with(metric, map[string]any{"AccountId": fakeAccount})
	if _, ok := metric["AccountId"]; ok {
		other = with(metric, map[string]any{"AccountId": nil})
	}
	nativetest.Run(t, nativetest.Suite{
		TypeName: anomalyDetectorType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return newHandler(newFakeAPI(store))
		},
		IdentifierProperty: "Id",
		Properties:         detector,
		Identifier:         identifier,
		Changed:            with(detector, map[string]any{"Configuration": config("America/New_York")}),
		Removable:          "Configuration",
		CreateOnlyChange:   with(detector, map[string]any{"SingleMetricAnomalyDetector": withStat("p90")}),
		OtherForm:          map[string]any{"SingleMetricAnomalyDetector": reordered},
		// A detector without AccountId is in the current account, so a template that names that account, or leaves it
		// out, is the same detector.
		SameResource: []map[string]any{with(detector, map[string]any{"SingleMetricAnomalyDetector": other})},
		// The identifier with its keys and dimensions in another order, and with the current account.
		SameResourceNames: []string{otherOrder(t, identifier, ""), otherOrder(t, identifier, fakeAccount)},
		// The suite's Cloud Control names a detector of the current account as AWS keeps it, without the account.
		CloudControlCreate: func(properties map[string]any) (string, map[string]any) {
			resolved, err := newHandler(newFakeAPI(awsfake.NewCloudControl())).ResolveIdentifier(t.Context(), region, identifierOf(t, properties))
			require.NoError(t, err)
			return resolved, properties
		},
	})
}

// otherOrder returns identifier, the identifier of a detector of one metric, with Stat first, its dimensions in reverse
// order, and account as AccountId when set.
func otherOrder(t *testing.T, identifier string, account string) string {
	id, err := parseIdentifier(identifier)
	require.NoError(t, err)
	dimensions := slices.Clone(id.Dimensions)
	slices.Reverse(dimensions)
	b, err := json.Marshal(dimensions)
	require.NoError(t, err)
	out := fmt.Sprintf(`{"Stat":%q,"Dimensions":%s,"MetricName":%q,"Namespace":%q`, id.Stat, b, id.MetricName, id.Namespace)
	if account != "" {
		out += fmt.Sprintf(`,"AccountId":%q`, account)
	}
	return out + "}"
}

func Test_Identifier(t *testing.T) {
	legacy := map[string]any{"Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average", "Dimensions": dims("b", "2", "a", "1")}
	modern := map[string]any{"SingleMetricAnomalyDetector": single("Average", dims("a", "1", "b", "2"))}
	require.Equal(t, identifierOf(t, legacy), identifierOf(t, modern), "both forms and any dimension order give one identifier")
	require.JSONEq(t, `{"Namespace":"AWS/EC2","MetricName":"CPUUtilization","Dimensions":[{"Name":"a","Value":"1"},{"Name":"b","Value":"2"}],"Stat":"Average"}`, identifierOf(t, legacy))

	math := map[string]any{"MetricMathAnomalyDetector": mathDetector("m1")}
	require.JSONEq(t, `{"MetricDataQueries":[{"Id":"m1","MetricStat":{"Metric":{"Namespace":"AWS/EC2","MetricName":"CPUUtilization","Dimensions":[{"Name":"a","Value":"1"},{"Name":"b","Value":"2"}]},"Period":300,"Stat":"Average"},"ReturnData":true}]}`, identifierOf(t, math))

	require.Empty(t, identifierOf(t, map[string]any{"Configuration": config("UTC")}), "no metric, no identifier")

	_, err := NewAnomalyDetector(nil, nil).Identifier(map[string]any{"Stat": 5})
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

// Test_MetricMath_ReturnDataDefault creates a metric math detector whose expression leaves out ReturnData, as the
// fewest fields a template sets. AWS returns the expression with ReturnData true, and the detector is still the
// template's.
func Test_MetricMath_ReturnDataDefault(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := newHandler(api)
	metric := mathDetector("m1")["MetricDataQueries"].([]any)[0].(map[string]any)
	properties := map[string]any{"MetricMathAnomalyDetector": map[string]any{"MetricDataQueries": []any{
		with(metric, map[string]any{"ReturnData": false}),
		map[string]any{"Id": "e1", "Expression": "m1 * 2"},
	}}}
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, identifierOf(t, properties), identifier)

	got, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err, "the detector AWS returns with ReturnData true is the template's")
	list, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, identifier, list[0].Identifier)
	require.Equal(t, got, list[0].Properties)
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Desired: with(properties, map[string]any{"Configuration": config("UTC")})}))
	require.NoError(t, h.Delete(t.Context(), region, identifier))
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_LegacyForm(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := newHandler(api)
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
	h := newHandler(api)
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
	h := newHandler(api)
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
	asked := 0
	account := func() (string, error) {
		asked++
		return "123456789012", nil
	}
	same := func(a, b identity) bool {
		ok, err := a.same(b, account)
		require.NoError(t, err)
		return ok
	}
	require.True(t, same(id, withAccount), "AWS may add the current account's ID to a detector the template created without one")
	require.False(t, same(id, otherAccount), "a detector of another account is another detector")
	require.False(t, same(withAccount, otherAccount))
	require.Equal(t, 2, asked)
	require.True(t, same(id, id))
	sum := id
	sum.Stat = "Sum"
	require.False(t, same(id, sum))
	require.Equal(t, 2, asked, "the account is asked for only when the rest of the identities match")
	failure := errors.New("denied")
	_, err := id.same(withAccount, func() (string, error) { return "", failure })
	require.ErrorIs(t, err, failure)

	api := newFakeAPI(awsfake.NewCloudControl())
	h := newHandler(api)
	api.errs["GetCallerIdentity"] = fault("AccessDenied")
	_, err = h.Read(t.Context(), region, identifierOf(t, map[string]any{"SingleMetricAnomalyDetector": single("Average", nil)}))
	require.ErrorIs(t, err, native.ErrNotFound, "a read that finds no detector needs no account")
	explicit := map[string]any{"SingleMetricAnomalyDetector": map[string]any{"AccountId": "123456789012", "Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average"}}
	implicit := identifierOf(t, map[string]any{"SingleMetricAnomalyDetector": map[string]any{"Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average"}})
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: explicit})
	require.ErrorContains(t, err, "AccessDenied", "the identifier of a template with AccountId needs the current account")
	_, err = h.ResolveIdentifier(t.Context(), region, identifierOf(t, explicit))
	require.ErrorContains(t, err, "AccessDenied")
	resolved, err := h.ResolveIdentifier(t.Context(), region, implicit)
	require.NoError(t, err, "an identifier without AccountId needs no STS call to resolve")
	require.Equal(t, implicit, resolved)
	delete(api.errs, "GetCallerIdentity")

	// The current account's detector has one identifier, whether the template names the account or not.
	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: explicit})
	require.NoError(t, err)
	require.Equal(t, implicit, identifier)
	resolved, err = h.ResolveIdentifier(t.Context(), region, identifierOf(t, explicit))
	require.NoError(t, err)
	require.Equal(t, implicit, resolved)
	source := identifierOf(t, map[string]any{"SingleMetricAnomalyDetector": map[string]any{"AccountId": sourceAccount, "Namespace": "AWS/EC2", "MetricName": "CPUUtilization", "Stat": "Average"}})
	resolved, err = h.ResolveIdentifier(t.Context(), region, source)
	require.NoError(t, err)
	require.Equal(t, source, resolved, "another account's detector keeps its AccountId")
	_, err = h.ResolveIdentifier(t.Context(), region, "not-json")
	require.ErrorContains(t, err, "is not the identifier of a CloudWatch anomaly detector")

	api.errs["GetCallerIdentity"] = fault("AccessDenied")
	_, err = h.Read(t.Context(), region, implicit)
	require.ErrorContains(t, err, "AccessDenied", "an identifier without AccountId needs the current account to match AWS's")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "AccessDenied")
	delete(api.errs, "GetCallerIdentity")
	require.NoError(t, h.Delete(t.Context(), region, identifier))
	require.Empty(t, api.store.List(anomalyDetectorType))
}

// Test_SourceAccountDetector deploys a detector next to one of the same metric in a source account, as a monitoring
// account sees them. A template without AccountId means the current account, so it reads, updates, and deletes only
// that detector, whether AWS returns its account ID or not, and the delete finishes while the source detector stays.
func Test_SourceAccountDetector(t *testing.T) {
	for _, omit := range []bool{false, true} {
		t.Run(fmt.Sprintf("AWS returns the account ID %v", !omit), func(t *testing.T) {
			ctx := t.Context()
			api := newFakeAPI(awsfake.NewCloudControl())
			api.omitAccount = omit
			h := newHandler(api)
			zone := func(identifier string) any {
				got, err := h.Read(ctx, region, identifier)
				require.NoError(t, err)
				return got["Configuration"].(map[string]any)["MetricTimeZone"]
			}
			metric := single("Average", nil)
			source, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{
				"SingleMetricAnomalyDetector": with(metric, map[string]any{"AccountId": sourceAccount}), "Configuration": config("Europe/London"),
			}})
			require.NoError(t, err)
			local, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"SingleMetricAnomalyDetector": metric, "Configuration": config("UTC")}})
			require.NoError(t, err)
			require.Equal(t, "UTC", zone(local))

			list, err := h.List(ctx, region)
			require.NoError(t, err)
			identifiers := []string{}
			for _, r := range list {
				identifiers = append(identifiers, r.Identifier)
			}
			require.ElementsMatch(t, []string{local, source}, identifiers, "a detector of the current account has the identifier a template without AccountId gives")

			desired := map[string]any{"SingleMetricAnomalyDetector": metric, "Configuration": config("Asia/Tokyo")}
			require.NoError(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: local, Desired: desired}))
			require.Equal(t, "Asia/Tokyo", zone(local))
			require.Equal(t, "Europe/London", zone(source))

			require.NoError(t, h.Delete(ctx, region, local))
			_, err = h.Progress(ctx, region, local, native.OperationDelete)
			require.ErrorIs(t, err, native.ErrNotFound, "the delete finishes while the source account's detector stays")
			_, err = h.Read(ctx, region, local)
			require.ErrorIs(t, err, native.ErrNotFound)
			require.Equal(t, "Europe/London", zone(source))
		})
	}
}

func Test_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	api.pageSize = 1
	h := newHandler(api)
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
	h := newHandler(api)
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

	h := newHandler(newFakeAPI(awsfake.NewCloudControl()))
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
	h = newHandler(api)
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
