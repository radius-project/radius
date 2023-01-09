// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"reflect"
	"time"

	"github.com/mitchellh/mapstructure"
)

// DecodeMap decodes map[string]interface{} structure to the type of out.
func DecodeMap(in any, out any) error {
	cfg := &mapstructure.DecoderConfig{
		TagName: "json", // Use the JSON config for conversions.
		Result:  out,
		Squash:  true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			toTimeHookFunc()),
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	if err != nil {
		return err
	}

	return decoder.Decode(in)
}

// https://github.com/mitchellh/mapstructure/issues/159
func toTimeHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(time.Time{}) {
			return data, nil
		}

		switch f.Kind() {
		case reflect.String:
			return time.Parse(time.RFC3339, data.(string))
		case reflect.Float64:
			return time.Unix(0, int64(data.(float64))*int64(time.Millisecond)), nil
		case reflect.Int64:
			return time.Unix(0, data.(int64)*int64(time.Millisecond)), nil
		default:
			return data, nil
		}
	}
}
