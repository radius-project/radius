// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"

	"github.com/Azure/radius/pkg/version"
)

func Binary(ctx context.Context, toolName string, destinationPath string) error {
	uri, err := getDownloadURI(toolName, path.Base(destinationPath))
	if err != nil {
		return err
	}

	c := http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %v", toolName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download %s from '%s'with status code: %d", toolName, uri, resp.StatusCode)
	}

	// create folders
	err = os.MkdirAll(path.Dir(destinationPath), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create folder %s: %v", path.Dir(destinationPath), err)
	}

	// will truncate the file if it exists
	out, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", destinationPath, err)
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %v", destinationPath, err)
	}

	// get the filemode so we can mark it as executable
	file, err := out.Stat()
	if err != nil {
		return fmt.Errorf("failed to read file attributes %s: %v", destinationPath, err)
	}

	// make file executable by everyone
	err = out.Chmod(file.Mode() | 0111)
	if err != nil {
		return fmt.Errorf("failed to change permissons for %s: %v", destinationPath, err)
	}

	return nil
}

// Placeholders are for: tool name, channel, platform, filename
const downloadURIFmt = "https://radiuspublic.blob.core.windows.net/tools/%s/%s/%s/%s"

func getDownloadURI(toolName, executableName string) (string, error) {
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf(downloadURIFmt, toolName, version.Channel(), "macos-x64", executableName), nil
	} else if runtime.GOOS == "linux" {
		return fmt.Sprintf(downloadURIFmt, toolName, version.Channel(), "linux-x64", executableName), nil
	} else if runtime.GOOS == "windows" {
		return fmt.Sprintf(downloadURIFmt, toolName, version.Channel(), "windows-x64", executableName), nil
	} else {
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}
