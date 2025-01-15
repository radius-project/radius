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
	"testing"

	// armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"

	"github.com/radius-project/radius/pkg/to"
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

				// Verify the expected resource provider exists
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
			expectedErrorMessage:     "Type testResource5 not found in manifest file testdata/registerdirectory/resourceprovider-valid2.yaml",
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
					require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s", tt.expectedResourceProvider, tt.expectedResourceTypeName))
					require.Contains(t, logOutput, fmt.Sprintf("Creating API Version %s/%s@%s", tt.expectedResourceProvider, tt.expectedResourceTypeName, tt.expectedAPIVersion))
				}
			}
		})
	}
}
