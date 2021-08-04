// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

func GetControlPlaneResourceGroup(resourceGroup string) string {
	return "RE-" + resourceGroup
}
