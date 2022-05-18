// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import "github.com/mitchellh/mapstructure"

// DecodeMap decodes map[string]interface{} structure to the type of out.
func DecodeMap(in interface{}, out interface{}) error {
	cfg := &mapstructure.DecoderConfig{
		TagName: "json",
		Result:  out,
		Squash:  true,
	}
	decoder, _ := mapstructure.NewDecoder(cfg)
	return decoder.Decode(in)
}
