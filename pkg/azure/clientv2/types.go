// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

const (
	DefaultBaseURI = "https://management.azure.com"

	// FIXM: Any ideas for moduleName and moduleVersion?
	moduleName    = "radius"
	moduleVersion = "public-preview"
)

type BaseClient struct {
	Client   *armresources.Client
	Pipeline *runtime.Pipeline
	BaseURI  string
}
