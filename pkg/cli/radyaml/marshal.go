// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"errors"

	"gopkg.in/yaml.v3"
)

var _ yaml.Marshaler = (*BuildTarget)(nil)
var _ yaml.Unmarshaler = (*BuildTarget)(nil)

func (bt BuildTarget) MarshalYAML() (interface{}, error) {
	return map[string]interface{}{
		bt.Builder: bt.Values,
	}, nil
}

func (bt *BuildTarget) UnmarshalYAML(node *yaml.Node) error {
	raw := map[string]map[string]interface{}{}
	err := node.Decode(&raw)
	if err != nil {
		return err
	}

	if len(raw) != 1 {
		return errors.New("a build target should specify a single builder")
	}

	for key, value := range raw {
		bt.Builder = key
		bt.Values = value
		return nil
	}

	panic("unreachable code")
}
