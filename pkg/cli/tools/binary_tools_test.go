// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

import (
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
