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
package credentials

import (
	"strings"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// GetSecretName returns the secret name of credential storage.
//
// # Function Explanation
// 
//	GetSecretName takes in a resources.ID and returns a string which is the normalized name of the resource.
func GetSecretName(id resources.ID) string {
	planeNamespace := id.PlaneNamespace()
	planeNamespace = strings.ReplaceAll(planeNamespace, "/", "-")
	return kubernetes.NormalizeResourceName(planeNamespace + "-" + id.Name())
}
