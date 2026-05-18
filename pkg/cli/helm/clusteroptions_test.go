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

package helm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ClusterOptions_logf_UsesCustomLogger(t *testing.T) {
	var captured string
	options := ClusterOptions{
		Logger: func(format string, v ...any) {
			// Capture the raw format/args so we can assert that logf forwards
			// them verbatim to the custom logger instead of routing them
			// through output.LogInfo.
			captured = format
			require.Equal(t, []any{"value"}, v)
		},
	}

	options.logf("hello %s", "value")

	require.Equal(t, "hello %s", captured)
}

func Test_ClusterOptions_logf_FallsBackWhenLoggerNil(t *testing.T) {
	// When Logger is nil, logf should fall back to output.LogInfo without
	// panicking. We don't assert on stdout here (other tests cover
	// output.LogInfo); we just ensure the nil-Logger branch is exercised
	// safely.
	options := ClusterOptions{}

	require.NotPanics(t, func() {
		options.logf("hello %s", "world")
	})
}
