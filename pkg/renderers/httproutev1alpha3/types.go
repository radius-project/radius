// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import "github.com/project-radius/radius/pkg/azure/radclient"

const (
	ResourceType = "HttpRoute"
)

func GetEffectivePort(h radclient.HTTPRouteProperties) int {
	if h.Port != nil {
		return int(*h.Port)
	} else {
		return 80
	}
}
