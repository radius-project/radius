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

package radcli

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_tailLines(t *testing.T) {
	t.Run("returns input unchanged when within limit", func(t *testing.T) {
		out := "line1\nline2\nline3"
		require.Equal(t, out, tailLines(out, 20))
	})

	t.Run("keeps only the trailing lines and marks truncation", func(t *testing.T) {
		var b strings.Builder
		for i := 1; i <= 50; i++ {
			b.WriteString("line")
			b.WriteByte(byte('0' + i%10))
			b.WriteByte('\n')
		}
		// The transport error rad prints appears on the final line.
		b.WriteString(`Error: Get "https://127.0.0.1:37481/...": read: connection reset by peer`)

		got := tailLines(b.String(), 20)

		assert.True(t, strings.HasPrefix(got, "...(output truncated"), "expected a truncation marker prefix")
		assert.Contains(t, got, "connection reset by peer", "must preserve the trailing transport marker")
		// 1 marker line + 20 tail lines.
		assert.Equal(t, 21, len(strings.Split(got, "\n")))
	})
}

// recorder captures log lines. It is safe for concurrent use because the heartbeat
// goroutine logs while the test reads.
type recorder struct {
	mu   sync.Mutex
	logs []string
}

func (r *recorder) Logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.logs...)
}

func Test_Silent(t *testing.T) {
	t.Parallel()

	t.Run("returns a suppressed copy without mutating the receiver", func(t *testing.T) {
		t.Parallel()

		cli := &CLI{T: t, ConfigFilePath: "/tmp/config.yaml", WorkingDirectory: "/tmp/work"}

		silent := cli.Silent()

		require.NotSame(t, cli, silent, "Silent must copy so concurrent callers cannot race on a shared CLI")
		assert.True(t, silent.suppressLogs)
		assert.False(t, cli.suppressLogs, "the receiver must keep logging enabled")

		// Every other field has to survive the copy, or the silent CLI would target the
		// wrong config or working directory.
		assert.Same(t, t, silent.T)
		assert.Equal(t, cli.ConfigFilePath, silent.ConfigFilePath)
		assert.Equal(t, cli.WorkingDirectory, silent.WorkingDirectory)
	})

	t.Run("is nil-safe", func(t *testing.T) {
		t.Parallel()

		var cli *CLI
		assert.Nil(t, cli.Silent())
	})

	t.Run("logf writes only when not suppressed", func(t *testing.T) {
		t.Parallel()

		rec := &recorder{}
		// T is deliberately nil: any log site that bypasses logf and reaches T directly
		// panics rather than silently passing this test.
		cli := &CLI{T: nil, logDestination: rec}

		cli.logf("loud %s", "message")
		require.Equal(t, []string{"loud message"}, rec.snapshot())

		cli.Silent().logf("quiet %s", "message")
		assert.Equal(t, []string{"loud message"}, rec.snapshot(), "a silent CLI must not log")
	})

	t.Run("ReportCommandResult logs every output line unless suppressed", func(t *testing.T) {
		t.Parallel()

		// ReportCommandResult is a production path that panics when it runs after the test
		// completes, so assert suppression reaches it rather than only logf.
		loud := &recorder{}
		cli := &CLI{T: nil, logDestination: loud}

		err := cli.ReportCommandResult(t.Context(), "line one\nline two", "rad app delete", nil)
		require.NoError(t, err)
		require.Equal(t, []string{"[rad] line one", "[rad] line two"}, loud.snapshot(),
			"the loud path must log, otherwise the suppressed assertion below proves nothing")

		quiet := &recorder{}
		silent := (&CLI{T: nil, logDestination: quiet}).Silent()

		err = silent.ReportCommandResult(t.Context(), "line one\nline two", "rad app delete", nil)
		require.NoError(t, err)
		assert.Empty(t, quiet.snapshot(), "command output must not reach the test log when suppressed")
	})

	t.Run("heartbeat logs periodically unless suppressed", func(t *testing.T) {
		t.Parallel()

		// The heartbeat is the riskier of the two log sites: it fires every interval for
		// the whole lifetime of a background cascade delete, well past test completion.
		loud := &recorder{}
		cli := &CLI{T: nil, logDestination: loud, heartbeatInterval: time.Millisecond}

		done := make(chan struct{})
		go cli.heartbeat("rad app delete", done)

		require.Eventually(t, func() bool { return len(loud.snapshot()) > 0 }, 5*time.Second, time.Millisecond,
			"the loud path must emit a heartbeat, otherwise the suppressed assertion below proves nothing")
		done <- struct{}{}
		close(done)

		assert.Contains(t, loud.snapshot()[0], "[heartbeat] command rad app delete is still running")

		quiet := &recorder{}
		silent := (&CLI{T: nil, logDestination: quiet, heartbeatInterval: time.Millisecond}).Silent()

		silentDone := make(chan struct{})
		go silent.heartbeat("rad app delete", silentDone)

		// Long enough for many intervals to elapse.
		time.Sleep(50 * time.Millisecond)
		silentDone <- struct{}{}
		close(silentDone)

		assert.Empty(t, quiet.snapshot(), "a silent CLI must not emit heartbeats")
	})
}
