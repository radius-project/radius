// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

const kcpEnvVar = "RAD_KCP"

func GetLocalKCPFilepath() (string, error) {
	return getLocalToolFilepath("kcp", kcpEnvVar)
}
