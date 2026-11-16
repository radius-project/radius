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

package elbv2

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region    = "us-west-2"
	listener  = "arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/web/50dc6c495c0c9188/f2f7dc8efc522ab2"
	listener2 = "arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/web/50dc6c495c0c9188/0467ef3c8400ae65"
	certA     = "arn:aws:acm:us-west-2:123456789012:certificate/11111111-1234-46f1-abcd-111111111111"
	certB     = "arn:aws:acm:us-west-2:123456789012:certificate/22222222-1234-46f1-abcd-222222222222"
	certC     = "arn:aws:acm:us-west-2:123456789012:certificate/33333333-1234-46f1-abcd-333333333333"
	certD     = "arn:aws:acm:us-west-2:123456789012:certificate/44444444-1234-46f1-abcd-444444444444"
	outsider  = "arn:aws:acm:us-west-2:123456789012:certificate/55555555-1234-46f1-abcd-555555555555"
	token     = "11111111-2222-3333-4444-555555555555"
)

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

func certificates(arns ...string) []any {
	out := []any{}
	for _, arn := range arns {
		out = append(out, map[string]any{"CertificateArn": arn})
	}
	return out
}

func properties(listenerArn string, arns ...string) map[string]any {
	return map[string]any{"ListenerArn": listenerArn, "Certificates": certificates(arns...)}
}

func Test_Suite_ListenerCertificate(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName: listenerCertificateType,
		Schema:   schema(t, "aws-elasticloadbalancingv2-listenercertificate.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return newTracked(store)
		},
		IdentifierProperty: "Id",
		Properties:         properties(listener, certA, certB),
		Changed:            properties(listener, certB, certC),
		SharedMember:       &nativetest.SharedMember{Overlapping: properties(listener, certB, certC), Disjoint: properties(listener, certC)},
		CreateOnlyChange:   properties(listener2, certA, certB),
		OtherForm:          map[string]any{"Certificates": certificates(certB, certA)},
		MultiCall:          properties(listener, certB, certC, certD),
		Calls:              3,
		RejectedCalls:      1,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &h.(*tracked).api.failer
		}),
		Outside: func(store *awsfake.CloudControl, props map[string]any) func() bool {
			listenerArn := props["ListenerArn"].(string)
			current := store.Resource(outsideType, listenerArn)
			if current == nil {
				current = properties(listenerArn)
			}
			current["Certificates"] = append(current["Certificates"].([]any), map[string]any{"CertificateArn": outsider})
			store.Put(outsideType, listenerArn, current)
			return func() bool {
				return slices.Contains(certificateArns(store.Resource(outsideType, listenerArn)), outsider)
			}
		},
	})
}

func Test_ListenerCertificate_CreateAndRead(t *testing.T) {
	store := awsfake.NewCloudControl()
	h := newTracked(store)
	ctx := t.Context()

	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA, certB), ClientToken: token})
	require.NoError(t, err)
	require.Equal(t, listener+"|"+native.GenerateName(token), identifier)
	require.Equal(t, []string{"AddListenerCertificates", "AddListenerCertificates"}, h.api.calls, "one certificate per call")
	for _, r := range h.api.regions {
		require.Equal(t, region, r)
	}

	again, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA, certB), ClientToken: token})
	require.NoError(t, err)
	require.Equal(t, identifier, again, "a resent create returns the same identifier")
	require.Equal(t, []string{certA, certB}, certificateArns(store.Resource(listenerCertificateType, identifier)))

	got, err := h.Read(ctx, region, identifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Id": identifier, "ListenerArn": listener, "Certificates": certificates(certA, certB)}, got)

	progress, err := h.Progress(ctx, region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_ListenerCertificate_ReadScoped(t *testing.T) {
	store := awsfake.NewCloudControl()
	h := newTracked(store)
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA), ClientToken: token})
	require.NoError(t, err)
	store.Put(outsideType, listener, properties(listener, outsider, defaultCertificate(listener)))

	t.Run("only managed certificates", func(t *testing.T) {
		got, err := h.ReadScoped(ctx, region, identifier, []map[string]any{properties(listener, certA, certC)})
		require.NoError(t, err)
		require.Equal(t, certificates(certA), got["Certificates"])
	})

	t.Run("every certificate but the default one without a record", func(t *testing.T) {
		got, err := h.ReadScoped(ctx, region, identifier, nil)
		require.NoError(t, err)
		require.Equal(t, certificates(certA, outsider, defaultCertificate(listener)), got["Certificates"], "the default certificate is returned only because it is also in the list")
	})

	t.Run("the default certificate when the template lists it", func(t *testing.T) {
		got, err := h.ReadScoped(ctx, region, identifier, []map[string]any{properties(listener, defaultCertificate(listener))})
		require.NoError(t, err)
		require.Equal(t, certificates(defaultCertificate(listener)), got["Certificates"])
	})

	t.Run("none of the managed certificates", func(t *testing.T) {
		_, err := h.ReadScoped(ctx, region, identifier, []map[string]any{properties(listener, certD)})
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("listener gone", func(t *testing.T) {
		h.api.missing[listener] = true
		defer delete(h.api.missing, listener)
		_, err := h.ReadScoped(ctx, region, identifier, []map[string]any{properties(listener, certA)})
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("describe fails", func(t *testing.T) {
		h.api.errs["DescribeListenerCertificates"] = fault("Throttling")
		defer delete(h.api.errs, "DescribeListenerCertificates")
		_, err := h.ReadScoped(ctx, region, identifier, []map[string]any{properties(listener, certA)})
		require.Error(t, err)
		require.NotErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("identifier another handler assigned", func(t *testing.T) {
		got, err := h.ReadScoped(ctx, region, "id-1", []map[string]any{{"Certificates": certificates(certA)}, properties(listener, certA)})
		require.NoError(t, err)
		require.Equal(t, listener, got["ListenerArn"])
	})

	t.Run("identifier with no listener", func(t *testing.T) {
		_, err := h.Read(ctx, region, "id-1")
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
	})
}

func Test_ListenerCertificate_DefaultCertificateOnly(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	_, err := h.Read(t.Context(), region, listener+"|radius-0123456789ab")
	require.ErrorIs(t, err, native.ErrNotFound, "a listener with only its default certificate has none to return")
}

func Test_ListenerCertificate_CreateFails(t *testing.T) {
	ctx := t.Context()

	t.Run("no certificates", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener), ClientToken: token})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Empty(t, identifier)
		require.Empty(t, h.api.calls)
	})

	t.Run("first call", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.api.failer.FailOn(0, fault("CertificateNotFound"), false)
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA, certB), ClientToken: token})
		require.Error(t, err)
		require.Empty(t, identifier)
	})

	t.Run("a later call", func(t *testing.T) {
		h := newTracked(awsfake.NewCloudControl())
		h.api.failer.FailOn(1, fault("TooManyCertificates"), false)
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA, certB), ClientToken: token})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomePartial, failure.Outcome)
		require.Equal(t, listener+"|"+native.GenerateName(token), identifier, "the identifier is saved for the certificate already added")
	})
}

func Test_ListenerCertificate_Update(t *testing.T) {
	store := awsfake.NewCloudControl()
	h := newTracked(store)
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA, certB), ClientToken: token})
	require.NoError(t, err)
	h.api.calls = nil

	update := native.UpdateRequest{Region: region, Identifier: identifier, Current: properties(listener, certA, certB), Desired: properties(listener, certB, certC), Patch: jsondiff.Patch{}}
	require.NoError(t, h.Update(ctx, update))
	require.Equal(t, []string{"RemoveListenerCertificates", "AddListenerCertificates"}, h.api.calls, "removals first, then adds")
	require.Equal(t, []string{certB, certC}, certificateArns(store.Resource(listenerCertificateType, identifier)))

	t.Run("a failed removal is rejected", func(t *testing.T) {
		h.api.failer.FailOn(0, errors.New("connection reset"), false)
		err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: properties(listener, certB, certC), Desired: properties(listener, certC)})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.False(t, failure.ClientError)
	})

	t.Run("identifier with no listener", func(t *testing.T) {
		err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: "id-1", Current: map[string]any{}, Desired: map[string]any{}})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
	})
}

func Test_ListenerCertificate_DeleteScoped(t *testing.T) {
	ctx := t.Context()
	setup := func(t *testing.T) (*tracked, *awsfake.CloudControl, string) {
		store := awsfake.NewCloudControl()
		h := newTracked(store)
		identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties(listener, certA, certB), ClientToken: token})
		require.NoError(t, err)
		store.Put(outsideType, listener, properties(listener, outsider))
		return h, store, identifier
	}

	t.Run("removes only managed certificates", func(t *testing.T) {
		h, store, identifier := setup(t)
		require.NoError(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{properties(listener, certA, certB)}))
		require.Nil(t, store.Resource(listenerCertificateType, identifier))
		require.Equal(t, []string{outsider}, certificateArns(store.Resource(outsideType, listener)))
		progress, err := h.Progress(ctx, region, identifier, native.OperationDelete)
		require.NoError(t, err)
		require.True(t, progress.Done)

		require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{properties(listener, certA, certB)}), native.ErrNotFound)
	})

	t.Run("no record removes nothing", func(t *testing.T) {
		h, store, identifier := setup(t)
		require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, nil), native.ErrNotFound)
		require.Equal(t, []string{certA, certB}, certificateArns(store.Resource(listenerCertificateType, identifier)))
	})

	t.Run("listener gone", func(t *testing.T) {
		h, _, identifier := setup(t)
		h.api.missing[listener] = true
		require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{properties(listener, certA)}), native.ErrNotFound)
	})

	t.Run("listener gone during the removal", func(t *testing.T) {
		h, _, identifier := setup(t)
		h.api.errs["RemoveListenerCertificates"] = fault("ListenerNotFound")
		require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{properties(listener, certA)}), native.ErrNotFound)
	})

	t.Run("delete without a record of the certificates", func(t *testing.T) {
		h, _, identifier := setup(t)
		require.Error(t, h.Delete(ctx, region, identifier))
	})
}

func Test_ListenerCertificate_Options(t *testing.T) {
	h := NewListenerCertificate(newFakeAPI(awsfake.NewCloudControl()))
	require.Equal(t, listenerCertificateType, h.TypeName())
	require.Equal(t, native.ListNone, h.Options().List)
	require.Equal(t, native.RecoverByResend, h.Options().Recovery)
	_, err := h.List(t.Context(), region)
	require.Error(t, err)
}
