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

package v1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeRFC3339_MarshalText(t *testing.T) {
	tt := timeRFC3339(time.Now())
	text, err := tt.MarshalText()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(text) == 0 {
		t.Errorf("Expected text time, got empty string")
	}
}

func TestTimeRFC3339_MarshalJSON(t *testing.T) {
	tt := timeRFC3339(time.Now())
	json, err := tt.MarshalJSON()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(json) == 0 {
		t.Errorf("Expected JSON time, got empty string")
	}
}

func TestUnmarshalTimeString(t *testing.T) {
	parsedTime := UnmarshalTimeString("2021-09-24T19:09:00.000000Z")
	require.NotNil(t, parsedTime)

	require.Equal(t, 2021, parsedTime.Year())
	require.Equal(t, time.Month(9), parsedTime.Month())
	require.Equal(t, 24, parsedTime.Day())

	parsedTime = UnmarshalTimeString("")
	require.NotNil(t, parsedTime)
	require.Equal(t, 1, parsedTime.Year())
}

func TestTimeRFC3339_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "Valid RFC3339Nano time",
			data:    time.Now().Format(time.RFC3339Nano),
			wantErr: false,
		},
		{
			name:    "Valid UTC time",
			data:    time.Now().UTC().Format(utcLayout),
			wantErr: false,
		},
		{
			name:    "Invalid time",
			data:    "invalid time",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tr timeRFC3339
			if err := tr.UnmarshalText([]byte(tt.data)); (err != nil) != tt.wantErr {
				t.Errorf("timeRFC3339.UnmarshalText() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimeRFC3339_UnmarshalJSON(t *testing.T) {
	testCases := []struct {
		name     string
		data     string
		expected time.Time
	}{
		{
			name:     "UTC time",
			data:     `"2023-01-01T00:00:00.000000000"`,
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "RFC3339Nano time",
			data:     `"2023-01-01T00:00:00.000000000Z"`,
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var tr timeRFC3339
			err := tr.UnmarshalJSON([]byte(tc.data))
			if err != nil {
				t.Fatalf("UnmarshalJSON failed: %v", err)
			}

			if !time.Time(tr).Equal(tc.expected) {
				t.Errorf("Unmarshalled time does not match expected. Got: %v, Expected: %v", time.Time(tr), tc.expected)
			}
		})
	}
}

func TestTimeRFC3339_Parse(t *testing.T) {
	testCases := []struct {
		name    string
		layout  string
		value   string
		want    timeRFC3339
		wantErr bool
	}{
		{
			name:    "valid RFC3339 time",
			layout:  time.RFC3339,
			value:   "2006-01-02T15:04:05Z",
			want:    timeRFC3339(time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)),
			wantErr: false,
		},
		{
			name:    "invalid time",
			layout:  time.RFC3339,
			value:   "invalid time",
			wantErr: true,
		},
		// Add more test cases as needed
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var tr timeRFC3339
			err := tr.Parse(tc.layout, tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("timeRFC3339.Parse() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr && time.Time(tr) != time.Time(tc.want) {
				t.Errorf("timeRFC3339.Parse() = %v, want %v", tr, tc.want)
			}
		})
	}
}
