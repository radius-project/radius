// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package builders

import (
	"context"
	"encoding/json"
	"fmt"
)

var _ Builder = (*dockerBuilder)(nil)

type npmBuilder struct {
}

type npmInput struct {
	Directory string             `json:"directory"`
	Script    string             `json:"script"`
	Container *npmContainerInput `json:"container,omitempty"`
}

type npmContainerInput struct {
	Image string `json:"image"`
}

func (builder *npmBuilder) Build(ctx context.Context, values interface{}, options BuilderOptions) (map[string]interface{}, error) {
	b, err := json.Marshal(&values)
	if err != nil {
		return nil, err
	}

	input := npmInput{}
	err = json.Unmarshal(b, &input)
	if err != nil {
		return nil, err
	}

	if input.Directory == "" {
		return nil, fmt.Errorf("%s is required", "directory")
	}
	if input.Script == "" {
		input.Script = "start"
	}

	input.Directory = normalize(options.BaseDirectory, input.Directory)
	output := map[string]interface{}{
		"name":             "npm",
		"workingDirectory": input.Directory,
		"args": []string{
			"run",
			input.Script,
		},
	}

	return output, nil
}
