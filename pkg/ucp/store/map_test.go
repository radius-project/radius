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

package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/project-radius/radius/pkg/ucp/resources"
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

	jsv, err := json.Marshal(test)
	require.NoError(t, err)
	i := make(map[string]any)

	err = json.Unmarshal(jsv, &i)
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

func TestDecodeMap_WithResourceIDs(t *testing.T) {
	type datatype struct {
		ID resources.ID
	}

	t.Run("valid", func(t *testing.T) {
		data := map[string]any{
			"ID": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app",
		}

		out := datatype{}
		err := DecodeMap(data, &out)
		require.NoError(t, err)

		require.Equal(t, data["ID"], out.ID.String())
	})

	t.Run("invalid", func(t *testing.T) {
		data := map[string]any{
			"ID": "asdf",
		}

		out := datatype{}
		err := DecodeMap(data, &out)
		require.Error(t, err)
	})
}
