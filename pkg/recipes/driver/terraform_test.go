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

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func TestCleanup(t *testing.T) {
	// Create a temporary test directory
	testDir, err := ioutil.TempDir("", "test_dir")
	if err != nil {
		t.Errorf("Failed to create temporary directory: %v", err)
	}

	// Define test inputs
	ctx := createContext(t)

	// Execute the cleanup function
	cleanup(ctx, testDir)

	// Verify cleanup
	_, err = os.Stat(testDir)
	if !os.IsNotExist(err) {
		os.RemoveAll(testDir) // Clean up the temporary test directory manually
		t.Errorf("Expected directory %s to be removed, but it still exists", testDir)
	}
}
