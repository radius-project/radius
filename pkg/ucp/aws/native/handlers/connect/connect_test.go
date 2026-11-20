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

package connect

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// testPhoneARN and otherPhoneARN are the ARNs of testPhone and otherPhone, and testIdentifier names testPhone's
// association.
var (
	testPhoneARN   = phoneARN(testPhone)
	otherPhoneARN  = phoneARN(otherPhone)
	testIdentifier = testPhoneARN + "|" + testInstance
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// newHandler returns a handler that doesn't wait, with a fake backed by store.
func newHandler(store *awsfake.CloudControl) (*PhoneNumberContactFlowAssociation, *fakeAPI) {
	api := newFakeAPI(store)
	h := NewPhoneNumberContactFlowAssociation(api)
	h.wait = noWait
	return h, api
}

func association(phone string, instance string, flow string) map[string]any {
	return map[string]any{"PhoneNumberId": phone, "InstanceId": instance, "ContactFlowId": flow}
}

func Test_Suite_PhoneNumberContactFlowAssociation(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    associationType,
		Schema:      readFile(t, "aws-connect-phonenumbercontactflowassociation.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			h, _ := newHandler(store)
			return h
		},
		Properties:       association(testPhoneARN, testInstance, testFlow),
		Identifier:       testIdentifier,
		Changed:          association(testPhoneARN, testInstance, otherFlow),
		CreateOnlyChange: association(otherPhoneARN, testInstance, testFlow),
		// AWS returns the flow's ARN.
		OtherForm: map[string]any{"ContactFlowId": flowARN(testFlow)},
		IdentifierProperties: func(identifier string) map[string]any {
			phone, instance, _ := strings.Cut(identifier, "|")
			return map[string]any{"PhoneNumberId": phone, "InstanceId": instance}
		},
		// The identifier takes one form of each, so two aliases can't hold one association.
		Invalid: []map[string]any{
			association(testPhone, testInstance, testFlow),
			association(testPhoneARN, instanceARN(testInstance), testFlow),
		},
		API: associationAPI,
	})
}

// requireRejected requires err to be a client failure AWS applied none of, whose message contains text.
func requireRejected(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, text)
}

func count(calls []string, method string) int {
	n := 0
	for _, call := range calls {
		if call == method {
			n++
		}
	}
	return n
}

func Test_PhoneNumberContactFlowAssociation_Read(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	ctx := t.Context()
	// Three numbers, so the instance's associations take two pages.
	for _, phone := range []string{testPhone, otherPhone, tdgPhone} {
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(phoneARN(phone), testInstance, testFlow)})
		require.NoError(t, err)
	}
	api.stale = map[string]*stale{}

	for _, phone := range []string{testPhone, otherPhone, tdgPhone} {
		got, err := h.Read(ctx, region, phoneARN(phone)+"|"+testInstance)
		require.NoError(t, err)
		require.Equal(t, association(phoneARN(phone), testInstance, testFlow), got)
	}

	t.Run("a number without a flow is not found", func(t *testing.T) {
		_, err := h.Read(ctx, region, phoneARN(missingPhone)+"|"+testInstance)
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("another Region's number with the same ID is not found", func(t *testing.T) {
		_, err := h.Read(ctx, region, strings.Replace(testPhoneARN, region, tdgRegion, 1)+"|"+testInstance)
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("a missing instance is not found", func(t *testing.T) {
		_, err := h.Read(ctx, region, testPhoneARN+"|"+missingInstance)
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("an error is returned as it is, so waiting for an operation AWS applied doesn't fail it", func(t *testing.T) {
		api.errs["ListFlowAssociations"] = apiError(http.StatusTooManyRequests, "ThrottlingException", "Rate exceeded")
		defer delete(api.errs, "ListFlowAssociations")
		_, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.ErrorContains(t, err, "Rate exceeded")
		var failure *native.Failure
		require.False(t, errors.As(err, &failure))
	})

	t.Run("an identifier in another form is rejected before any call", func(t *testing.T) {
		api.calls = nil
		for _, identifier := range []string{
			testPhoneARN, testPhoneARN + "|", "|" + testInstance, testPhone + "|" + testInstance,
			testPhoneARN + "|" + instanceARN(testInstance), testPhoneARN + "|" + testInstance + "|x",
		} {
			_, err := h.Read(ctx, region, identifier)
			requireRejected(t, err, "the phone number's ARN")
		}
		require.Empty(t, api.calls)
	})
}

func Test_PhoneNumberContactFlowAssociation_Equivalent(t *testing.T) {
	require.True(t, equivalent("ContactFlowId", testFlow, flowARN(testFlow)))
	require.True(t, equivalent("ContactFlowId", flowARN(testFlow), testFlow))
	require.True(t, equivalent("ContactFlowId", flowARN(testFlow), flowARN(testFlow)))
	require.False(t, equivalent("ContactFlowId", otherFlow, flowARN(testFlow)))
	require.False(t, equivalent("ContactFlowId", "contact-flow/"+testFlow, flowARN(testFlow)), "only an ARN gives a flow's ID")
	require.False(t, equivalent("InstanceId", testInstance, instanceARN(testInstance)), "only the flow has two forms")
}

func Test_PhoneNumberContactFlowAssociation_Create(t *testing.T) {
	ctx := t.Context()

	t.Run("waits until a list shows the flow", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		require.NoError(t, err)
		require.Equal(t, testIdentifier, identifier)
		require.Equal(t, staleLists+1, count(api.calls, "ListFlowAssociations"))
		progress, err := h.Progress(ctx, region, identifier, native.OperationCreate)
		require.NoError(t, err)
		require.True(t, progress.Done)
	})

	t.Run("a number AWS doesn't have is rejected", func(t *testing.T) {
		h, _ := newHandler(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(phoneARN(missingPhone), testInstance, testFlow)})
		requireRejected(t, err, "Phone number not found")
	})

	t.Run("a number of a traffic distribution group is found by ARN in the other Region", func(t *testing.T) {
		h, _ := newHandler(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(phoneARN(tdgPhone), testInstance, testFlow)})
		require.NoError(t, err)
	})
}

func Test_PhoneNumberContactFlowAssociation_Update(t *testing.T) {
	ctx := t.Context()
	setup := func(t *testing.T) (*PhoneNumberContactFlowAssociation, *fakeAPI, map[string]any) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		require.NoError(t, err)
		current, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		api.calls = nil
		return h, api, current
	}
	update := func(t *testing.T, current map[string]any, desired map[string]any) native.UpdateRequest {
		patch, err := jsondiff.Compare(current, desired)
		require.NoError(t, err)
		return native.UpdateRequest{Region: region, Identifier: testIdentifier, Current: current, Desired: desired, Patch: patch}
	}

	t.Run("another flow replaces the number's flow, by ID or ARN", func(t *testing.T) {
		for _, flow := range []string{otherFlow, flowARN(otherFlow)} {
			h, api, current := setup(t)
			require.NoError(t, h.Update(ctx, update(t, current, association(testPhoneARN, testInstance, flow))))
			got, err := h.Read(ctx, region, testIdentifier)
			require.NoError(t, err)
			require.Equal(t, flow, got["ContactFlowId"], "the update waits until a list shows the flow")
			require.Equal(t, 1, count(api.calls, "AssociatePhoneNumberContactFlow"))
		}
	})

	t.Run("an update that doesn't change the flow makes no call", func(t *testing.T) {
		h, api, current := setup(t)
		require.NoError(t, h.Update(ctx, update(t, current, current)))
		require.Empty(t, api.calls)
	})

	t.Run("an update of a number AWS no longer has is not found", func(t *testing.T) {
		h, api, current := setup(t)
		api.errs["AssociatePhoneNumberContactFlow"] = resourceNotFound("Phone number not found: %s", testPhone)
		err := h.Update(ctx, update(t, current, association(testPhoneARN, testInstance, otherFlow)))
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_PhoneNumberContactFlowAssociation_Delete(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	ctx := t.Context()
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
	require.NoError(t, err)

	require.NoError(t, h.Delete(ctx, region, testIdentifier))
	progress, err := h.Progress(ctx, region, testIdentifier, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, progress.Done, "a list right after the delete still shows the flow")
	require.Equal(t, "disassociated", progress.Waiting)
	for range staleLists {
		_, err = h.Progress(ctx, region, testIdentifier, native.OperationDelete)
	}
	require.ErrorIs(t, err, native.ErrNotFound)
	_, ok := api.store.Read(associationType, testIdentifier)
	require.False(t, ok)

	require.ErrorIs(t, h.Delete(ctx, region, phoneARN(missingPhone)+"|"+testInstance), native.ErrNotFound, "a delete of a number AWS doesn't have finishes")

	_, err = h.List(ctx, region)
	require.ErrorContains(t, err, "can't be listed")
}

// requireUnknown requires err to be a failure whose outcome is unknown, which asks to deploy again and contains text.
func requireUnknown(t *testing.T, err error, text string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.False(t, failure.ClientError)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.Contains(t, failure.Message, "Deploy again")
	require.Contains(t, failure.Message, text)
}

func Test_PhoneNumberContactFlowAssociation_UnconfirmedChange(t *testing.T) {
	ctx := t.Context()

	t.Run("a create that lists keep failing to show is unknown", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		h.wait = func(context.Context, time.Duration) {
			api.errs["ListFlowAssociations"] = apiError(http.StatusTooManyRequests, "ThrottlingException", "Rate exceeded")
		}
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		requireUnknown(t, err, "Rate exceeded")
		require.Equal(t, testIdentifier, identifier, "UCP keeps the identifier of a create AWS accepted")
		require.Equal(t, confirmReads, count(api.calls, "ListFlowAssociations"))
	})

	t.Run("a create AWS failed on its side keeps the identifier", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		api.errs["AssociatePhoneNumberContactFlow"] = &smithy.OperationError{ServiceID: "Connect", OperationName: "AssociatePhoneNumberContactFlow", Err: &smithyhttp.ResponseError{
			Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusInternalServerError}},
			Err:      &smithy.GenericAPIError{Code: "InternalServiceException", Message: "internal error", Fault: smithy.FaultServer},
		}}
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		require.ErrorContains(t, err, "internal error")
		require.Equal(t, testIdentifier, identifier, "AWS may have associated the flow")
	})

	t.Run("a create no list shows in time is unknown", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		h.wait = func(context.Context, time.Duration) { api.stale[testIdentifier].lists = confirmReads }
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		requireUnknown(t, err, "no read showed it")
	})

	t.Run("a create whose context ends is unknown", func(t *testing.T) {
		h, _ := newHandler(awsfake.NewCloudControl())
		ctx, cancel := context.WithCancel(ctx)
		h.wait = func(context.Context, time.Duration) { cancel() }
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		requireUnknown(t, err, "context canceled")
	})

	t.Run("an update no list shows in time is unknown", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: association(testPhoneARN, testInstance, testFlow)})
		require.NoError(t, err)
		current, err := h.Read(ctx, region, testIdentifier)
		require.NoError(t, err)
		h.wait = func(context.Context, time.Duration) { api.stale[testIdentifier].lists = confirmReads }
		desired := association(testPhoneARN, testInstance, otherFlow)
		patch, err := jsondiff.Compare(current, desired)
		require.NoError(t, err)
		err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: testIdentifier, Current: current, Desired: desired, Patch: patch})
		requireUnknown(t, err, "no read showed it")
	})

	t.Run("sleep returns when the context ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		sleep(ctx, time.Hour)
	})
}

func Test_PhoneNumberContactFlowAssociation_CreateProgress(t *testing.T) {
	ctx := t.Context()

	t.Run("a lost create waits for a list to show the flow", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		require.NoError(t, associate(ctx, api.store, testPhoneARN, testInstance, testFlow))
		api.stale[testIdentifier] = &stale{after: association(testPhoneARN, testInstance, testFlow), lists: staleLists}
		progress, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.NoError(t, err)
		require.True(t, progress.Done)
		require.Equal(t, staleLists+1, count(api.calls, "ListFlowAssociations"))
	})

	t.Run("a create AWS never got is not found after the lists", func(t *testing.T) {
		h, api := newHandler(awsfake.NewCloudControl())
		_, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.ErrorIs(t, err, native.ErrNotFound)
		require.Equal(t, confirmReads, count(api.calls, "ListFlowAssociations"))
	})

	t.Run("a context that ends stops the lists", func(t *testing.T) {
		h, _ := newHandler(awsfake.NewCloudControl())
		ctx, cancel := context.WithCancel(ctx)
		h.wait = func(context.Context, time.Duration) { cancel() }
		_, err := h.Progress(ctx, region, testIdentifier, native.OperationCreate)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func Test_PhoneNumberContactFlowAssociation_InvalidIdentifier(t *testing.T) {
	h, api := newHandler(awsfake.NewCloudControl())
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: association(testPhone, testInstance, testFlow)})
	requireRejected(t, err, "the phone number's ARN")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"PhoneNumberId": testPhoneARN, "ContactFlowId": testFlow}})
	requireRejected(t, err, "the phone number's ARN")
	requireRejected(t, h.Delete(t.Context(), region, testPhoneARN), "the phone number's ARN")
	require.Empty(t, api.calls)
}
