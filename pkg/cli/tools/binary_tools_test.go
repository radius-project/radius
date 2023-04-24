// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	got, gotErr := GetDownloadURI("%s/%s/%s", "test-bin")
	var want string
	var wantErr bool

	switch runtime.GOOS {
	case "darwin":
		want = fmt.Sprint(version.Channel(), "/macos-x64/test-bin")
	case "linux", "windows":
		want = fmt.Sprintf("%s/%s-x64/test-bin", version.Channel(), runtime.GOOS)
	default:
		wantErr = true
	}

	if wantErr {
		wantErr := fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
		require.ErrorIs(t, wantErr, gotErr, "GetDownloadURI() error = %v, wantErr %v", gotErr, wantErr)
	}
	require.Equal(t, want, got, "GetDownloadURI() got = %v, wantErr %v", got, want)
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
