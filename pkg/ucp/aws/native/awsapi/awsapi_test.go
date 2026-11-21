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

package awsapi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

func Test_Ambiguous(t *testing.T) {
	err := Ambiguous("network interface permissions", []string{"eni-perm-1", "eni-perm-2"})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "UCP doesn't know which one its create made")
	require.False(t, failure.ClientError)
	require.Contains(t, failure.Message, "2 network interface permissions")
	require.Contains(t, failure.Message, "(eni-perm-1, eni-perm-2)")
}

func Test_NotFound(t *testing.T) {
	missing := &smithy.GenericAPIError{Code: "DBClusterNotFoundFault", Message: "no such cluster"}
	require.True(t, HasCode(missing, "DBClusterNotFoundFault"))
	require.False(t, HasCode(errors.New("other"), "DBClusterNotFoundFault"))

	err := NotFound(missing, "DBClusterNotFoundFault")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorContains(t, err, "no such cluster")

	other := &smithy.GenericAPIError{Code: "AccessDenied"}
	require.Same(t, other, NotFound(other, "DBClusterNotFoundFault"))
	require.NoError(t, NotFound(nil, "DBClusterNotFoundFault"))
}

type sdkShape struct {
	Name     *string
	Port     *int32
	Enabled  *bool
	Empty    *string
	Tags     []sdkTag
	Settings *sdkSettings
	Created  *time.Time
	Unset    []string
}

type sdkTag struct {
	Key   *string
	Value *string
}

type sdkSettings struct {
	Mode *string
}

func Test_DecodeEncode(t *testing.T) {
	properties := map[string]any{
		"name":     "app",
		"Port":     27017,
		"Enabled":  false,
		"Tags":     []any{map[string]any{"Key": "team", "Value": "a"}},
		"Settings": map[string]any{"Mode": "fast"},
		"Ignored":  "not a field",
	}
	var out sdkShape
	require.NoError(t, Decode(properties, &out))
	require.Equal(t, "app", aws.ToString(out.Name), "names match ignoring case")
	require.EqualValues(t, 27017, aws.ToInt32(out.Port))
	require.Equal(t, "fast", aws.ToString(out.Settings.Mode))

	out.Empty = aws.String("")
	encoded, err := EncodeObject(out)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"Name":     "app",
		"Port":     float64(27017),
		"Enabled":  false,
		"Tags":     []any{map[string]any{"Key": "team", "Value": "a"}},
		"Settings": map[string]any{"Mode": "fast"},
	}, encoded, "unset and empty values are left out")

	empty, err := EncodeObject(sdkShape{})
	require.NoError(t, err)
	require.Empty(t, empty)

	list, err := Encode([]sdkSettings{{}, {Mode: aws.String("slow")}})
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{"Mode": "slow"}}, list)

	require.ErrorContains(t, Decode(map[string]any{"Port": "not a number"}, &out), "failed to read the properties")
	require.Error(t, Decode(map[string]any{"bad": func() {}}, &out))
	_, err = Encode(func() {})
	require.Error(t, err)
	notObject, err := EncodeObject("text")
	require.NoError(t, err)
	require.Empty(t, notObject)
}

// Test_EncodeKeepsEmptyTagValues checks that a tag with an empty value stays, in the map form and the list form, while
// an empty Tags property is still left out.
func Test_EncodeKeepsEmptyTagValues(t *testing.T) {
	encoded, err := EncodeObject(map[string]any{
		"Tags":   map[string]any{"owner": "", "team": "a"},
		"Nested": map[string]any{"Tags": []any{map[string]any{"Key": "owner", "Value": "", "ResourceArn": nil}}},
		"Empty":  map[string]any{"Tags": []any{}},
		"Name":   "",
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"Tags":   map[string]any{"owner": "", "team": "a"},
		"Nested": map[string]any{"Tags": []any{map[string]any{"Key": "owner", "Value": ""}}},
	}, encoded)
}

func Test_PropertyHelpers(t *testing.T) {
	properties := map[string]any{"Name": "app", "Ids": []any{"a", 1, "b"}, "Size": 2}
	require.Equal(t, "app", String(properties, "Name"))
	require.Empty(t, String(properties, "Size"))
	require.Equal(t, []string{"a", "b"}, Strings(properties, "Ids"))
	require.Empty(t, Strings(properties, "Missing"))
	require.Equal(t, map[string]any{"Size": 2}, Without(properties, "Name", "Ids"))
	require.Contains(t, properties, "Name", "Without doesn't change properties")

	request := native.UpdateRequest{Patch: jsondiff.Patch{
		{Type: jsondiff.OperationReplace, Path: "/EngineVersion"},
		{Type: jsondiff.OperationAdd, Path: "/Config/Version"},
		{Type: jsondiff.OperationRemove, Path: "/Config/Other"},
	}}
	require.Equal(t, []string{"EngineVersion", "Config"}, Changed(request))
}

func Test_Tags(t *testing.T) {
	properties := map[string]any{"Tags": []any{
		map[string]any{"Key": "team", "Value": "a"},
		map[string]any{"Key": "", "Value": "dropped"},
		"not a tag",
	}}
	require.Equal(t, []Tag{{Key: "team", Value: "a"}}, Tags(properties))

	require.Nil(t, TagList(map[string]string{"aws:cloudformation:stack-name": "s", OperationTag: "token"}))
	require.Equal(t, []any{
		map[string]any{"Key": "a", "Value": "1"},
		map[string]any{"Key": "b", "Value": "2"},
	}, TagList(map[string]string{"b": "2", "a": "1", OperationTag: "token"}))

	set, remove := TagChanges([]Tag{{"keep", "1"}, {"change", "1"}, {"drop", "1"}}, []Tag{{"keep", "1"}, {"change", "2"}, {"add", "1"}})
	require.Equal(t, []Tag{{"add", "1"}, {"change", "2"}}, set)
	require.Equal(t, []string{"drop"}, remove)
}

func Test_States(t *testing.T) {
	states := States{Done: []string{"available"}, Failed: []string{"failed", "incompatible-parameters"}}
	require.Equal(t, native.Progress{Done: true, State: "Available"}, states.Progress("Available"))
	failed := states.Progress("incompatible-parameters")
	require.True(t, failed.Done)
	require.Contains(t, failed.Failure.Message, "incompatible-parameters")
	require.Equal(t, native.Progress{State: "creating", Waiting: "available"}, states.Progress("creating"))
	require.Equal(t, native.Progress{State: "creating"}, States{}.Progress("creating"))
}

func Test_Calls(t *testing.T) {
	rejected := &smithy.GenericAPIError{Code: "InvalidParameterValue", Fault: smithy.FaultClient}
	made := []int{}
	call := func(i int, err error) func() error {
		return func() error {
			made = append(made, i)
			return err
		}
	}

	require.NoError(t, Calls(call(0, nil), call(1, nil)))
	require.Equal(t, []int{0, 1}, made)

	made = nil
	require.Same(t, rejected, Calls(call(0, rejected), call(1, nil)), "the first call's error is returned as it is")
	require.Equal(t, []int{0}, made)

	made = nil
	err := Calls(call(0, nil), call(1, rejected), call(2, nil))
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomePartial, failure.Outcome)
	require.True(t, failure.ClientError)
	require.Equal(t, []int{0, 1}, made)

	for _, tt := range []struct {
		err       error
		throttled bool
	}{
		{&smithy.GenericAPIError{Code: "InternalFailure", Fault: smithy.FaultServer}, false},
		{&smithy.GenericAPIError{Code: "ThrottlingException", Fault: smithy.FaultClient}, true},
		{attempts(response(http.StatusServiceUnavailable, nil), response(http.StatusBadRequest, rejected)), false},
		{attempts(response(http.StatusServiceUnavailable, nil), response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "Throttling"})), true},
	} {
		require.ErrorAs(t, Calls(call(0, nil), call(1, tt.err)), &failure)
		require.False(t, failure.ClientError, "%v", tt.err)
		require.Equal(t, tt.throttled, failure.Throttled, "%v", tt.err)
		require.Equal(t, native.OutcomePartial, failure.Outcome)
	}

	own := &native.Failure{Message: "rejected", Outcome: native.OutcomeRejected}
	require.Same(t, own, Calls(call(0, nil), call(1, own)), "a handler's own failure is kept")
}

func Test_FieldNames(t *testing.T) {
	require.Equal(t, []string{"Mode"}, FieldNames(&sdkSettings{}))
	require.Equal(t, []string{"Key", "Value"}, FieldNames(sdkTag{}))
}

func Test_ReservedTags(t *testing.T) {
	require.NoError(t, ReservedTags(map[string]any{"Tags": []any{map[string]any{"Key": "team", "Value": "a"}}}, OperationTag))
	require.NoError(t, ReservedTags(map[string]any{}, OperationTag))
	err := ReservedTags(map[string]any{"Tags": []any{
		map[string]any{"Key": "b", "Value": "1"}, map[string]any{"Key": "a", "Value": "2"}, map[string]any{"Key": "b", "Value": "3"},
	}}, "a", "b")
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "can't set them: a, b.")
}

// sdkOptions has the shape of an AWS SDK client's options.
type sdkOptions struct {
	Region string
}

func Test_SharedHelpers(t *testing.T) {
	options := &sdkOptions{}
	InRegion[sdkOptions]("eu-west-2")(options)
	require.Equal(t, "eu-west-2", options.Region)

	tags := SDKTags[sdkTag]([]Tag{{Key: "team", Value: "a"}, {Key: "owner", Value: ""}})
	require.Equal(t, []sdkTag{{Key: aws.String("team"), Value: aws.String("a")}, {Key: aws.String("owner"), Value: aws.String("")}}, tags)
	require.Empty(t, SDKTags[sdkTag](nil))

	tags = append(tags, sdkTag{Key: aws.String(OperationTag), Value: aws.String("token")}, sdkTag{Key: aws.String("private"), Value: aws.String("x")}, sdkTag{Key: aws.String("bare")})
	require.Equal(t, []any{
		map[string]any{"Key": "bare", "Value": ""},
		map[string]any{"Key": "owner", "Value": ""},
		map[string]any{"Key": "team", "Value": "a"},
	}, TagListOf(tags, "private"))
	require.Nil(t, TagListOf([]sdkTag{}))
	require.Panics(t, func() { SDKTags[int]([]Tag{{Key: "a"}}) })
	require.Panics(t, func() { TagListOf([]int{1}) })

	equivalent := CaseInsensitive("Name")
	require.True(t, equivalent("Name", "App", "app"))
	require.False(t, equivalent("Name", "App", "other"))
	require.False(t, equivalent("Other", "App", "app"))
	require.False(t, equivalent("Name", 1, "1"))
}

// response returns err as AWS's answer with status, as the SDK reports it.
func response(status int, err error) error {
	return &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}}, Err: err}
}

// attempts returns the error of a call the SDK made once for each of errs, the last of which it returned, with the
// attempts RecordAttempts keeps.
func attempts(errs ...error) error {
	return &smithy.OperationError{ServiceID: "RDS", OperationName: "CreateTenantDatabase", Err: &attemptsError{err: errs[len(errs)-1], attempts: errs}}
}

func Test_Rejected(t *testing.T) {
	invalid := response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "InvalidParameterValue", Message: "bad"})
	exists := response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "TenantDatabaseAlreadyExists", Fault: smithy.FaultClient})
	throttling := response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "Throttling", Message: "Rate exceeded"})
	unavailable := response(http.StatusServiceUnavailable, &smithy.GenericAPIError{Code: "ServiceUnavailable"})
	lost := errors.New("read tcp: connection reset by peer")

	rejected := []error{
		invalid,
		attempts(invalid),
		attempts(throttling, invalid),
		attempts(response(http.StatusTooManyRequests, &smithy.GenericAPIError{Code: "TooManyRequestsException"}), exists),
		response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "LimitExceededException", Message: "quota"}),
	}
	for _, err := range rejected {
		var failure *native.Failure
		require.ErrorAs(t, Rejected(err), &failure, "%v", err)
		require.True(t, failure.ClientError, "%v", err)
		require.False(t, failure.Throttled, "%v", err)
		require.Equal(t, native.OutcomeRejected, failure.Outcome, "%v", err)
		require.Equal(t, err.Error(), failure.Message)
	}

	throttles := map[string]error{
		"throttled":                  throttling,
		"throttled on every attempt": attempts(throttling, throttling, &retry.MaxAttemptsError{Attempt: 3, Err: throttling}),
		"429":                        response(http.StatusTooManyRequests, &smithy.GenericAPIError{Code: "SlowDown"}),
		"modeled as a client fault":  response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "ThrottlingException", Fault: smithy.FaultClient}),
	}
	for name, err := range throttles {
		var failure *native.Failure
		require.ErrorAs(t, Rejected(err), &failure, name)
		require.False(t, failure.ClientError, "%s: throttling isn't the request's fault", name)
		require.True(t, failure.Throttled, name)
		require.Equal(t, native.OutcomeRejected, failure.Outcome, "%s: AWS answered every attempt, so none applied", name)
		require.Contains(t, failure.Message, "AWS limited the rate of calls, so it changed nothing. Deploy again later", name)
		require.Contains(t, failure.Message, err.Error(), name)
	}

	var failure *native.Failure
	require.ErrorAs(t, Rejected(attempts(unavailable, throttling)), &failure)
	require.True(t, failure.Throttled)
	require.False(t, failure.ClientError)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "the 5xx may have reached AWS")
	require.Contains(t, failure.Message, "AWS limited the rate of calls, and an earlier attempt may have reached AWS")

	unknown := map[string]error{
		"after a 5xx":                attempts(unavailable, exists),
		"after a lost response":      attempts(lost, exists),
		"after a throttle and a 5xx": attempts(throttling, unavailable, exists),
		"used up the SDK's attempts": &retry.MaxAttemptsError{Attempt: 2, Err: invalid},
	}
	for name, err := range unknown {
		var failure *native.Failure
		require.ErrorAs(t, Rejected(err), &failure, name)
		require.False(t, failure.ClientError, name)
		require.False(t, failure.Conflict, name)
		require.False(t, failure.Throttled, "%s: an earlier attempt may have changed AWS", name)
		require.Equal(t, native.OutcomeUnknown, failure.Outcome, name)
		require.Contains(t, failure.Message, err.Error(), name)
	}

	for _, kept := range []error{
		nil,
		unavailable,
		attempts(invalid, unavailable),
		response(http.StatusFound, invalid),
		&smithy.GenericAPIError{Code: "InvalidParameterValue"},
		NotFound(attempts(unavailable, response(http.StatusNotFound, &smithy.GenericAPIError{Code: "DBInstanceNotFound"})), "DBInstanceNotFound"),
		&native.Failure{Message: "partly applied", Outcome: native.OutcomePartial},
	} {
		require.Equal(t, kept, Rejected(kept), "%v", kept)
	}
}

func Test_Retried(t *testing.T) {
	invalid := response(http.StatusBadRequest, &smithy.GenericAPIError{Code: "InvalidDBInstanceState"})
	unavailable := response(http.StatusServiceUnavailable, &smithy.GenericAPIError{Code: "ServiceUnavailable"})

	require.True(t, Retried(attempts(unavailable, invalid)))
	require.True(t, Retried(attempts(nil, invalid)), "an attempt without an error says nothing about what AWS did")
	require.True(t, Retried(&retry.MaxAttemptsError{Attempt: 3, Err: invalid}), "without the attempts, using them up shows a retry")
	require.False(t, Retried(attempts(invalid)))
	require.False(t, Retried(attempts(invalid, invalid)), "AWS answered every attempt with a 4xx, so none applied")
	require.False(t, Retried(&smithy.OperationError{Err: &attemptsError{err: invalid}}))
	require.False(t, Retried(invalid))
	require.False(t, Retried(nil))
}

func Test_RecordAttempts(t *testing.T) {
	stack := middleware.NewStack("call", smithyhttp.NewStackRequest)
	require.NoError(t, RecordAttempts(stack))
	require.Empty(t, stack.Finalize.List(), "a client that never retries has no attempts to keep")
}
