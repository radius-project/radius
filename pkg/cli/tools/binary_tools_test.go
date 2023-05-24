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

package tools

import (
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/version"
)

func TestGetDownloadURI(t *testing.T) {
	got, err := GetDownloadURI("%s/%s/%s", "test-bin")
	require.NoError(t, err)

	platform, err := GetValidPlatform(runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err, "GetValidPlatform() error = %v", err)
	want := fmt.Sprintf("%s/%s/test-bin", version.Channel(), platform)

	require.Equal(t, want, got, "GetDownloadURI() got = %v, want %v", got, want)
}

func TestGetValidPlatform(t *testing.T) {
	osArchTests := []struct {
		currentOS   string
		currentArch string
		out         string
		err         error
	}{
		{
			currentOS:   "darwin",
			currentArch: "amd64",
			out:         "macos-x64",
		},
		{
			currentOS:   "darwin",
			currentArch: "arm64",
			out:         "macos-arm64",
		},
		{
			currentOS:   "windows",
			currentArch: "amd64",
			out:         "windows-x64",
		},
		{
			currentOS:   "windows",
			currentArch: "arm64",
			out:         "",
			err:         errors.New("unsupported platform windows/arm64"),
		},
		{
			currentOS:   "linux",
			currentArch: "amd64",
			out:         "linux-x64",
		},
		{
			currentOS:   "linux",
			currentArch: "arm",
			out:         "linux-arm",
		},
		{
			currentOS:   "linux",
			currentArch: "arm64",
			out:         "linux-arm64",
		},
	}

	for _, tc := range osArchTests {
		t.Run(tc.currentOS+"-"+tc.currentArch, func(t *testing.T) {
			platform, err := GetValidPlatform(tc.currentOS, tc.currentArch)
			if tc.err != nil {
				require.ErrorContains(t, err, err.Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.out, platform, "GetValidPlatform() got = %v, want %v", platform, tc.out)
		})
	}
}
