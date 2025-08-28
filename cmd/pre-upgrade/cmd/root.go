/*
Copyright 2025 The Radius Authors.

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
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/upgrade/preupgrade"
)

// Config holds all configuration for the pre-upgrade command
type Config struct {
	TargetVersion string
	EnabledChecks []string
	Timeout       time.Duration
	RetryAttempts int
	RetryDelay    time.Duration
}

var rootCmd = &cobra.Command{
	Use:   "pre-upgrade",
	Short: "Pre-upgrade service",
	Long:  `Pre-upgrade service for Radius, which performs checks before an upgrade.`,
}

// ExecuteWithContext executes the root command with the given context
func ExecuteWithContext(ctx context.Context) error {
	rootCmd.SetContext(ctx)
	return Execute()
}

// Execute runs the root command and is the main entry point for the pre-upgrade CLI
func Execute() error {
	ctx := rootCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Parse configuration from environment
	cfg := parseConfig()

	// Set up logging using ucplog for consistency with other Radius containers
	loggingOptions := &ucplog.LoggingOptions{
		// These will be overridden by RADIUS_LOGGING_LEVEL and RADIUS_LOGGING_JSON env vars
		Level: "info",
		Json:  true,
	}

	logger, flush, err := ucplog.NewLogger("pre-upgrade", loggingOptions)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer flush()

	// Add logger to context
	ctx = logr.NewContext(ctx, logger)

	// Create output writer that uses the logger
	outputWriter := &logrOutputWriter{
		logger: logger,
	}

	// Create preflight configuration
	preflightConfig := preupgrade.Config{
		Helm: &helm.Impl{
			Helm: helm.NewHelmClient(),
		},
		Output: outputWriter,
	}

	// Get current version from cluster
	currentVersion := getCurrentVersion(preflightConfig, logger)

	// Prepare options for preflight checks
	options := preupgrade.Options{
		EnabledChecks:  cfg.EnabledChecks,
		TargetVersion:  cfg.TargetVersion,
		CurrentVersion: currentVersion,
		Timeout:        cfg.Timeout,
	}

	// Run preflight checks with retry
	logger.V(ucplog.LevelDebug).Info("Starting preflight checks",
		"targetVersion", cfg.TargetVersion,
		"currentVersion", currentVersion,
		"checks", cfg.EnabledChecks,
		"retryAttempts", cfg.RetryAttempts)

	err = retryWithBackoff(ctx, cfg.RetryAttempts, cfg.RetryDelay, logger, func() error {
		return preupgrade.RunPreflightChecks(ctx, preflightConfig, options)
	})

	if err != nil {
		logger.Error(err, "Preflight checks failed")
		return err
	}

	logger.Info("Preflight checks completed successfully")
	return nil
}

// parseConfig parses configuration from environment variables
func parseConfig() Config {
	return Config{
		TargetVersion: getEnvString("TARGET_VERSION", ""),
		EnabledChecks: parseEnabledChecks(),
		Timeout:       getEnvDuration("PREFLIGHT_TIMEOUT_SECONDS", 1*time.Minute),
		RetryAttempts: getEnvInt("RETRY_ATTEMPTS", 1),
		RetryDelay:    getEnvDuration("RETRY_DELAY_SECONDS", 2*time.Second),
	}
}

// parseEnabledChecks parses the enabled checks from environment
func parseEnabledChecks() []string {
	checksEnv := os.Getenv("ENABLED_CHECKS")
	if checksEnv == "" {
		// Default checks if not specified
		return []string{"version", "helm", "installation", "kubernetes"}
	}

	var checks []string
	for _, check := range strings.Split(checksEnv, ",") {
		check = strings.TrimSpace(check)
		if check != "" {
			checks = append(checks, check)
		}
	}
	return checks
}

// getCurrentVersion retrieves the current Radius version from the cluster
func getCurrentVersion(config preupgrade.Config, logger logr.Logger) string {
	state, err := config.Helm.CheckRadiusInstall(config.KubeContext)
	if err != nil {
		logger.Error(err, "Failed to detect current Radius version")
		return "unknown"
	}
	if !state.RadiusInstalled {
		return "not-installed"
	}
	return state.RadiusVersion
}

// retryWithBackoff executes a function with retry logic
func retryWithBackoff(ctx context.Context, attempts int, delay time.Duration, logger logr.Logger, fn func() error) error {
	var lastErr error

	for i := 1; i <= attempts; i++ {
		if i > 1 {
			logger.Info("Retrying preflight checks", "attempt", i, "maxAttempts", attempts)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			if i < attempts {
				logger.V(ucplog.LevelDebug).Info("Attempt failed, will retry", "attempt", i, "error", err)
			}
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", attempts, lastErr)
}

// Helper functions for parsing environment variables

func getEnvString(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}

	// Handle seconds as integer (for backward compatibility)
	if seconds, err := strconv.Atoi(v); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as duration string
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}

	return defaultValue
}

// logrOutputWriter adapts logr.Logger to output.Interface
type logrOutputWriter struct {
	logger logr.Logger
}

func (l *logrOutputWriter) LogInfo(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	l.logger.Info(msg)
}

func (l *logrOutputWriter) WriteFormatted(format string, obj any, options output.FormatterOptions) error {
	// For pre-upgrade, we don't need formatted output
	return nil
}

func (l *logrOutputWriter) BeginStep(format string, v ...any) output.Step {
	msg := fmt.Sprintf(format, v...)
	l.logger.Info(msg)
	return output.Step{}
}

func (l *logrOutputWriter) CompleteStep(step output.Step) {
	// No-op for pre-upgrade
}
