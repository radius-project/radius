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

package step

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
)

var _ Executor = (*DeployExecutor)(nil)

type DeployExecutor struct {
	Description string
	Template    string
	Parameters  []string

	// Application sets the `--application` command-line parameter. This is needed in cases where
	// the application is not defined in bicep.
	Application string

	// Environment sets the `--environment` command-line parameter. This is needed in cases where
	// the environment is not defined in bicep.
	Environment string

	// RetryDelay is the minimum time to wait between retry attempts.
	RetryDelay time.Duration

	// RetryBudget bounds how long the executor keeps starting new attempts,
	// measured from the moment the first attempt fails. Once it elapses no
	// further attempt begins, but an attempt already in flight runs to
	// completion under the caller's context. Zero disables retries.
	RetryBudget time.Duration

	// ShouldRetry is a predicate that determines whether a failed deployment should be retried.
	// If nil, no retries are attempted regardless of RetryBudget.
	ShouldRetry func(error) bool

	// waitForReady, when set, blocks before each retry until the Radius control
	// plane is serving again. Execute populates it from the test's Kubernetes
	// client; a nil value falls back to RetryDelay alone.
	waitForReady func(context.Context) error
}

// Default retry behavior applied by NewDeployExecutor for transient deployment
// Default retry behavior applied by NewDeployExecutor for transient deployment
// failures: image pull blips from shared registries, and the UCP connection
// resets, EOFs and restart-phase HTTP errors produced when the kind control
// plane restarts under runner pressure.
//
// The bound is wall-clock time rather than an attempt count because a
// control-plane outage makes every attempt fail instantly against a dead
// socket, so "2 retries, 30s apart" is spent in about a minute while the outage
// lasts several. Waiting for readiness between attempts (see waitForReady)
// spends that budget on recovery instead of on attempts that cannot succeed.
const (
	defaultTransientRetryDelay = 30 * time.Second

	// defaultTransientRetryBudget covers the longest control-plane outage
	// observed in CI (about 3.5 minutes between the first etcd request timeout
	// and the kube-apiserver serving again) with a little margin. It is an upper
	// bound: effectiveRetryBudget shortens it to fit the test binary's deadline.
	defaultTransientRetryBudget = 5 * time.Minute
)

// transientImagePullErrorMarkers are substrings that indicate a container image
// pull failed for a transient reason (a registry or network blip) rather than a
// permanent one (for example a nonexistent image or an authentication failure).
// The kubelet automatically retries pulls that surface these states, so
// re-running the deployment after a short delay typically succeeds.
var transientImagePullErrorMarkers = []string{
	// kubelet pull states. These are reported while the kubelet is still
	// retrying the pull with backoff and usually clear on their own once the
	// registry becomes reachable again.
	"ErrImagePull",
	"ImagePullBackOff",
	// containerd surfaces this when a manifest or layer download from the
	// registry fails partway through, commonly because the registry timed out.
	"failed to pull and unpack image",
	// The underlying HTTP error when a registry such as ghcr.io does not respond
	// to a manifest or blob request in time.
	"timeout awaiting response headers",
}

// IsTransientImagePullError reports whether err was caused by a transient
// container image pull failure that is likely to succeed on retry. It is used
// by tests that pull images from a shared registry (for example the
// ghcr.io/radius-project/mirror images), which occasionally fail to pull due to
// registry or network blips in CI.
func IsTransientImagePullError(err error) bool {
	return ErrorContainsAny(err, transientImagePullErrorMarkers...)
}

// transientConnectionErrorMarkers are substrings that indicate a deployment
// failed because the connection between rad and the UCP API server was reset or
// closed mid-request, rather than because the deployment itself was invalid.
//
// rad reaches UCP over the aggregated API path (/apis/api.ucp.dev/v1alpha3/...)
// using the connection built from the test kubeconfig, which for kind points at
// the apiserver port published on 127.0.0.1. When the control plane goes down -
// observed in CI as etcd failing to serve requests, followed a few minutes
// later by the kube-apiserver restarting - every in-flight connection is reset
// at once, so all parallel `rad deploy` invocations fail together. UCP and the
// Radius pods do not crash, so re-running the deployment once the control plane
// recovers typically succeeds.
var transientConnectionErrorMarkers = []string{
	// The socket to the apiserver was reset when it bounced, e.g.
	// `read tcp 127.0.0.1:38764->127.0.0.1:37481: read: connection reset by peer`.
	"connection reset by peer",
	// rad's HTTP client observed a clean close mid-response, e.g.
	// `Get "https://127.0.0.1:37481/.../operationStatuses/...": EOF`.
	": EOF",
	// The pod log-stream tailers and larger response bodies surface this variant
	// when the connection closes partway through a read.
	"unexpected EOF",
	// The write side was torn down while rad was still sending.
	"broken pipe",
}

// IsTransientConnectionError reports whether err was caused by a transient
// network disruption between rad and the UCP API server (a reset or closed
// connection) that is likely to succeed on retry. See
// transientConnectionErrorMarkers for the environmental root cause.
//
// A connection reset/EOF is a transport-level failure that rad surfaces as a
// non-structured exit error, never as a structured ARM error. It therefore
// never matches a *radcli.CLIError: guarding on the concrete type ensures a
// genuine deployment failure whose flattened ARM message happens to contain a
// marker such as "unexpected EOF" is not misclassified as retryable.
func IsTransientConnectionError(err error) bool {
	if _, ok := errors.AsType[*radcli.CLIError](err); ok {
		return false
	}
	return ErrorContainsAny(err, transientConnectionErrorMarkers...)
}

// transientAPIServerRestartErrorMarkers are substrings that indicate the
// kube-apiserver answered rad while it was still restarting, rather than that
// the deployment itself failed.
//
// A kind control-plane restart reaches rad in two phases. First the in-flight
// connections are torn down, producing the transport errors matched by
// transientConnectionErrorMarkers. Then the apiserver process is back and
// accepting connections, but has not finished initializing, so it answers with
// structured HTTP errors instead. Classifying only the first phase as retryable
// exhausts the retry budget while the cluster is still recovering, which is what
// makes a control-plane restart fail every parallel test in the job at once.
var transientAPIServerRestartErrorMarkers = []string{
	// 503 ServiceUnavailable returned by the kube-apiserver mux until every
	// handler - including the aggregation layer that fronts UCP - is registered:
	// `{"message":"the request has been made before all known HTTP paths have
	// been installed, please try again","reason":"ServiceUnavailable"}`.
	"before all known HTTP paths have been installed",
}

// IsTransientAPIServerRestartError reports whether err was caused by the kind
// kube-apiserver still recovering from a restart. See
// transientAPIServerRestartErrorMarkers for the environmental root cause.
//
// These errors originate in rad's connection health check, which returns a plain
// error rather than an ARM error response, so - as in IsTransientConnectionError
// - a *radcli.CLIError is never a match.
func IsTransientAPIServerRestartError(err error) bool {
	if _, ok := errors.AsType[*radcli.CLIError](err); ok {
		return false
	}

	if ErrorContainsAny(err, transientAPIServerRestartErrorMarkers...) {
		return true
	}

	// A 403 on the aggregated Radius API path is also a restart signal: until the
	// RBAC authorizer finishes reconciling the bootstrap policy, even the
	// cluster-admin user is denied, e.g. `forbidden: User "kubernetes-admin"
	// cannot get path "/apis/api.ucp.dev/v1alpha3"`. That denial is impossible
	// once the authorizer chain is wired up. Both markers are required so a real
	// authorization failure elsewhere is not swept up; if RBAC is genuinely
	// misconfigured the deployment still fails once the retries are exhausted.
	return ErrorContainsAny(err, "cannot get path") && ErrorContainsAny(err, "/apis/api.ucp.dev")
}

// IsTransientDeployError reports whether err was caused by any transient failure
// that a deployment is likely to recover from on retry - a container image pull
// blip (IsTransientImagePullError), a UCP connection reset/EOF
// (IsTransientConnectionError), or a kube-apiserver restart
// (IsTransientAPIServerRestartError). It is the default ShouldRetry predicate for
// DeployExecutor (see NewDeployExecutor).
func IsTransientDeployError(err error) bool {
	return IsTransientImagePullError(err) ||
		IsTransientConnectionError(err) ||
		IsTransientAPIServerRestartError(err)
}

// NewDeployExecutor creates a new DeployExecutor instance with the given template and parameters.
//
// By default the executor retries a deployment that fails with a transient
// error (see IsTransientDeployError) for up to defaultTransientRetryBudget,
// waiting for the control plane to become ready again before each attempt. Use
// WithRetry to override the budget, delay, and predicate.
func NewDeployExecutor(template string, parameters ...string) *DeployExecutor {
	return &DeployExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
		Parameters:  parameters,
		RetryDelay:  defaultTransientRetryDelay,
		RetryBudget: defaultTransientRetryBudget,
		ShouldRetry: IsTransientDeployError,
	}
}

// WithApplication sets the application name for the DeployExecutor instance and returns the same instance.
func (d *DeployExecutor) WithApplication(application string) *DeployExecutor {
	d.Application = application
	return d
}

// WithEnvironment sets the environment name for the DeployExecutor instance and returns the same instance.
func (d *DeployExecutor) WithEnvironment(environment string) *DeployExecutor {
	d.Environment = environment
	return d
}

// WithRetry configures retry behavior for transient deployment failures,
// replacing the default set by NewDeployExecutor. budget bounds how long new
// attempts keep being started, delay is the minimum wait between them, and
// shouldRetry decides which errors are eligible.
func (d *DeployExecutor) WithRetry(budget time.Duration, delay time.Duration, shouldRetry func(error) bool) *DeployExecutor {
	d.RetryBudget = budget
	d.RetryDelay = delay
	d.ShouldRetry = shouldRetry
	return d
}

// GetDescription returns the Description field of the DeployExecutor instance.
func (d *DeployExecutor) GetDescription() string {
	return d.Description
}

// Execute deploys an application from a template file using the provided parameters and logs the deployment process.
func (d *DeployExecutor) Execute(ctx context.Context, t *testing.T, options test.TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	t.Logf("deploying %s from file %s", d.Description, d.Template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	if d.waitForReady == nil && options.K8sClient != nil {
		d.waitForReady = newControlPlaneReadyWaiter(t, options.K8sClient)
	}

	deployFunc := func() error {
		return cli.Deploy(ctx, templateFilePath, d.Environment, d.Application, d.Parameters...)
	}

	err = d.executeWithRetry(ctx, t, deployFunc)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}

// executeWithRetry runs the deploy function, retrying transient failures until
// the retry budget is spent or the context is cancelled.
//
// The bound is wall-clock time because a control-plane outage makes a
// deployment fail immediately rather than slowly, so a fixed attempt count is
// consumed long before the control plane returns. Between attempts the loop
// waits for the delay and then for readiness, spending the budget on recovery
// rather than on attempts that cannot succeed.
func (d *DeployExecutor) executeWithRetry(ctx context.Context, t *testing.T, deployFunc func() error) error {
	err := deployFunc()
	if err == nil || d.ShouldRetry == nil {
		return err
	}

	budget := d.effectiveRetryBudget(t)
	if budget <= 0 {
		return err
	}

	retryCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	for attempt := 1; d.ShouldRetry(err); attempt++ {
		t.Logf("deployment attempt %d failed with a retryable error: %v", attempt, err)

		if waitErr := d.waitBeforeRetry(retryCtx, t); waitErr != nil {
			// A cancelled parent context is a caller-driven abort and is
			// reported as such. Exhausting the budget is not: the deployment
			// error is the meaningful failure to surface.
			if ctx.Err() != nil {
				return ctx.Err()
			}

			t.Logf("no longer retrying the deployment after %d attempts: %v", attempt, waitErr)
			return err
		}

		if err = deployFunc(); err == nil {
			return nil
		}
	}

	return err
}

// effectiveRetryBudget returns RetryBudget shortened so that retrying can never
// run the test binary out of time.
//
// The budget is per deploy step, so a control plane that never recovers makes
// every test pay it. The suites run in parallel batches - corerp-noncloud is 52
// tests at -parallel 10 - and their `go test -timeout` is as low as 15 minutes
// (FUNCTIONALTEST_TIMEOUT in functional-test-noncloud.yaml, versus 30 minutes
// for the cloud suites). A fixed budget large enough to cover a multi-minute
// outage in one suite would blow the deadline in another, replacing clear
// per-test failures with an opaque panic and risking the loss of the
// diagnostics collected on failure.
//
// Spending at most half the remaining time leaves each subsequent batch half of
// what is left, so the deadline is approached but never reached, without
// needing a per-suite constant.
func (d *DeployExecutor) effectiveRetryBudget(t *testing.T) time.Duration {
	deadline, ok := t.Deadline()
	if !ok {
		return d.RetryBudget
	}

	if half := time.Until(deadline) / 2; half < d.RetryBudget {
		return half
	}

	return d.RetryBudget
}

// waitBeforeRetry waits out the retry delay and then blocks until the control
// plane is ready, bounded by ctx.
func (d *DeployExecutor) waitBeforeRetry(ctx context.Context, t *testing.T) error {
	if d.RetryDelay > 0 {
		timer := time.NewTimer(d.RetryDelay)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if d.waitForReady == nil {
		// ctx.Err() is nil while budget remains. Returning it rather than a
		// bare nil matters when RetryDelay is also zero: with neither a delay
		// nor a readiness gate to block on, nothing else would consult the
		// deadline and the loop would keep starting attempts past the budget.
		return ctx.Err()
	}

	return d.waitForReady(ctx)
}
