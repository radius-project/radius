// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"encoding/json"
	"fmt"
	"os"
)

func ReadARMJSON(filePath string) (map[string]any, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read json file: %w", err)
	}

	var template map[string]any
	err = json.Unmarshal(bytes, &template)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal json file: %w", err)
	}

	return template, nil
}
