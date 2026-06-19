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

package progress

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

// waitTimeout bounds how long the teatest helpers wait for rendered output.
const waitTimeout = 5 * time.Second

func Test_model_rendersSpinnerThenFinalState(t *testing.T) {
	updates := make(chan clients.ResourceProgress, 1)
	tm := teatest.NewTestModel(t, newModel(updates), teatest.WithInitialTermSize(150, 40))

	id, err := resources.ParseResource("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/containers/test-container-0")
	require.NoError(t, err)

	waitFor := func(substr string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
			return strings.Contains(ansi.Strip(string(bts)), substr)
		}, teatest.WithDuration(waitTimeout))
	}

	// A started resource should render with the spinner; assert on the resource name.
	updates <- clients.ResourceProgress{Resource: id, Status: clients.StatusStarted}
	waitFor("test-container-0")

	// Completing the resource should replace the spinner with the final-state token.
	updates <- clients.ResourceProgress{Resource: id, Status: clients.StatusCompleted}
	waitFor(output.ProgressCompleted)

	// Closing the channel makes the model quit.
	close(updates)
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
}

func Test_model_failedStateWithoutStart(t *testing.T) {
	updates := make(chan clients.ResourceProgress, 1)
	tm := teatest.NewTestModel(t, newModel(updates), teatest.WithInitialTermSize(150, 40))

	id, err := resources.ParseResource("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/containers/test-container-1")
	require.NoError(t, err)

	// A resource can move directly to a terminal state without being started first.
	updates <- clients.ResourceProgress{Resource: id, Status: clients.StatusFailed}
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(ansi.Strip(string(bts)), output.ProgressFailed)
	}, teatest.WithDuration(waitTimeout))

	close(updates)
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
}
