/*
Copyright 2023.

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

package reconciler

import (
	"fmt"
	"reflect"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
)

// resourceToConnectionValues converts a resource to a map of connection values. This will filter out any
// properties that should not be considered as env-vars or secrets.
func resourceToConnectionValues(name string, resource generated.GenericResource) (map[string]string, error) {
	values := map[string]string{}

	for k, v := range resource.Properties {
		switch k {
		case "application":
		case "environment":
		case "provisioningState":
		case "resourceProvisioning":
		case "recipe":
		case "resources":
		case "status":
		default:
			// Ignore composite types. Values are scalars.
			kind := reflect.TypeOf(v).Kind()
			if kind == reflect.Map {
				break
			} else if kind == reflect.Slice {
				break
			} else if kind == reflect.Struct {
				break
			}

			switch v := v.(type) {
			case string:
				values[k] = v
			case bool:
				values[k] = fmt.Sprintf("%t", v)
			case float64:
				values[k] = fmt.Sprintf("%v", v)
			default:
				return nil, fmt.Errorf("unsupported type for property %s: %T", k, v)
			}
		}
	}

	return values, nil
}
