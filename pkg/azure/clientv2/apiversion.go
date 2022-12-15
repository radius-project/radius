// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"strings"
)

// GetAPIVersionFromUserAgent can convert the user-agent used by an Azure SDK into an ARM API Version.
//
// Example: `apiVersion := clients.GetAPIVersionFromUserAgent(resource.UserAgent())`
func GetAPIVersionFromUserAgent(userAgent string) string {
	// UserAgent() returns a string of format: Azure-SDK-For-Go/v52.2.0 keyvault/2019-09-01 profiles/latest

	// Now we've got keyvault/2019-09-01
	middleSegment := strings.Split(userAgent, " ")[1]

	// Now we've got 2019-09-01
	return strings.Split(middleSegment, "/")[1]
}
