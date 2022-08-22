// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
)

type TestTime struct {
	Name      string
	CreatedAt *time.Time
}

type Test struct {
	Flag int
	Data interface{}
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
	i := make(map[string]interface{})

	err := json.Unmarshal(jsv, &i)
	require.NoError(t, err)

	r := TestTime{}
	err = mapstructure.Decode(i["Data"], &r)
	require.Error(t, err)
}

// // Timestamp struct
// type Timestamp struct {
// 	time.Time
// 	rfc3339 bool
// }

// // MatshalJSON is the custom marshaller function for Timestamp
// func (t Timestamp) MarshalJSON() ([]byte, error) {
// 	if t.rfc3339 {
// 		return t.Time.MarshalJSON()
// 	}
// 	return t.formatUnix()
// }

// // MatshalJSON is the custom marshaller function for Timestamp
// func (t *Timestamp) UnmarshalJSON(data []byte) error {
// 	err := t.Time.UnmarshalJSON(data)
// 	if err != nil {
// 		return t.parseUnix(data)
// 	}
// 	t.rfc3339 = true
// 	return nil
// }

// func (t Timestamp) formatUnix() ([]byte, error) {
// 	sec := float64(t.Time.UnixNano()) * float64(time.Nanosecond) / float64(time.Second)
// 	return strconv.AppendFloat(nil, sec, 'f', -1, 64), nil
// }

// func (t *Timestamp) parseUnix(data []byte) error {
// 	f, err := strconv.ParseFloat(string(data), 64)
// 	if err != nil {
// 		return err
// 	}
// 	t.Time = time.Unix(0, int64(f*float64(time.Second/time.Nanosecond)))
// 	return nil
// }

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

	testCases := []struct {
		desc string
		ts   time.Time
	}{
		{
			"time-now",
			time.Now(),
		},
		{
			"time-unix-date",
			time.Unix(time.Now().Unix(), 0),
		},
	}

	for idx, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			test := Test{
				Flag: idx,
				Data: &TestTime{
					Name:      uuid.NewString(),
					CreatedAt: &tc.ts,
				},
			}

			// this marshals unix time as string all the time
			jsv, _ := json.Marshal(test)
			i := make(map[string]interface{})

			err = json.Unmarshal(jsv, &i)
			require.NoError(t, err)

			err = decoder.Decode(i["Data"])
			require.NoError(t, err)
			require.True(t, out.CreatedAt.Equal(tc.ts))
		})
	}
}
