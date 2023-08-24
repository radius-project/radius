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

package clients

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

const (
	// ModuleName is used for telemetry if needed.
	ModuleName = "radius"

	// ModuleVersion is used for telemetry if needed.
	ModuleVersion = "public-preview"
)

// Options represents the client option for azure sdk client including authentication.
type Options struct {
	// Cred represents a credential for OAuth token.
	Cred azcore.TokenCredential

	// BaseURI represents the base URI for the client.
	BaseURI string

	// ARMClientOptions represents the client options for ARM clients.
	ARMClientOptions *arm.ClientOptions
}

// DeploymentEngineURL takes a base URI and a resource ID and returns a URL string by combining the two.
func DeploymentEngineURL(baseURI string, resourceID string) string {
	return runtime.JoinPaths(strings.TrimSuffix(baseURI, "/"), "/", strings.TrimPrefix(resourceID, "/"))
}
