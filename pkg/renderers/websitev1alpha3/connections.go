// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package websitev1alpha3

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/radius/pkg/renderers"
)

func makeEnvironmentVariablesForConnections(connections map[string]Connection, dependencies map[string]renderers.RendererDependency) map[string]string {
	env := map[string]string{}

	// Take each connection and create environment variables for each part
	for name, con := range connections {
		properties := dependencies[con.Source]
		for key, value := range properties.ComputedValues {
			name := fmt.Sprintf("%s_%s_%s", "CONNECTION", strings.ToUpper(name), strings.ToUpper(key))
			switch v := value.(type) {
			case string:
				env[name] = v
			case float64: // Float is used by the JSON serializer
				env[name] = string(strconv.Itoa(int(v)))
			case int:
				env[name] = string(strconv.Itoa(v))
			}
		}
	}

	return env
}

func merge(env map[string]string, additional map[string]string) {
	for k, v := range additional {
		env[k] = v
	}
}

func getSortedKeys(env map[string]string) []string {
	keys := []string{}
	for k := range env {
		key := k
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}
