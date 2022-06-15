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
	CreatedAt time.Time
}

type Test struct {
	Flag int
	Data interface{}
}

var (
	testData = &TestTime{
		Name:      "hello",
		CreatedAt: time.Now(),
	}
)

func TestDecodeMap_WithoutTimeDecodeHook(t *testing.T) {
	test := Test{
		Flag: 1,
		Data: testData,
	}
	jsv, _ := json.Marshal(test)
	i := make(map[string]interface{})

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
			ToTimeHookFunc()),
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	require.NoError(t, err)

	test := Test{
		Flag: 2,
		Data: testData,
	}

	jsv, _ := json.Marshal(test)
	i := make(map[string]interface{})

	err = json.Unmarshal(jsv, &i)
	require.NoError(t, err)

	err = decoder.Decode(i["Data"])
	require.NoError(t, err)
	require.True(t, out.CreatedAt.Equal(testData.CreatedAt))
}
