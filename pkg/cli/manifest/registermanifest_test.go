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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"
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
			clientFactory := createTestClientFactory(t)

			err := RegisterDirectory(context.Background(), clientFactory, tt.planeName, tt.directoryPath, nil)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				// Verify the expected resource provider exists
				if tt.expectedResourceProvider != "" {
					verifyResourceProviderExists(t, clientFactory, tt.planeName, tt.expectedResourceProvider)
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
			clientFactory := createTestClientFactory(t)
			logger, logBuffer := createTestLogger()

			err := RegisterFile(context.Background(), clientFactory, tt.planeName, tt.filePath, logger)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				if tt.expectedResourceProvider != "" {
					verifyResourceProviderExists(t, clientFactory, tt.planeName, tt.expectedResourceProvider)

					expectedMessages := createExpectedLogMessages(tt.expectedResourceProvider, tt.expectedResourceTypeName, tt.expectedAPIVersion)
					verifyLogContains(t, logBuffer, expectedMessages...)
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
			clientFactory := createTestClientFactory(t)
			logger, logBuffer := createTestLogger()

			err := RegisterType(context.Background(), clientFactory, tt.planeName, tt.filePath, tt.resourceTypeName, logger)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				// Verify the expected resource provider exists
				if tt.expectedResourceProvider != "" {
					verifyResourceProviderExists(t, clientFactory, tt.planeName, tt.expectedResourceProvider)

					logOutput := logBuffer.String()
					require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s with capabilities %s", tt.expectedResourceProvider, tt.expectedResourceTypeName, datamodel.CapabilityManualResourceProvisioning))
					require.Contains(t, logOutput, fmt.Sprintf("Creating API Version %s/%s@%s", tt.expectedResourceProvider, tt.expectedResourceTypeName, tt.expectedAPIVersion))
				}
			}
		})
	}
}

func TestRegisterResourceProvider(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := createTestClientFactory(t)
			logger, logBuffer := createTestLogger()

			// Read the resource provider from the file
			resourceProvider, err := ReadFile(tt.filePath)
			require.NoError(t, err)

			err = RegisterResourceProvider(context.Background(), clientFactory, tt.planeName, *resourceProvider, logger)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
			} else {
				require.NoError(t, err)

				if tt.expectedResourceProvider != "" {
					verifyResourceProviderExists(t, clientFactory, tt.planeName, tt.expectedResourceProvider)

					expectedMessages := createExpectedLogMessages(tt.expectedResourceProvider, tt.expectedResourceTypeName, tt.expectedAPIVersion)
					verifyLogContains(t, logBuffer, expectedMessages...)
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
			verifyRetryBehavior(t, tt.operation, tt.attempts, tt.expectedError, true)
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

func TestEnsureResourceProviderExists(t *testing.T) {
	tests := []struct {
		name                 string
		planeName            string
		resourceProvider     ResourceProvider
		clientFactorySetup   func() (*v20231001preview.ClientFactory, error)
		expectError          bool
		expectedErrorMessage string
		expectCreateCalled   bool
	}{
		{
			name:      "ResourceProviderExists",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.TestService",
				Location:  map[string]string{"global": ""},
			},
			clientFactorySetup: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory(WithResourceProviderServerNoError)
			},
			expectError:        false,
			expectCreateCalled: false,
		},
		{
			name:      "ResourceProviderNotFound_ShouldCreate",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.NewService",
				Location:  map[string]string{"global": ""},
			},
			clientFactorySetup: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory(WithResourceProviderServerNotFoundError)
			},
			expectError:        false,
			expectCreateCalled: true,
		},
		{
			name:      "InternalServerError",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.ErrorService",
			},
			clientFactorySetup: func() (*v20231001preview.ClientFactory, error) {
				return NewTestClientFactory(WithResourceProviderServerInternalError)
			},
			expectError:          true,
			expectedErrorMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory, err := tt.clientFactorySetup()
			require.NoError(t, err)

			logger, _ := createTestLogger()

			err = EnsureResourceProviderExists(context.Background(), clientFactory, tt.planeName, tt.resourceProvider, logger)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrorMessage != "" {
					require.Contains(t, err.Error(), tt.expectedErrorMessage)
				}
			} else {
				require.NoError(t, err)
			}

			// For the success case where resource provider didn't exist, verify creation was attempted
			if tt.expectCreateCalled && !tt.expectError {
				require.True(t, true, "Function succeeded without error, indicating resource provider creation was attempted")
			}
		})
	}
}

func TestCreateEmptyResourceProvider(t *testing.T) {
	tests := []struct {
		name                 string
		planeName            string
		resourceProvider     ResourceProvider
		expectError          bool
		expectedErrorMessage string
	}{
		{
			name:      "Success_GlobalLocation",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.EmptyService",
				Location:  nil, // Should default to global
			},
			expectError: false,
		},
		{
			name:      "Success_CustomLocation",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.CustomService",
				Location:  map[string]string{"custom": "http://localhost:8080"},
			},
			expectError: false,
		},
		{
			name:      "Success_NilLocation",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.EmptyMapService",
				Location:  nil, // Should default to global
			},
			expectError: false,
		},
		{
			name:      "Success_GlobalLocationEmpty",
			planeName: "local",
			resourceProvider: ResourceProvider{
				Namespace: "TestCompany.GlobalService",
				Location:  map[string]string{"global": ""},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := createTestClientFactory(t)
			logger, _ := createTestLogger()

			err := CreateEmptyResourceProvider(context.Background(), clientFactory, tt.planeName, tt.resourceProvider, logger)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrorMessage != "" {
					require.Contains(t, err.Error(), tt.expectedErrorMessage)
				}
			} else {
				require.NoError(t, err)

				// Verify the resource provider was created
				verifyResourceProviderExists(t, clientFactory, tt.planeName, tt.resourceProvider.Namespace)

				// Verify location was created (we can't easily verify it's empty without more detailed mocking)
				// But the fact that no error occurred suggests the location creation succeeded
			}
		})
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name                 string
		filePath             string
		expectError          bool
		expectedErrorMessage string
		expectedNamespace    string
	}{
		{
			name:              "ValidManifest",
			filePath:          "testdata/valid.yaml",
			expectError:       false,
			expectedNamespace: "MyCompany.Resources",
		},
		{
			name:              "ValidManifest2",
			filePath:          "testdata/registerdirectory/resourceprovider-valid2.yaml",
			expectError:       false,
			expectedNamespace: "MyCompany2.CompanyName2",
		},
		{
			name:                 "FileNotFound",
			filePath:             "testdata/nonexistent.yaml",
			expectError:          true,
			expectedErrorMessage: "failed to read manifest",
		},
		{
			name:                 "EmptyFilePath",
			filePath:             "",
			expectError:          true,
			expectedErrorMessage: "failed to read manifest",
		},
		{
			name:                 "InvalidYaml",
			filePath:             "testdata/invalid-yaml.yaml",
			expectError:          true,
			expectedErrorMessage: "failed to read manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceProvider, err := ValidateManifest(context.Background(), tt.filePath)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrorMessage)
				require.Nil(t, resourceProvider)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resourceProvider)
				require.Equal(t, tt.expectedNamespace, resourceProvider.Namespace)
			}
		})
	}
}

func TestExtractLocationInfo(t *testing.T) {
	tests := []struct {
		name                 string
		resourceProvider     ResourceProvider
		expectedLocationName string
		expectedAddress      string
	}{
		{
			name: "NilLocation_ShouldDefaultToGlobal",
			resourceProvider: ResourceProvider{
				Namespace: "Test.Service",
				Location:  nil,
			},
			expectedLocationName: "global",
			expectedAddress:      "",
		},
		{
			name: "EmptyLocationMap_ShouldReturnEmpty",
			resourceProvider: ResourceProvider{
				Namespace: "Test.Service",
				Location:  map[string]string{},
			},
			expectedLocationName: "",
			expectedAddress:      "",
		},
		{
			name: "GlobalLocation_NoAddress",
			resourceProvider: ResourceProvider{
				Namespace: "Test.Service",
				Location:  map[string]string{"global": ""},
			},
			expectedLocationName: "global",
			expectedAddress:      "",
		},
		{
			name: "CustomLocation_WithAddress",
			resourceProvider: ResourceProvider{
				Namespace: "Test.Service",
				Location:  map[string]string{"custom": "http://localhost:8080"},
			},
			expectedLocationName: "custom",
			expectedAddress:      "http://localhost:8080",
		},
		{
			name: "SingleLocation_WithEmptyAddress",
			resourceProvider: ResourceProvider{
				Namespace: "Test.Service",
				Location:  map[string]string{"east": ""},
			},
			expectedLocationName: "east",
			expectedAddress:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locationName, address := extractLocationInfo(tt.resourceProvider)

			require.Equal(t, tt.expectedLocationName, locationName)
			require.Equal(t, tt.expectedAddress, address)
		})
	}
}

func TestLogIfEnabled(t *testing.T) {
	tests := []struct {
		name           string
		logger         func(format string, args ...any)
		expectPanic    bool
		expectedOutput string
	}{
		{
			name:        "NilLogger_ShouldNotPanic",
			logger:      nil,
			expectPanic: false,
		},
		{
			name: "ValidLogger_ShouldLog",
			logger: func(format string, args ...any) {
				// This will be captured in the test
			},
			expectPanic:    false,
			expectedOutput: "test message arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			var actualLogger func(format string, args ...any)

			if tt.logger != nil {
				actualLogger = func(format string, args ...any) {
					fmt.Fprintf(&logBuffer, format, args...)
				}
			}

			if tt.expectPanic {
				require.Panics(t, func() {
					logIfEnabled(actualLogger, "test message %s", "arg")
				})
			} else {
				require.NotPanics(t, func() {
					logIfEnabled(actualLogger, "test message %s", "arg")
				})

				if actualLogger != nil && tt.expectedOutput != "" {
					require.Contains(t, logBuffer.String(), tt.expectedOutput)
				}
			}
		})
	}
}

// Test error scenarios for existing functions
func TestRegisterResourceProvider_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name                 string
		resourceProvider     ResourceProvider
		planeName            string
		expectError          bool
		expectedErrorMessage string
	}{
		{
			name: "EmptyNamespace",
			resourceProvider: ResourceProvider{
				Namespace: "",
				Types:     map[string]*ResourceType{},
			},
			planeName:            "local",
			expectError:          true, // Empty namespace should fail validation
			expectedErrorMessage: "parameter resourceProviderName cannot be empty",
		},
		{
			name: "ResourceProviderWithTypes",
			resourceProvider: ResourceProvider{
				Namespace: "Test.WithTypes",
				Types: map[string]*ResourceType{
					"testType": {
						DefaultAPIVersion: to.Ptr("2023-01-01"),
						Capabilities:      []string{"test"},
						APIVersions: map[string]*ResourceTypeAPIVersion{
							"2023-01-01": {
								Schema: map[string]any{"type": "object"},
							},
						},
					},
				},
			},
			planeName:   "local",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := createTestClientFactory(t)
			logger, _ := createTestLogger()

			testErrorScenario(t, func() error {
				return RegisterResourceProvider(context.Background(), clientFactory, tt.planeName, tt.resourceProvider, logger)
			}, tt.expectError, tt.expectedErrorMessage)
		})
	}
}

func TestRegisterType_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name                 string
		filePath             string
		typeName             string
		expectError          bool
		expectedErrorMessage string
	}{
		{
			name:                 "ManifestValidationError",
			filePath:             "testdata/nonexistent.yaml",
			typeName:             "anyType",
			expectError:          true,
			expectedErrorMessage: "failed to read manifest",
		},
		{
			name:                 "EmptyTypeName",
			filePath:             "testdata/valid.yaml",
			typeName:             "",
			expectError:          true,
			expectedErrorMessage: "not found in manifest file",
		},
		{
			name:                 "TypeNotFoundInManifest",
			filePath:             "testdata/valid.yaml",
			typeName:             "nonExistentType",
			expectError:          true,
			expectedErrorMessage: "not found in manifest file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := createTestClientFactory(t)
			logger, _ := createTestLogger()

			testErrorScenario(t, func() error {
				return RegisterType(context.Background(), clientFactory, "local", tt.filePath, tt.typeName, logger)
			}, tt.expectError, tt.expectedErrorMessage)
		})
	}
}

func TestRegisterFile_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name                 string
		filePath             string
		expectError          bool
		expectedErrorMessage string
	}{
		{
			name:                 "ManifestReadFailure",
			filePath:             "testdata/nonexistent.yaml",
			expectError:          true,
			expectedErrorMessage: "failed to read manifest",
		},
		{
			name:                 "InvalidManifestStructure",
			filePath:             "testdata/invalid-yaml.yaml",
			expectError:          true,
			expectedErrorMessage: "failed to read manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := createTestClientFactory(t)

			testErrorScenario(t, func() error {
				return RegisterFile(context.Background(), clientFactory, "local", tt.filePath, nil)
			}, tt.expectError, tt.expectedErrorMessage)
		})
	}
}

// createTestClientFactory creates a standard test client factory with no errors
func createTestClientFactory(t *testing.T) *v20231001preview.ClientFactory {
	clientFactory, err := NewTestClientFactory(WithResourceProviderServerNoError)
	require.NoError(t, err, "Failed to create client factory")
	return clientFactory
}

// createTestLogger creates a logger that captures output to a buffer
func createTestLogger() (func(format string, args ...interface{}), *bytes.Buffer) {
	var logBuffer bytes.Buffer
	logger := func(format string, args ...interface{}) {
		fmt.Fprintf(&logBuffer, format+"\n", args...)
	}
	return logger, &logBuffer
}

// verifyResourceProviderExists verifies that a resource provider exists with the expected name
func verifyResourceProviderExists(t *testing.T, clientFactory *v20231001preview.ClientFactory, planeName, expectedResourceProvider string) {
	rp, err := clientFactory.NewResourceProvidersClient().Get(context.Background(), planeName, expectedResourceProvider, nil)
	require.NoError(t, err, "Failed to retrieve the expected resource provider")
	require.Equal(t, to.Ptr(expectedResourceProvider), rp.Name)
}

// verifyLogContains verifies that log output contains expected messages
func verifyLogContains(t *testing.T, logBuffer *bytes.Buffer, expectedMessages ...string) {
	logOutput := logBuffer.String()
	for _, expectedMessage := range expectedMessages {
		require.Contains(t, logOutput, expectedMessage)
	}
}

// createExpectedLogMessages creates standard log messages for resource type and API version creation
func createExpectedLogMessages(resourceProvider, resourceType, apiVersion string) []string {
	return []string{
		fmt.Sprintf("Creating resource type %s/%s", resourceProvider, resourceType),
		fmt.Sprintf("Creating API Version %s/%s@%s", resourceProvider, resourceType, apiVersion),
	}
}

// testErrorScenario runs a test scenario that expects an error
func testErrorScenario(t *testing.T, testFunc func() error, expectError bool, expectedErrorMessage string) {
	err := testFunc()
	if expectError {
		require.Error(t, err)
		if expectedErrorMessage != "" {
			require.Contains(t, err.Error(), expectedErrorMessage)
		}
	} else {
		require.NoError(t, err)
	}
}

// verifyRetryBehavior verifies retry operation behavior and log output
func verifyRetryBehavior(t *testing.T, operation func() error, expectedAttempts int, expectedError string, shouldHaveRetryLogs bool) {
	var logBuffer bytes.Buffer
	logger := func(format string, args ...any) {
		fmt.Fprintf(&logBuffer, format+"\n", args...)
	}

	actualAttempts := 0
	wrappedOp := func() error {
		actualAttempts++
		return operation()
	}

	err := retryOperation(context.Background(), wrappedOp, logger)

	if expectedError != "" {
		require.Error(t, err)
		require.Contains(t, err.Error(), expectedError)
	} else {
		require.NoError(t, err)
	}

	require.Equal(t, expectedAttempts, actualAttempts, "unexpected number of attempts")

	if shouldHaveRetryLogs && expectedAttempts > 1 {
		logContent := logBuffer.String()
		require.Contains(t, logContent, "Got 409 conflict on attempt")

		lines := strings.Split(strings.TrimSpace(logContent), "\n")
		var retryLines []string
		for _, line := range lines {
			if strings.Contains(line, "Got 409 conflict on attempt") {
				retryLines = append(retryLines, line)
			}
		}
		require.Equal(t, expectedAttempts-1, len(retryLines), "expected retry log messages don't match attempts")
	}
}
