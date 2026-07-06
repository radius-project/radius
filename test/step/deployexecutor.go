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

	// MaxRetries is the maximum number of retry attempts after the initial deployment fails.
	// Zero means no retries (default behavior).
	MaxRetries int

	// RetryDelay is the duration to wait between retry attempts.
	RetryDelay time.Duration

	// ShouldRetry is a predicate that determines whether a failed deployment should be retried.
	// If nil, no retries are attempted regardless of MaxRetries.
	ShouldRetry func(error) bool
}

// Default retry behavior applied by NewDeployExecutor for transient deployment
// failures. Functional tests hit two classes of environmental flake in CI:
// container image pulls from shared registries (for example the
// ghcr.io/radius-project/* images) that occasionally fail due to registry or
// network blips, and UCP connection resets/EOFs when the kind control-plane
// restarts under runner resource pressure and drops the port-forward tunnel.
// Callers can override these defaults with WithRetry.
const (
	defaultTransientMaxRetries = 2
	defaultTransientRetryDelay = 30 * time.Second
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
// In CI the Radius workspace reaches UCP through a local `kubectl port-forward`
// tunnel that proxies through the kind cluster's kube-apiserver. When the
// GitHub-hosted runner is under resource pressure the kind static control-plane
// pods (kube-apiserver/controller-manager/scheduler) restart, which drops every
// in-flight port-forward tunnel at once and resets all parallel `rad deploy`
// connections simultaneously. UCP and the Radius pods do not crash, so
// re-running the deployment once the control-plane recovers typically succeeds.
var transientConnectionErrorMarkers = []string{
	// The socket to the port-forward tunnel was reset when the kube-apiserver
	// (which the tunnel proxies through) bounced, e.g.
	// `read tcp 127.0.0.1:38764->127.0.0.1:37481: read: connection reset by peer`.
	"connection reset by peer",
	// rad's HTTP client observed a clean close of the tunnel mid-response, e.g.
	// `Get "https://127.0.0.1:37481/.../operationStatuses/...": EOF`.
	": EOF",
	// The pod log-stream tailers and larger response bodies surface this variant
	// when the tunnel closes partway through a read.
	"unexpected EOF",
	// The write side of the tunnel was torn down while rad was still sending.
	"broken pipe",
}

// IsTransientConnectionError reports whether err was caused by a transient
// network disruption between rad and the UCP API server (a reset or closed
// port-forward tunnel) that is likely to succeed on retry. See
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
// IsTransientDeployError). Use WithRetry to override the retry count, delay, and
// predicate.
func NewDeployExecutor(template string, parameters ...string) *DeployExecutor {
	return &DeployExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
		Parameters:  parameters,
		MaxRetries:  defaultTransientMaxRetries,
		RetryDelay:  defaultTransientRetryDelay,
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
// replacing the default transient image pull retry set by NewDeployExecutor.
// maxRetries is the number of additional attempts after the first failure.
// delay is the wait time between attempts. shouldRetry determines whether
// a given error is eligible for retry.
func (d *DeployExecutor) WithRetry(maxRetries int, delay time.Duration, shouldRetry func(error) bool) *DeployExecutor {
	d.MaxRetries = maxRetries
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

	deployFunc := func() error {
		return cli.Deploy(ctx, templateFilePath, d.Environment, d.Application, d.Parameters...)
	}

	err = d.executeWithRetry(ctx, t, deployFunc)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}

// executeWithRetry runs the deploy function with optional retry logic.
func (d *DeployExecutor) executeWithRetry(ctx context.Context, t *testing.T, deployFunc func() error) error {
	maxAttempts := 1
	if d.MaxRetries > 0 && d.ShouldRetry != nil {
		maxAttempts = d.MaxRetries + 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			t.Logf("waiting %s before retry attempt %d/%d", d.RetryDelay, attempt, maxAttempts)
			timer := time.NewTimer(d.RetryDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				lastErr = ctx.Err()
			}
			if ctx.Err() != nil {
				break
			}
		}

		lastErr = deployFunc()
		if lastErr == nil {
			break
		}

		if attempt == maxAttempts || !d.ShouldRetry(lastErr) {
			break
		}

		t.Logf("deployment attempt %d/%d failed with retryable error: %v", attempt, maxAttempts, lastErr)
	}

	return lastErr
}
