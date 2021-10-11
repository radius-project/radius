// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"io"

	"gopkg.in/yaml.v3"
)

func Parse(reader io.Reader) (Manifest, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	config := Manifest{}
	err := decoder.Decode(&config)
	if err != nil {
		return Manifest{}, nil
	}

	return config, nil
}
