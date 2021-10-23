// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

const radiusdEnvVar = "RAD_RADIUSD"

func GetLocalRadiusDFilepath() (string, error) {
	return getLocalToolFilepath("radiusd", radiusdEnvVar)
}
