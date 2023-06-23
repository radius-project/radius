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

package terraform

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	install "github.com/hashicorp/hc-install"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func TestInstall(t *testing.T) {
	// Create a temporary test directory
	testDir, err := ioutil.TempDir("", "testTF")
	require.NoError(t, err, "Failed to create temporary directory")
	defer os.RemoveAll(testDir)

	ctx := createContext(t)
	i := install.NewInstaller()
	defer func() {
		err := i.Remove(ctx)
		require.NoError(t, err, "Failed to remove Terraform installation")
	}()

	execPath, err := Install(ctx, i, testDir)
	require.NoError(t, err, "Terraform installation failed")
	require.NotEmpty(t, execPath, "Install returned empty execPath")
	require.FileExists(t, execPath, "Install did not create executable at %q", execPath)
	installDir := filepath.Join(testDir, installSubDir)
	require.DirExists(t, installDir, "Install did not create installation directory at %q", installDir)
}
