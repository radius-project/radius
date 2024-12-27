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
func NewNoOpRetryer() *Retryer {
	b := retry.NewConstant(1 * time.Second)
	b = retry.WithMaxRetries(0, b)

	noOpRetryer := NewRetryer(&RetryConfig{
		BackoffStrategy: b,
	})

	return noOpRetryer
}

func DefaultBackoffStrategy() retry.Backoff {
	b := retry.NewExponential(1 * time.Second)
	b = retry.WithMaxDuration(defaultMaxDuration, b)
	b = retry.WithMaxRetries(defaultMaxRetries, b)

	return b
}

func NewDefaultRetryer() *Retryer {
	defaultRetryer := NewRetryer(&RetryConfig{
		BackoffStrategy: DefaultBackoffStrategy(),
	})

	return defaultRetryer
}

// NewRetryer creates a new Retryer with the given configuration.
func NewRetryer(config *RetryConfig) *Retryer {
	retryConfig := &RetryConfig{}

	if config != nil {
		if config.BackoffStrategy != nil {
			retryConfig.BackoffStrategy = config.BackoffStrategy
		}
	} else {
		retryConfig.BackoffStrategy = retry.NewExponential(defaultInterval)
	}

	return &Retryer{
		config: retryConfig,
	}
}

// RetryFunc retries the given function until it returns nil or the maximum number of retries is reached.
func (r *Retryer) RetryFunc(ctx context.Context, f func(ctx context.Context) error) error {
	return retry.Do(ctx, r.config.BackoffStrategy, f)
}

// RetryableError marks an error as retryable.
func RetryableError(err error) error {
	return retry.RetryableError(err)
}
