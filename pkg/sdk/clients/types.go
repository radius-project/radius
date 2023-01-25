// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

func DeploymentEngineURL(baseURI string, resourceID string) string {
	return runtime.JoinPaths(strings.TrimSuffix(baseURI, "/"), "/", strings.TrimPrefix(resourceID, "/"))
}
