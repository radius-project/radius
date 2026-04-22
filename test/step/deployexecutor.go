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

// NewDeployExecutor creates a new DeployExecutor instance with the given template and parameters.
func NewDeployExecutor(template string, parameters ...string) *DeployExecutor {
	return &DeployExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
		Parameters:  parameters,
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

// WithRetry configures retry behavior for transient deployment failures.
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
