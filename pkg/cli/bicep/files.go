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

package bicep

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReadARMJSON reads a JSON file from the given file path and returns a map of strings to any type, or an error if the file
//
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
