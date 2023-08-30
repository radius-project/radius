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

package v20220315privatepreview

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TODO: To be moved to a common package armrpc/api/v1
func TestUnmarshalTimeString(t *testing.T) {
	parsedTime := UnmarshalTimeString("2021-09-24T19:09:00.000000Z")
	require.NotNil(t, parsedTime)

	require.Equal(t, 2021, parsedTime.Year())
	require.Equal(t, time.Month(9), parsedTime.Month())
	require.Equal(t, 24, parsedTime.Day())

	parsedTime = UnmarshalTimeString("")
	require.NotNil(t, parsedTime)
	require.Equal(t, 1, parsedTime.Year())
}
