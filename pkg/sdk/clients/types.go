// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
)

const (
	ModuleName    = "radius"
	ModuleVersion = "public-preview"
)

// Options represents the client option for azure sdk client including authentication.
type Options struct {
	// Cred represents a credential for OAuth token.
	Cred azcore.TokenCredential

	BaseURI string

	ARMClientOptions *arm.ClientOptions
}
