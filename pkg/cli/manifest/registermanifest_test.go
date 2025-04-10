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

package manifest

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	// armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	supportsRecipeCapability = "SupportsRecipe"
)

func TestRegisterDirectory(t *testing.T) {
	tests := []struct {
		name                     string
		planeName                string
		directoryPath            string
		expectError              bool
		expectedErrorMessage     string
		expectedResourceProvider string
	}{
		{
			name:                     "Success",
			planeName:                "local",
			directoryPath:            "testdata/registerdirectory",
			expectError:              false,
			expectedErrorMessage:     "",
			expectedResourceProvider: "MyCompany2.CompanyName2",
		},
		{
			name:                     "EmptyDirectoryPath",
			planeName:                "local",
			directoryPath:            "",
			expectError:              true,
			expectedErrorMessage:     "invalid manifest directory",
			expectedResourceProvider: "",
		},
		{
			name:                     "InvalidDirectoryPath",
			planeName:                "local",
			directoryPath:            "#^$/invalid",
			expectError:              true,
			expectedErrorMessage:     "failed to access manifest path #^$/invalid: stat #^$/invalid: no such file or directory",
			expectedResourceProvider: "",
		},
		{
			name:                     "FilePathInsteadOfDirectory",
			planeName:                "local",
			directoryPath:            "testdata/valid.yaml",
			expectError:              true,
			expectedErrorMessage:     "manifest path testdata/valid.yaml is not a directory",
			expectedResourceProvider: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory, err := NewTestClientFactory(WithResourceProviderServerNoError)
			require.NoError(t, err)

			err = RegisterDirectory(context.Background(), clientFactory, tt.planeName, tt.directoryPath, nil)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				// Verify the expected resource provider exists
				if tt.expectedResourceProvider != "" {
					rp, err := clientFactory.NewResourceProvidersClient().Get(context.Background(), tt.planeName, tt.expectedResourceProvider, nil)
					require.NoError(t, err, "Failed to retrieve the expected resource provider")
					require.Equal(t, to.Ptr(tt.expectedResourceProvider), rp.Name)
				}
			}
		})
	}
}

func TestRegisterFile(t *testing.T) {
	tests := []struct {
		name                     string
		planeName                string
		filePath                 string
		expectError              bool
		expectedErrorMessage     string
		expectedResourceProvider string
		expectedResourceTypeName string
		expectedAPIVersion       string
	}{
		{
			name:                     "Success",
			planeName:                "local",
			filePath:                 "testdata/registerdirectory/resourceprovider-valid2.yaml",
			expectError:              false,
			expectedErrorMessage:     "",
			expectedResourceProvider: "MyCompany2.CompanyName2",
			expectedResourceTypeName: "testResource3",
			expectedAPIVersion:       "2025-01-01-preview",
		},
		{
			name:                     "EmptyDirectoryPath",
			planeName:                "local",
			filePath:                 "",
			expectError:              true,
			expectedErrorMessage:     "invalid manifest file path",
			expectedResourceProvider: "",
			expectedResourceTypeName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			clientFactory, err := NewTestClientFactory(WithResourceProviderServerNoError)
			require.NoError(t, err, "Failed to create client factory")

			var logBuffer bytes.Buffer
			logger := func(format string, args ...interface{}) {
				fmt.Fprintf(&logBuffer, format+"\n", args...)
			}

			err = RegisterFile(context.Background(), clientFactory, tt.planeName, tt.filePath, logger)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				if tt.expectedResourceProvider != "" {
					rp, err := clientFactory.NewResourceProvidersClient().Get(context.Background(), tt.planeName, tt.expectedResourceProvider, nil)
					require.NoError(t, err, "Failed to retrieve the expected resource provider")
					require.Equal(t, to.Ptr(tt.expectedResourceProvider), rp.Name)

					logOutput := logBuffer.String()
					require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s", tt.expectedResourceProvider, tt.expectedResourceTypeName))
					require.Contains(t, logOutput, fmt.Sprintf("Creating API Version %s/%s@%s", tt.expectedResourceProvider, tt.expectedResourceTypeName, tt.expectedAPIVersion))
				}
			}
		})
	}
}

func TestRegisterType(t *testing.T) {
	tests := []struct {
		name                     string
		planeName                string
		resourceProviderName     string
		resourceTypeName         string
		filePath                 string
		expectError              bool
		expectedErrorMessage     string
		expectedResourceProvider string
		expectedResourceTypeName string
		expectedAPIVersion       string
	}{
		{
			name:                     "Success",
			planeName:                "local",
			resourceProviderName:     "MyCompany2.CompanyName2",
			resourceTypeName:         "testResource3",
			filePath:                 "testdata/registerdirectory/resourceprovider-valid2.yaml",
			expectError:              false,
			expectedErrorMessage:     "",
			expectedResourceProvider: "MyCompany2.CompanyName2",
			expectedResourceTypeName: "testResource3",
			expectedAPIVersion:       "2025-01-01-preview",
		},
		{
			name:                     "ResourceTypeNotFound",
			planeName:                "local",
			resourceProviderName:     "MyCompany2.CompanyName2",
			resourceTypeName:         "testResource5",
			filePath:                 "testdata/registerdirectory/resourceprovider-valid2.yaml",
			expectError:              true,
			expectedErrorMessage:     "type testResource5 not found in manifest file testdata/registerdirectory/resourceprovider-valid2.yaml",
			expectedResourceProvider: "",
			expectedResourceTypeName: "",
		},
		{
			name:                     "EmptyFilePath",
			planeName:                "local",
			resourceProviderName:     "MyCompany2.CompanyName2",
			resourceTypeName:         "testResource3",
			filePath:                 "",
			expectError:              true,
			expectedErrorMessage:     "invalid manifest file path",
			expectedResourceProvider: "",
			expectedResourceTypeName: "",
		},
	}

	// Run the tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			clientFactory, err := NewTestClientFactory(WithResourceProviderServerNoError)
			require.NoError(t, err, "Failed to create client factory")

			var logBuffer bytes.Buffer
			logger := func(format string, args ...interface{}) {
				fmt.Fprintf(&logBuffer, format+"\n", args...)
			}

			err = RegisterType(context.Background(), clientFactory, tt.planeName, tt.filePath, tt.resourceTypeName, logger)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				// Verify the expected resource provider exists
				if tt.expectedResourceProvider != "" {
					rp, err := clientFactory.NewResourceProvidersClient().Get(context.Background(), tt.planeName, tt.expectedResourceProvider, nil)
					require.NoError(t, err, "Failed to retrieve the expected resource provider")
					require.Equal(t, to.Ptr(tt.expectedResourceProvider), rp.Name)

					logOutput := logBuffer.String()
					require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s with capabilities %s", tt.expectedResourceProvider, tt.expectedResourceTypeName, supportsRecipeCapability))
					require.Contains(t, logOutput, fmt.Sprintf("Creating API Version %s/%s@%s", tt.expectedResourceProvider, tt.expectedResourceTypeName, tt.expectedAPIVersion))
				}
			}
		})
	}
}
func TestRetryOperation(t *testing.T) {
	tests := []struct {
		name          string
		operation     func() error
		attempts      int
		expectedError string
	}{
		{
			name: "success on first attempt",
			operation: func() error {
				// No retries needed; always succeeds.
				return nil
			},
			attempts: 1,
		},
		{
			name: "success after retry",
			// Return a closure that keeps track of how many times it's invoked.
			// The first call returns 409, the second returns nil.
			operation: (func() func() error {
				var attempt int
				return func() error {
					attempt++
					if attempt == 1 {
						return &azcore.ResponseError{StatusCode: 409}
					}
					return nil
				}
			})(),
			attempts: 2,
		},
		{
			name: "non-409 error",
			operation: func() error {
				// Will fail immediately, no retry.
				return fmt.Errorf("non-409 error")
			},
			attempts:      1,
			expectedError: "non-409 error",
		},
		{
			name: "verify increasing backoff",
			// Test that each retry is spaced out longer than the previous one.
			operation: (func() func() error {
				var lastTime time.Time
				var lastDuration time.Duration
				var attempt int
				return func() error {
					now := time.Now()
					if attempt > 0 {
						// Measure how long since last invocation
						currentDuration := now.Sub(lastTime)
						if currentDuration <= lastDuration {
							return fmt.Errorf("backoff did not increase: previous %v, current %v",
								lastDuration, currentDuration)
						}
						lastDuration = currentDuration
					}
					lastTime = now

					attempt++
					// Force 409 until the third attempt
					if attempt < 3 {
						return &azcore.ResponseError{StatusCode: 409}
					}
					return nil
				}
			})(),
			attempts: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We'll capture log output here
			var logBuffer bytes.Buffer
			logger := func(format string, args ...any) {
				fmt.Fprintf(&logBuffer, format+"\n", args...)
			}

			var actualAttempts int

			// wrappedOp is what's passed to retryOperation().
			// Each retry calls this, so we increment actualAttempts each time.
			wrappedOp := func() error {
				actualAttempts++
				return tt.operation()
			}

			err := retryOperation(context.Background(), wrappedOp, logger)

			if tt.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.attempts, actualAttempts, "unexpected number of attempts")

			// If more than 1 attempt, we expect conflict logs.
			if tt.attempts > 1 {
				logContent := logBuffer.String()
				require.Contains(t, logContent, "Got 409 conflict on attempt")

				lines := strings.Split(strings.TrimSpace(logContent), "\n")
				// We'll see one log line per retry. E.g. if attempts=3, that means 2 retries logged.
				require.Equal(t, tt.attempts-1, len(lines), "expected retry log messages don't match attempts")
			}
		})
	}
}

func TestRetryOperationWithContext(t *testing.T) {
	tests := []struct {
		name          string
		operation     func() error
		setupCtx      func() context.Context
		attempts      int
		expectedError string
	}{
		{
			name: "context cancelled",
			operation: func() error {
				return &azcore.ResponseError{StatusCode: 409}
			},
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx
			},
			attempts:      1,
			expectedError: "context canceled",
		},
		{
			name: "context timeout",
			operation: func() error {
				return &azcore.ResponseError{StatusCode: 409}
			},
			setupCtx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				// Ensure cancel is called after context is done
				go func() {
					<-ctx.Done()
					cancel()
				}()
				return ctx
			},
			attempts:      1,
			expectedError: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			logger := func(format string, args ...any) {
				fmt.Fprintf(&logBuffer, format+"\n", args...)
			}

			actualAttempts := 0
			wrappedOp := func() error {
				actualAttempts++
				return tt.operation()
			}

			ctx := tt.setupCtx()
			err := retryOperation(ctx, wrappedOp, logger)

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedError)
			require.Equal(t, tt.attempts, actualAttempts, "unexpected number of attempts")
		})
	}
}

func TestIs409ConflictError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "simple 409 conflict",
			err: &azcore.ResponseError{
				StatusCode: 409,
			},
			want: true,
		},
		{
			name: "409 error with code=Conflict",
			err: &azcore.ResponseError{
				StatusCode: 409,
				ErrorCode:  "Conflict",
			},
			want: true,
		},
		{
			name: "different status code (404)",
			err: &azcore.ResponseError{
				StatusCode: 404,
			},
			want: false,
		},
		{
			name: "non-ResponseError type",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
		{
			name: "wrapped 409 error",
			err:  fmt.Errorf("wrapped: %w", &azcore.ResponseError{StatusCode: 409}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := is409ConflictError(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}
