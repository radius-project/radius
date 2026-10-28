/*
Copyright 2026 The Radius Authors.

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

package hostoptions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigWorkerServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("workerServer:\n  maxOperationConcurrency: 3\n  dequeueInterval: 2s\n"), 0o600))

	config, err := loadConfig(path)

	require.NoError(t, err)
	require.Equal(t, 3, *config.WorkerServer.MaxOperationConcurrency)
	require.Equal(t, 2*time.Second, *config.WorkerServer.DequeueInterval)
	require.Nil(t, config.WorkerServer.MaxOperationRetryCount)
}
