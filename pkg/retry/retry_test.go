package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	goretry "github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/require"
)

func TestNewNoOpRetryer(t *testing.T) {
	retryer := NewNoOpRetryer()
	require.NotNil(t, retryer)
	require.NotNil(t, retryer.config)
	require.NotNil(t, retryer.config.BackoffStrategy)

	expectedBackoffStrategy := goretry.NewConstant(time.Second * 1)
	expectedBackoffStrategy = goretry.WithMaxRetries(0, expectedBackoffStrategy)

	require.IsType(t, expectedBackoffStrategy, retryer.config.BackoffStrategy)
}

func TestDefaultBackoffStrategy(t *testing.T) {
	backoff := DefaultBackoffStrategy()
	require.NotNil(t, backoff)
}

func TestNewDefaultRetryer(t *testing.T) {
	retryer := NewDefaultRetryer()
	require.NotNil(t, retryer)
	require.NotNil(t, retryer.config)
	require.NotNil(t, retryer.config.BackoffStrategy)

	expectedBackoffStrategy := goretry.NewConstant(time.Second * 1)
	expectedBackoffStrategy = goretry.WithMaxRetries(0, expectedBackoffStrategy)

	require.IsType(t, expectedBackoffStrategy, retryer.config.BackoffStrategy)
}

func TestNewRetryer(t *testing.T) {
	config := &RetryConfig{
		BackoffStrategy: goretry.NewConstant(1 * time.Second),
	}
	retryer := NewRetryer(config)
	require.NotNil(t, retryer)
	require.NotNil(t, retryer.config)

	retryer = NewRetryer(nil)
	require.NotNil(t, retryer)
	require.NotNil(t, retryer.config)
	require.NotNil(t, retryer.config.BackoffStrategy)
}

func TestRetryer_RetryFunc(t *testing.T) {
	retryer := NewDefaultRetryer()
	ctx := context.Background()

	// Test successful function
	err := retryer.RetryFunc(ctx, func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)

	// Test retryable error
	retryCount := 0
	err = retryer.RetryFunc(ctx, func(ctx context.Context) error {
		retryCount++
		if retryCount < 3 {
			return RetryableError(errors.New("retryable error"))
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, retryCount)

	// Test non-retryable error
	err = retryer.RetryFunc(ctx, func(ctx context.Context) error {
		return errors.New("non-retryable error")
	})
	require.Error(t, err)
}
