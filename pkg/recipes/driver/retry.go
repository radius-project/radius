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

package driver

import "time"

const (
	defaultRetryCount = 20
	defaultRetryDelay = time.Minute * 1
)

// DeleteRetryConfig represents the configuration for retrying a deletion request.
type DeleteRetryConfig struct {
	// RetryCount is the number of times to retry the request.
	RetryCount int

	// RetryDelay is the delay between retries.
	RetryDelay time.Duration
}

// NewDefaultDeleteRetryConfig creates a new DeleteRetryConfig with default values.
func NewDefaultDeleteRetryConfig() DeleteRetryConfig {
	return DeleteRetryConfig{
		RetryCount: defaultRetryCount,
		RetryDelay: defaultRetryDelay,
	}
}
