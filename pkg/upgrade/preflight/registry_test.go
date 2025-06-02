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

package preflight

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPreflightCheck implements PreflightCheck for testing
type MockPreflightCheck struct {
	name     string
	severity CheckSeverity
	success  bool
	message  string
	err      error
}

func (m *MockPreflightCheck) Name() string            { return m.name }
func (m *MockPreflightCheck) Severity() CheckSeverity { return m.severity }
func (m *MockPreflightCheck) Run(ctx context.Context) (bool, string, error) {
	return m.success, m.message, m.err
}

func TestRegistry_NewRegistry(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	assert.NotNil(t, registry)
	assert.Equal(t, mockOutput, registry.output)
	assert.Empty(t, registry.checks)
}

func TestRegistry_AddCheck(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	check1 := &MockPreflightCheck{name: "test1"}
	check2 := &MockPreflightCheck{name: "test2"}

	registry.AddCheck(check1)
	registry.AddCheck(check2)

	assert.Len(t, registry.checks, 2)
	assert.Equal(t, check1, registry.checks[0])
	assert.Equal(t, check2, registry.checks[1])
}

func TestRegistry_RunChecks_EmptyRegistry(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	results, err := registry.RunChecks(context.Background())

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestRegistry_RunChecks_AllSuccess(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	check1 := &MockPreflightCheck{
		name:     "Check 1",
		severity: SeverityError,
		success:  true,
		message:  "Check 1 passed",
	}
	check2 := &MockPreflightCheck{
		name:     "Check 2",
		severity: SeverityWarning,
		success:  true,
		message:  "Check 2 passed",
	}

	registry.AddCheck(check1)
	registry.AddCheck(check2)

	results, err := registry.RunChecks(context.Background())

	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify first result
	assert.Equal(t, check1, results[0].Check)
	assert.True(t, results[0].Success)
	assert.Equal(t, "Check 1 passed", results[0].Message)
	assert.NoError(t, results[0].Error)
	assert.Equal(t, SeverityError, results[0].Severity)

	// Verify second result
	assert.Equal(t, check2, results[1].Check)
	assert.True(t, results[1].Success)
	assert.Equal(t, "Check 2 passed", results[1].Message)
	assert.NoError(t, results[1].Error)
	assert.Equal(t, SeverityWarning, results[1].Severity)
}

func TestRegistry_RunChecks_ErrorSeverityFails(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	check1 := &MockPreflightCheck{
		name:     "Passing Check",
		severity: SeverityInfo,
		success:  true,
		message:  "This check passed",
	}
	check2 := &MockPreflightCheck{
		name:     "Failing Check",
		severity: SeverityError,
		success:  false,
		message:  "This check failed",
	}
	check3 := &MockPreflightCheck{
		name:     "Should Not Run",
		severity: SeverityWarning,
		success:  true,
		message:  "This should not be reached",
	}

	registry.AddCheck(check1)
	registry.AddCheck(check2)
	registry.AddCheck(check3)

	results, err := registry.RunChecks(context.Background())

	// Should fail immediately on error severity check
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight check 'Failing Check' failed")
	assert.Len(t, results, 2) // Only first two checks should have run

	// Verify results
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
}

func TestRegistry_RunChecks_WarningSeverityDoesNotFail(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	check1 := &MockPreflightCheck{
		name:     "Warning Check",
		severity: SeverityWarning,
		success:  false,
		message:  "This is a warning",
	}
	check2 := &MockPreflightCheck{
		name:     "Info Check",
		severity: SeverityInfo,
		success:  true,
		message:  "This is info",
	}

	registry.AddCheck(check1)
	registry.AddCheck(check2)

	results, err := registry.RunChecks(context.Background())

	// Should succeed even with failed warning check
	require.NoError(t, err)
	assert.Len(t, results, 2)

	assert.False(t, results[0].Success)
	assert.Equal(t, SeverityWarning, results[0].Severity)
	assert.True(t, results[1].Success)
	assert.Equal(t, SeverityInfo, results[1].Severity)
}

func TestRegistry_RunChecks_CheckReturnsError(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	check1 := &MockPreflightCheck{
		name:     "Error Check",
		severity: SeverityError,
		success:  false,
		message:  "Check failed",
		err:      errors.New("internal error"),
	}

	registry.AddCheck(check1)

	results, err := registry.RunChecks(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight check 'Error Check' failed")
	assert.Len(t, results, 1)

	assert.False(t, results[0].Success)
	assert.Equal(t, "Check failed", results[0].Message)
	assert.EqualError(t, results[0].Error, "internal error")
}

func TestRegistry_GetFailureReason(t *testing.T) {
	mockOutput := &output.MockOutput{}
	registry := NewRegistry(mockOutput)

	tests := []struct {
		name     string
		result   CheckResult
		expected string
	}{
		{
			name: "error takes precedence",
			result: CheckResult{
				Error:   errors.New("error occurred"),
				Message: "some message",
			},
			expected: "error occurred",
		},
		{
			name: "message when no error",
			result: CheckResult{
				Message: "check failed message",
			},
			expected: "check failed message",
		},
		{
			name:     "default when no error or message",
			result:   CheckResult{},
			expected: "check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := registry.getFailureReason(tt.result)
			assert.Equal(t, tt.expected, reason)
		})
	}
}
