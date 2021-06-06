// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import "github.com/Azure/radius/pkg/rad/clients"

type ARMManagementClient struct {
}

var _ clients.ManagementClient = (*ARMManagementClient)(nil)
