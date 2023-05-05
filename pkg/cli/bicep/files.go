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

// # Function Explanation
// 
//	ReadARMJSON reads a JSON file from the given file path and returns a map of strings to any type, or an error if the file
//	 could not be read or unmarshalled. Error handling is included to provide useful information to the caller if the file 
//	could not be read or unmarshalled.
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
