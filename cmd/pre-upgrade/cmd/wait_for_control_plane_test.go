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
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
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
		{name: "deployments without namespace", options: waitForControlPlaneOptions{timeout: time.Second, pollInterval: time.Second, requestTimeout: time.Second, deployments: []string{"ucp"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.options.validate())
		})
	}
}

func TestNewCommandParsesDeploymentFlags(t *testing.T) {
	cmd := newWaitForControlPlaneCommand()
	require.NoError(t, cmd.ParseFlags([]string{"--namespace=radius-system", "--deployment=ucp", "--deployment=controller"}))

	deployments, err := cmd.Flags().GetStringArray("deployment")
	require.NoError(t, err)
	require.Equal(t, []string{"ucp", "controller"}, deployments)
	namespace, err := cmd.Flags().GetString("namespace")
	require.NoError(t, err)
	require.Equal(t, "radius-system", namespace)
}

func TestProbeDeployments(t *testing.T) {
	replicas := int32(1)
	deployment := func(name string, generation, observed int64, updated, available int32) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "radius-system", Generation: generation},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ObservedGeneration: observed, UpdatedReplicas: updated, AvailableReplicas: available},
		}
	}

	tests := []struct {
		name        string
		deployments []*appsv1.Deployment
		wantErr     string
		notFound    bool
	}{
		{name: "all available", deployments: []*appsv1.Deployment{deployment("ucp", 2, 2, 1, 1), deployment("controller", 1, 1, 1, 1)}},
		{name: "generation not observed", deployments: []*appsv1.Deployment{deployment("ucp", 2, 1, 1, 1), deployment("controller", 1, 1, 1, 1)}, wantErr: "has not observed generation 2"},
		{name: "rollout not updated", deployments: []*appsv1.Deployment{deployment("ucp", 1, 1, 1, 1), deployment("controller", 2, 2, 0, 1)}, wantErr: "0 of 1 replicas updated"},
		{name: "not available", deployments: []*appsv1.Deployment{deployment("ucp", 1, 1, 1, 0), deployment("controller", 1, 1, 1, 1)}, wantErr: "0 of 1 replicas available"},
		{name: "missing deployment", deployments: []*appsv1.Deployment{deployment("ucp", 1, 1, 1, 1)}, notFound: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset()
			for _, d := range tt.deployments {
				_, err := client.AppsV1().Deployments(d.Namespace).Create(t.Context(), d, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := probeDeployments(t.Context(), client, "radius-system", []string{"ucp", "controller"}, time.Second)
			switch {
			case tt.notFound:
				require.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v", err)
			case tt.wantErr != "":
				require.ErrorContains(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
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
