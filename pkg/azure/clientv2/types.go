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
	ModuleName     = "radius"
	ModuleVersion  = "public-preview"
)

// BaseClient
type BaseClient struct {
	Client   *armresources.Client
	Pipeline *runtime.Pipeline
	BaseURI  string
}
