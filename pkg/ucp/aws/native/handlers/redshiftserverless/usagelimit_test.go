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

package redshiftserverless

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// update reads the resource with the identifier and sends an update to desired, with the patch UCP computes from the
// template and the last deploy, lastApplied, under the schema in file.
func update(t *testing.T, h native.Handler, file string, identifier string, lastApplied map[string]any, desired map[string]any) error {
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	c, err := json.Marshal(current)
	require.NoError(t, err)
	d, err := json.Marshal(desired)
	require.NoError(t, err)
	removed := []string{}
	for k := range lastApplied {
		if _, ok := desired[k]; !ok {
			removed = append(removed, k)
		}
	}
	patch, err := awsoperations.GeneratePatchWithOptions(c, d, []byte(readFile(t, file)), awsoperations.PatchOptions{LastApplied: lastApplied, Removed: removed})
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch})
}

// requireFailure checks that err is a native.Failure with the outcome, and a client error and a conflict as wanted.
func requireFailure(t *testing.T, err error, outcome native.Outcome, conflict bool) string {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, outcome, failure.Outcome)
	require.Equal(t, conflict, failure.Conflict)
	return failure.Message
}

// patchOf returns a patch that sets the value at path.
func patchOf(t *testing.T, path string, value any) jsondiff.Patch {
	t.Helper()
	return jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: path, Value: value}}
}

const limitFile = "aws-redshiftserverless-usagelimit.json"

// createdLimit creates the usage limit properties give and waits until the create is done.
func createdLimit(t *testing.T, h *UsageLimit, properties map[string]any) string {
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	waitDone(t, h, id, native.OperationCreate)
	return id
}

// waitDone calls Progress until op is done, and fails if it is done the first time, because a change also leaves
// the workgroup busy.
func waitDone(t *testing.T, h native.Handler, id string, op native.Operation) {
	t.Helper()
	for i := range 10 {
		progress, err := h.Progress(t.Context(), region, id, op)
		require.NoError(t, err)
		if progress.Done {
			require.NotZero(t, i, "AWS is still changing the workgroup when the code first looks")
			require.Nil(t, progress.Failure)
			return
		}
		require.Equal(t, "workgroup MODIFYING", progress.State)
		require.Equal(t, "workgroup AVAILABLE", progress.Waiting)
	}
	t.Fatal("the operation never finished")
}

func Test_UsageLimit_Update(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(api)
	id := createdLimit(t, h, limit)

	require.NoError(t, update(t, h, limitFile, id, limit, limit))
	require.Zero(t, api.calls["UpdateUsageLimit"], "an unchanged template sends nothing")

	require.NoError(t, update(t, h, limitFile, id, limit, with(limit, map[string]any{"BreachAction": nil, "Amount": float64(30)})))
	stored := api.store.Resource(usageLimitType, id)
	require.Equal(t, "log", stored["BreachAction"], "a removed breach action goes back to AWS's default")
	require.EqualValues(t, 30, stored["Amount"])
	waitDone(t, h, id, native.OperationUpdate)

	require.NoError(t, update(t, h, limitFile, id, limit, with(limit, map[string]any{"BreachAction": "deactivate"})))
	require.Equal(t, "deactivate", api.store.Resource(usageLimitType, id)["BreachAction"])
	require.EqualValues(t, 10, api.store.Resource(usageLimitType, id)["Amount"])

	err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(limit, map[string]any{"Amount": "lots"})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
}

func Test_UsageLimit_Errors(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(api)
	id := createdLimit(t, h, limit)

	_, err := h.Read(t.Context(), region, "missing")
	require.ErrorIs(t, err, native.ErrNotFound, "GetUsageLimit answers a missing ID with ValidationException")
	require.ErrorIs(t, h.Delete(t.Context(), region, "missing"), native.ErrNotFound)
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "missing", Desired: limit, Patch: patchOf(t, "/Amount", 5)})
	require.ErrorIs(t, err, native.ErrNotFound)

	api.errs["UpdateUsageLimit"] = fault(conflict, "busy")
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: limit, Patch: patchOf(t, "/Amount", 5)})
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, true), "dependsOn")
	delete(api.errs, "UpdateUsageLimit")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(limit, map[string]any{"ResourceArn": namespaceArn})})
	require.ErrorContains(t, err, validation, "AWS's error is returned as it is; the real SDK's is a 400, which awsapi.Rejected reports as rejected")

	for _, bad := range []map[string]any{with(limit, map[string]any{"Amount": nil}), with(limit, map[string]any{"Period": "yearly"})} {
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: bad})
		_ = requireFailure(t, err, native.OutcomeRejected, false)
	}
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: with(limit, map[string]any{"Amount": nil}), Patch: patchOf(t, "/Amount", nil)})
	require.Contains(t, requireFailure(t, err, native.OutcomeRejected, false), "Amount", "a limit always has an amount")

	api.errs["GetUsageLimit"] = fault("ThrottlingException", "slow down")
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
	delete(api.errs, "GetUsageLimit")
	api.errs["ListWorkgroups"] = fault("ThrottlingException", "slow down")
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
}

// noID is an API whose CreateUsageLimit makes the limit and returns no ID.
type noID struct {
	*fakeAPI
}

func (n noID) CreateUsageLimit(ctx context.Context, in *redshiftserverless.CreateUsageLimitInput, opts ...func(*redshiftserverless.Options)) (*redshiftserverless.CreateUsageLimitOutput, error) {
	_, err := n.fakeAPI.CreateUsageLimit(ctx, in, opts...)
	return &redshiftserverless.CreateUsageLimitOutput{}, err
}

func Test_UsageLimit_CreateWithoutID(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(noID{api})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: limit})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "AWS made the limit, so the next deploy finds it")
	id, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: limit})
	require.NoError(t, err)
	require.NotEmpty(t, id)
}

// Test_UsageLimit_Find checks that Find matches every value, with AWS's defaults for the ones the create leaves out.
func Test_UsageLimit_Find(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(api)
	minimal := with(limit, map[string]any{"Period": nil, "BreachAction": nil})
	id := createdLimit(t, h, minimal)
	createdLimit(t, h, with(limit, map[string]any{"Amount": float64(11)}))
	createdLimit(t, h, with(minimal, map[string]any{"ResourceArn": workgroupArn("reports")}))
	createdLimit(t, h, with(minimal, map[string]any{"UsageType": "cross-region-datasharing"}))

	found, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: minimal})
	require.NoError(t, err)
	require.Equal(t, id, found)
	found, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(limit, map[string]any{"Period": "monthly", "BreachAction": "log"})})
	require.NoError(t, err)
	require.Equal(t, id, found, "the defaults AWS filled in match")

	for _, other := range []map[string]any{
		with(minimal, map[string]any{"Period": "weekly"}),
		with(minimal, map[string]any{"BreachAction": "deactivate"}),
		with(minimal, map[string]any{"Amount": float64(12)}),
		with(minimal, map[string]any{"ResourceArn": "arn:aws:redshift-serverless:us-west-2:123456789012:workgroup/gone"}),
	} {
		found, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: other})
		require.NoError(t, err)
		require.Empty(t, found, "%v", other)
	}

	second := createdLimit(t, h, minimal)
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: minimal})
	require.ErrorContains(t, err, "2 usage limits")
	heldBy := func(held string) func(context.Context, string) (bool, error) {
		return func(_ context.Context, identifier string) (bool, error) { return identifier == held, nil }
	}
	found, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: minimal, Held: heldBy(id)})
	require.NoError(t, err)
	require.Equal(t, second, found, "a limit another alias holds isn't the lost create's")
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: minimal, Held: func(context.Context, string) (bool, error) {
		return false, errors.New("the store failed")
	}})
	require.ErrorContains(t, err, "the store failed")

	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: with(minimal, map[string]any{"Amount": "ten"})})
	_ = requireFailure(t, err, native.OutcomeRejected, false)
	api.errs["ListUsageLimits"] = fault("ThrottlingException", "slow down")
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: minimal})
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_UsageLimit_DeleteProgress(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(api)
	id := createdLimit(t, h, limit)
	require.NoError(t, h.Delete(t.Context(), region, id))
	progress, err := h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "deleting", Waiting: "deleted"}, progress, "AWS may still return a deleted limit")
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

// Test_UsageLimit_WorkgroupGone checks that a create whose workgroup AWS no longer lists has nothing to wait for.
func Test_UsageLimit_WorkgroupGone(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(api)
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: limit})
	require.NoError(t, err)
	api.store.Remove(workgroupType, "analytics")
	progress, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_UsageLimit_List(t *testing.T) {
	api := newAPI()
	putLimits(api.store, 150)
	resources, err := NewUsageLimit(api).List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, 150)
	require.Equal(t, workgroupArn("analytics"), resources[0].Properties["ResourceArn"])

	api.errs["ListUsageLimits"] = fault("ThrottlingException", "slow down")
	_, err = NewUsageLimit(api).List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_amount(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  int64
		ok    bool
	}{
		{float64(10), 10, true},
		{10, 10, true},
		{int64(10), 10, true},
		{float64(maxAmount), maxAmount, true},
		{float64(0), 0, false},
		{-1, 0, false},
		{1.5, 0, false},
		{float64(maxAmount * 2), 0, false},
		{"10", 0, false},
		{nil, 0, false},
	} {
		got, ok := amount(tt.value)
		require.Equal(t, tt.ok, ok, "%v", tt.value)
		if ok {
			require.Equal(t, tt.want, got)
		}
	}
	require.Equal(t, "a, b, or c", joinValues([]string{"a", "b", "c"}))
	require.Equal(t, []string{"log", "emit-metric", "deactivate"}, enumValues(types.UsageLimitBreachAction("").Values()))
}

// Test_UsageLimit_UpdateAWSAlreadyHas checks that a change AWS already has isn't sent, because every change leaves the
// workgroup busy: a removed breach action that is already log, AWS's default.
func Test_UsageLimit_UpdateAWSAlreadyHas(t *testing.T) {
	api := newAPI()
	h := NewUsageLimit(api)
	explicit := with(limit, map[string]any{"BreachAction": "log"})
	id := createdLimit(t, h, explicit)
	require.NoError(t, update(t, h, limitFile, id, explicit, with(limit, map[string]any{"BreachAction": nil})))
	require.Zero(t, api.calls["UpdateUsageLimit"])
}

// countingTransport answers every request with 500, as AWS does when it may have applied a call but its answer is
// lost, and counts the requests.
type countingTransport struct {
	mu    sync.Mutex
	count int
}

func (c *countingTransport) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	body := `{"__type":"InternalServerException","message":"lost"}`
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// Test_UsageLimit_CreateSentOnce checks that the SDK doesn't send a create again after an answer it would retry,
// because AWS would make a second limit no record tracks, and that UCP keeps the outcome unknown so the next deploy
// finds the limit.
func Test_UsageLimit_CreateSentOnce(t *testing.T) {
	transport := &countingTransport{}
	client := redshiftserverless.New(redshiftserverless.Options{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider("id", "secret", ""),
		HTTPClient:  transport,
	})
	_, err := NewUsageLimit(client).Create(t.Context(), native.CreateRequest{Region: region, Properties: limit})
	require.Error(t, err)
	var failure *native.Failure
	require.False(t, errors.As(err, &failure) && failure.Outcome == native.OutcomeRejected, "the create may have reached AWS")
	require.Equal(t, 1, transport.count)

	_, err = client.GetUsageLimit(t.Context(), &redshiftserverless.GetUsageLimitInput{UsageLimitId: aws.String("x")})
	require.Error(t, err)
	require.Greater(t, transport.count, 2, "other calls keep the SDK's retries")
}
