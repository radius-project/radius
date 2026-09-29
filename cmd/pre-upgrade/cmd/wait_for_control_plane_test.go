/*
Copyright 2026 The Radius Authors.

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

package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	restfake "k8s.io/client-go/rest/fake"
)

func TestWaitForControlPlaneRetriesServiceUnavailable(t *testing.T) {
	attempts := 0
	probe := func(context.Context) error {
		attempts++
		if attempts < 3 {
			return apierrors.NewServiceUnavailable("aggregated API not available")
		}
		return nil
	}

	err := waitForControlPlane(t.Context(), testWaitOptions(), logr.Discard(), probe)
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestWaitForControlPlaneRetriesNotFound(t *testing.T) {
	attempts := 0
	probe := func(context.Context) error {
		attempts++
		if attempts == 1 {
			return apierrors.NewNotFound(schema.GroupResource{Group: "apiregistration.k8s.io", Resource: "apiservices"}, "v1alpha3.api.ucp.dev")
		}
		return nil
	}

	err := waitForControlPlane(t.Context(), testWaitOptions(), logr.Discard(), probe)
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestWaitForControlPlaneReturnsLastErrorOnTimeout(t *testing.T) {
	lastErr := errors.New("connection reset")
	options := testWaitOptions()
	options.timeout = 20 * time.Millisecond
	options.pollInterval = 2 * time.Millisecond

	err := waitForControlPlane(t.Context(), options, logr.Discard(), func(context.Context) error {
		return lastErr
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, lastErr.Error())
}

func TestWaitForControlPlaneFailsRepeatedAuthorizationErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "unauthorized", err: apierrors.NewUnauthorized("missing token")},
		{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Resource: "apis"}, "", errors.New("denied"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			err := waitForControlPlane(t.Context(), testWaitOptions(), logr.Discard(), func(context.Context) error {
				attempts++
				return tt.err
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "non-retryable authorization error")
			require.Equal(t, authorizationFailureThreshold, attempts)
		})
	}
}

func TestWaitForControlPlaneRetriesTransientAuthorizationError(t *testing.T) {
	attempts := 0
	err := waitForControlPlane(t.Context(), testWaitOptions(), logr.Discard(), func(context.Context) error {
		attempts++
		if attempts == 1 {
			return apierrors.NewForbidden(schema.GroupResource{Resource: "apis"}, "", errors.New("RBAC propagation"))
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestWaitForControlPlaneBoundsHungProbe(t *testing.T) {
	options := testWaitOptions()
	options.timeout = 30 * time.Millisecond
	options.requestTimeout = 5 * time.Millisecond
	options.pollInterval = time.Millisecond

	err := waitForControlPlane(t.Context(), options, logr.Discard(), func(ctx context.Context) error {
		probeCtx, cancel := context.WithTimeout(ctx, options.requestTimeout)
		defer cancel()
		<-probeCtx.Done()
		return probeCtx.Err()
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestProbeControlPlaneUsesAggregatedAPIPath(t *testing.T) {
	client := &restfake.RESTClient{
		NegotiatedSerializer: kubernetesscheme.Codecs.WithoutConversion(),
		GroupVersion:         schema.GroupVersion{},
		Resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		},
	}

	err := probeControlPlane(t.Context(), client, time.Second)
	require.NoError(t, err)
	require.Equal(t, radiusAggregatedAPIPath, client.Req.URL.Path)
}

func TestProbeControlPlaneReportsServiceUnavailable(t *testing.T) {
	client := &restfake.RESTClient{
		NegotiatedSerializer: kubernetesscheme.Codecs.WithoutConversion(),
		GroupVersion:         schema.GroupVersion{},
		Resp: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"service unavailable","reason":"ServiceUnavailable","code":503}`,
			)),
		},
	}

	err := probeControlPlane(t.Context(), client, time.Second)
	require.True(t, apierrors.IsServiceUnavailable(err), "expected ServiceUnavailable, got %v", err)
	require.Equal(t, radiusAggregatedAPIPath, client.Req.URL.Path)
}

func TestNewCommandIncludesWaitForControlPlane(t *testing.T) {
	cmd := NewCommand()

	found, args, err := cmd.Find([]string{"wait-for-control-plane", "--timeout=15s"})
	require.NoError(t, err)
	require.Equal(t, "wait-for-control-plane", found.Name())
	require.Equal(t, []string{"--timeout=15s"}, args)
	require.NotNil(t, cmd.RunE, "the root command must preserve no-argument preflight behavior")
}

func TestWaitForControlPlaneOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		options waitForControlPlaneOptions
	}{
		{name: "timeout", options: waitForControlPlaneOptions{pollInterval: time.Second, requestTimeout: time.Second}},
		{name: "poll interval", options: waitForControlPlaneOptions{timeout: time.Second, requestTimeout: time.Second}},
		{name: "request timeout", options: waitForControlPlaneOptions{timeout: time.Second, pollInterval: time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.options.validate())
		})
	}
}

func testWaitOptions() waitForControlPlaneOptions {
	return waitForControlPlaneOptions{
		timeout:        time.Second,
		pollInterval:   time.Millisecond,
		requestTimeout: 10 * time.Millisecond,
	}
}
