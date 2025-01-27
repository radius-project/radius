package retry

import (
	"context"
	"time"

	"github.com/sethvargo/go-retry"
)

const (
	defaultInterval    = 1 * time.Second
	defaultMaxRetries  = 10
	defaultMaxDuration = 60 * time.Second
)

// RetryConfig is the configuration for a retry operation.
type RetryConfig struct {
	// BackoffStrategy is the backoff strategy to use.
	BackoffStrategy retry.Backoff
}

// Retryer is a utility for retrying functions.
type Retryer struct {
	config *RetryConfig
}

// NewNoOpRetryer creates a new Retryer that does not retry.
// This is useful for testing.
func NewNoOpRetryer() *Retryer {
	b := retry.NewConstant(1 * time.Second)
	b = retry.WithMaxRetries(0, b)

	return NewRetryer(&RetryConfig{
		BackoffStrategy: b,
	})
}

// DefaultBackoffStrategy returns the default backoff strategy.
// The default backoff strategy is an exponential backoff with a maximum duration and maximum retries.
func DefaultBackoffStrategy() retry.Backoff {
	b := retry.NewExponential(1 * time.Second)
	b = retry.WithMaxDuration(defaultMaxDuration, b)
	b = retry.WithMaxRetries(defaultMaxRetries, b)

	return b
}

// NewDefaultRetryer creates a new Retryer with the default configuration.
// The default configuration is an exponential backoff with a maximum duration and maximum retries.
func NewDefaultRetryer() *Retryer {
	return NewRetryer(&RetryConfig{
		BackoffStrategy: DefaultBackoffStrategy(),
	})
}

// NewRetryer creates a new Retryer with the given configuration.
// If either the config or config.BackoffStrategy are nil,
// the default configuration is used.
// The default configuration is an exponential backoff with a maximum duration and maximum retries.
func NewRetryer(config *RetryConfig) *Retryer {
	retryConfig := &RetryConfig{}

	if config != nil && config.BackoffStrategy != nil {
		retryConfig.BackoffStrategy = config.BackoffStrategy
	} else {
		retryConfig.BackoffStrategy = DefaultBackoffStrategy()
	}

	return &Retryer{
		config: retryConfig,
	}
}

// RetryFunc retries the given function with the backoff strategy.
func (r *Retryer) RetryFunc(ctx context.Context, f func(ctx context.Context) error) error {
	return retry.Do(ctx, r.config.BackoffStrategy, f)
}

// RetryableError marks an error as retryable.
func RetryableError(err error) error {
	return retry.RetryableError(err)
}
