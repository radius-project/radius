// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
)

type TestTime struct {
	Name      string
	CreatedAt *time.Time
}

type Test struct {
	Flag int
	Data any
}

func TestDecodeMap_WithoutTimeDecodeHook(t *testing.T) {
	now := time.Now()

	test := Test{
		Flag: 1,
		Data: &TestTime{
			Name:      "hello",
			CreatedAt: &now,
		},
	}

	jsv, _ := json.Marshal(test)
	i := make(map[string]any)

	err := json.Unmarshal(jsv, &i)
	require.NoError(t, err)

	r := TestTime{}
	err = mapstructure.Decode(i["Data"], &r)
	require.Error(t, err)
}

func TestDecodeMap_WithTimeDecodeHook(t *testing.T) {
	out := &TestTime{}
	cfg := &mapstructure.DecoderConfig{
		TagName: "json",
		Squash:  true,
		Result:  out,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			toTimeHookFunc()),
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	require.NoError(t, err)

	now, err := time.Parse(time.RFC3339, "2022-09-01T15:00:00Z")
	require.NoError(t, err)

	testCases := []struct {
		desc string
		obj  map[string]any
	}{
		{
			"time-now",
			map[string]any{
				"name":      "time-string",
				"createdAt": "2022-09-01T15:00:00Z",
			},
		},
		{
			"time-unix-float",
			map[string]any{
				"name":      "time-unix-float",
				"createdAt": float64(now.UnixMilli()),
			},
		},
		{
			"time-unix-int",
			map[string]any{
				"name":      "time-unix-int",
				"createdAt": int64(now.UnixMilli()),
			},
		},
	}
	for _, tt := range testCases {
		err = decoder.Decode(tt.obj)
		require.NoError(t, err)
		require.Equal(t, now, out.CreatedAt.UTC())
	}
}
