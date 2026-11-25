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

package ucplog

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Test_LogConstants(t *testing.T) {
	type write struct {
		Level   zapcore.Level
		Message string
	}
	tests := []struct {
		desiredLevel zapcore.Level
		expected     []write
	}{
		{
			desiredLevel: zapcore.ErrorLevel,
			expected:     []write{},
		},
		{
			desiredLevel: zapcore.WarnLevel,
			expected:     []write{},
		},
		{
			desiredLevel: zapcore.InfoLevel,
			expected: []write{
				{Level: 0, Message: "Default"},
				{Level: 0, Message: "Info"},
			},
		},
		{
			desiredLevel: zapcore.DebugLevel,
			expected: []write{
				{Level: 0, Message: "Default"},
				{Level: 0, Message: "Info"},
				{Level: -1, Message: "Debug"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desiredLevel.String(), func(t *testing.T) {
			t.Logf("Desired Level is %s - %d", test.desiredLevel.String(), test.desiredLevel)
			sink := &testCore{DesiredLevel: test.desiredLevel}
			zap := zap.New(sink)

			logger := zapr.NewLogger(zap)

			// NOTE: we only need to test Info() here. Using Error() ignores the V() call.
			logger.Info("Default")
			logger.V(LevelInfo).Info("Info")
			logger.V(LevelDebug).Info("Debug")

			// We only want to compare the level and message.
			actual := []write{}
			for i := range sink.Writes {
				actual = append(actual, write{Level: sink.Writes[i].Level, Message: sink.Writes[i].Message})
			}

			require.Equal(t, test.expected, actual)
		})
	}
}

var _ zapcore.Core = (*testCore)(nil)

type testCore struct {
	DesiredLevel zapcore.Level
	Writes       []zapcore.Entry
}

func (c *testCore) Enabled(level zapcore.Level) bool {
	return level >= c.DesiredLevel
}

func (c *testCore) With([]zap.Field) zapcore.Core {
	return c
}

func (c *testCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}

	return ce
}

func (c *testCore) Write(entry zapcore.Entry, _ []zapcore.Field) error {
	c.Writes = append(c.Writes, entry)
	return nil
}

func (c *testCore) Sync() error {
	return nil
}

func Test_BuildLoggerDoesNotSampleAuthorizationRequests(t *testing.T) {
	cfg := zap.NewProductionConfig()
	require.NotNil(t, cfg.Sampling)
	// An in-memory sink leaves no open file behind, and a fixed clock keeps
	// the whole burst in one sampling interval.
	sink := registerMemorySink(t)
	cfg.OutputPaths = []string{memorySinkScheme + ":"}
	logger, err := buildLogger(cfg, zap.WithClock(fixedClock{}))
	require.NoError(t, err)
	logger = logger.With(zap.String("request", "fields"))

	const burst = 200
	for range burst {
		logger.Info("Other message")
	}
	for range burst {
		logger.Info(AuthorizationRequestMessage, zap.String("result", "allow"))
	}
	logger.Info(AuthorizationRequestMessage, zap.String("result", "wouldDeny"))
	require.NoError(t, logger.Sync())

	counts := map[string]int{}
	for line := range strings.SplitSeq(strings.TrimSpace(sink.String()), "\n") {
		var entry struct {
			Message string `json:"msg"`
			Result  string `json:"result"`
			Request string `json:"request"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.Equal(t, "fields", entry.Request, "logger.With fields apply to both paths")
		counts[entry.Message+"/"+entry.Result]++
	}
	// Production sampling keeps the first 100 other messages, then every 100th.
	require.Equal(t, 101, counts["Other message/"])
	require.Equal(t, burst, counts[AuthorizationRequestMessage+"/allow"])
	require.Equal(t, 1, counts[AuthorizationRequestMessage+"/wouldDeny"])
}

const memorySinkScheme = "ucplogtest"

var memorySink = sync.OnceValues(func() (*memoryBuffer, error) {
	buffer := &memoryBuffer{}
	return buffer, zap.RegisterSink(memorySinkScheme, func(*url.URL) (zap.Sink, error) {
		return buffer, nil
	})
})

// registerMemorySink returns the process-wide test sink, emptied for this test.
func registerMemorySink(t *testing.T) *memoryBuffer {
	t.Helper()
	sink, err := memorySink()
	require.NoError(t, err)
	sink.Reset()
	return sink
}

type memoryBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *memoryBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *memoryBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *memoryBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer.Reset()
}

func (*memoryBuffer) Sync() error  { return nil }
func (*memoryBuffer) Close() error { return nil }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(0, 0) }

func (fixedClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }
