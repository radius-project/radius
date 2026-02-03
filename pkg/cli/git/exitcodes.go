// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

// Package git provides functionality for Git workspace mode operations.
// Git workspace mode enables decentralized deployments without a centralized
// control plane, using a Git repository as the system of record.
package git

// Exit codes for Git workspace commands per FR-005.
// These codes enable GitHub Actions workflows to conditionally execute
// steps based on command outcomes.
const (
	// ExitSuccess indicates the command completed successfully.
	ExitSuccess = 0

	// ExitGeneralError indicates an unexpected error occurred.
	ExitGeneralError = 1

	// ExitValidationError indicates configuration or input validation failed.
	// Examples: invalid .env file, malformed Bicep, missing required flags.
	ExitValidationError = 2

	// ExitAuthError indicates an authentication or authorization failure.
	// Examples: expired credentials, missing permissions, invalid tokens.
	ExitAuthError = 3

	// ExitResourceConflict indicates a state conflict prevented the operation.
	// Examples: failed to acquire lock, concurrent modification detected.
	ExitResourceConflict = 4

	// ExitDeploymentFailure indicates a deployment operation failed.
	// Resources may need manual cleanup when this exit code is returned.
	ExitDeploymentFailure = 5
)
