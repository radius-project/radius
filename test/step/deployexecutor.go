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

	// MaxRetries caps the number of retry attempts after the initial deployment
	// fails. Zero means uncapped: retries continue until RetryBudget is spent.
	MaxRetries int

	// RetryDelay is the minimum time to wait between retry attempts. When
	// WaitForReady is set the actual wait is this delay plus however long the
	// control plane takes to become ready again.
	RetryDelay time.Duration

	// RetryBudget bounds the total wall-clock time spent retrying, measured
	// from the moment the first attempt fails. Zero disables retries.
	RetryBudget time.Duration

	// ShouldRetry is a predicate that determines whether a failed deployment should be retried.
	// If nil, no retries are attempted regardless of RetryBudget.
	ShouldRetry func(error) bool

	// WaitForReady, when set, is called before each retry attempt and must
	// block until the Radius control plane is serving again or the context is
	// done. Execute populates it from the test's Kubernetes client; a nil value
	// disables the readiness gate and falls back to RetryDelay alone.
	WaitForReady func(context.Context) error
}

// Default retry behavior applied by NewDeployExecutor for transient deployment
// failures. Functional tests hit two classes of environmental flake in CI:
// container image pulls from shared registries (for example the
// ghcr.io/radius-project/* images) that occasionally fail due to registry or
// network blips, and UCP connection resets/EOFs when the kind control-plane
// restarts under runner resource pressure.
//
// Retrying is bounded by wall-clock time rather than by a fixed attempt count.
// A fixed count does not work for a control-plane outage: once the
// kube-apiserver is down every retry fails instantly against a dead socket, so
// a budget of "2 retries, 30s apart" is spent in about a minute while the
// outage lasts several. Pairing a deadline with the readiness gate in
// WaitForReady means the retry loop waits for the control plane to return
// instead of burning attempts against it. Callers can override these defaults
// with WithRetry.
const (
	defaultTransientRetryDelay = 30 * time.Second

	// defaultTransientRetryBudget is roughly three times the longest
	// control-plane outage observed in CI (about 3.5 minutes between the first
	// etcd request timeout and the kube-apiserver serving again).
	defaultTransientRetryBudget = 10 * time.Minute
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
// rad reaches UCP over the aggregated API path
// (/apis/api.ucp.dev/v1alpha3/...) using the connection built from the test
// kubeconfig, which for kind points at the apiserver port published on
// 127.0.0.1. When the control plane goes down - observed in CI as etcd failing
// to serve requests, followed a few minutes later by the kube-apiserver being
// restarted - every in-flight connection is reset at once, so all parallel
// `rad deploy` invocations fail together. Other parts of the suite (for example
// the gateway tests) reach workloads through a `kubectl port-forward` tunnel
// that proxies through the same kube-apiserver, and those tunnels break with
// the same errors. UCP and the Radius pods do not crash in either case, so
// re-running the deployment once the control plane recovers typically
// succeeds - which is what the readiness gate in WaitForReady waits for.
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

// IsTransientDeployError reports whether err was caused by any transient failure
// that a deployment is likely to recover from on retry - either a container
// image pull blip (IsTransientImagePullError) or a UCP connection reset/EOF
// (IsTransientConnectionError). It is the default ShouldRetry predicate for
// DeployExecutor (see NewDeployExecutor).
func IsTransientDeployError(err error) bool {
	return IsTransientImagePullError(err) || IsTransientConnectionError(err)
}

// NewDeployExecutor creates a new DeployExecutor instance with the given template and parameters.
//
// By default the executor retries a deployment that fails with a transient
// error - either a container image pull blip or a UCP connection reset/EOF (see
// IsTransientDeployError) - for up to defaultTransientRetryBudget, waiting for
// the control plane to become ready again before each attempt. Use WithRetry to
// override the attempt cap, delay, and predicate.
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
// replacing the default transient-error retry set by NewDeployExecutor.
// maxRetries caps the number of additional attempts after the first failure;
// zero leaves the attempt count uncapped so only RetryBudget bounds it. delay
// is the minimum wait between attempts. shouldRetry determines whether a given
// error is eligible for retry. The overall retry budget is unchanged; use
// WithRetryBudget to adjust it.
func (d *DeployExecutor) WithRetry(maxRetries int, delay time.Duration, shouldRetry func(error) bool) *DeployExecutor {
	d.MaxRetries = maxRetries
	d.RetryDelay = delay
	d.ShouldRetry = shouldRetry
	return d
}

// WithRetryBudget sets the total wall-clock time the executor may spend
// retrying a transient failure and returns the same instance.
func (d *DeployExecutor) WithRetryBudget(budget time.Duration) *DeployExecutor {
	d.RetryBudget = budget
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

	if d.WaitForReady == nil && options.K8sClient != nil {
		d.WaitForReady = newControlPlaneReadyWaiter(t, options.K8sClient)
	}

	deployFunc := func() error {
		return cli.Deploy(ctx, templateFilePath, d.Environment, d.Application, d.Parameters...)
	}

	err = d.executeWithRetry(ctx, t, deployFunc)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}

// retryEnabled reports whether the executor is configured to retry at all.
func (d *DeployExecutor) retryEnabled() bool {
	return d.ShouldRetry != nil && d.RetryBudget > 0
}

// executeWithRetry runs the deploy function, retrying transient failures until
// the retry budget is exhausted, the attempt cap (if any) is reached, or the
// context is cancelled.
//
// The budget is wall-clock based on purpose. The failure this guards against is
// a control-plane outage, during which a deployment fails immediately rather
// than slowly, so a fixed attempt count is consumed long before the control
// plane returns. Between attempts the loop waits for the delay and then blocks
// on WaitForReady, which means time is spent waiting for recovery rather than
// on attempts that cannot succeed.
func (d *DeployExecutor) executeWithRetry(ctx context.Context, t *testing.T, deployFunc func() error) error {
	err := deployFunc()
	if err == nil || !d.retryEnabled() {
		return err
	}

	deadline := time.Now().Add(d.RetryBudget)

	for attempt := 1; ; attempt++ {
		if !d.ShouldRetry(err) {
			return err
		}

		if d.MaxRetries > 0 && attempt > d.MaxRetries {
			t.Logf("deployment failed after %d retries: %v", d.MaxRetries, err)
			return err
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Logf("deployment retry budget of %s is exhausted after %d attempts: %v", d.RetryBudget, attempt, err)
			return err
		}

		t.Logf("deployment attempt %d failed with retryable error (%s of the %s retry budget remaining): %v",
			attempt, remaining.Round(time.Second), d.RetryBudget, err)

		retryCtx, cancel := context.WithDeadline(ctx, deadline)
		waitErr := d.waitBeforeRetry(retryCtx, t)
		cancel()

		if waitErr != nil {
			// A cancelled parent context is a caller-driven abort and is
			// reported as such. Exhausting the budget is not: the deployment
			// error is the meaningful failure to surface.
			if ctx.Err() != nil {
				return ctx.Err()
			}

			t.Logf("giving up on retrying the deployment: %v", waitErr)
			return err
		}

		err = deployFunc()
		if err == nil {
			return nil
		}
	}
}

// waitBeforeRetry waits out the retry delay and then blocks until the control
// plane is ready, bounded by ctx.
func (d *DeployExecutor) waitBeforeRetry(ctx context.Context, t *testing.T) error {
	if d.RetryDelay > 0 {
		timer := time.NewTimer(d.RetryDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}

	if d.WaitForReady == nil {
		return nil
	}

	return d.WaitForReady(ctx)
}
