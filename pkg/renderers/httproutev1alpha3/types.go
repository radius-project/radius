// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

const (
	ResourceType = "HttpRoute"
)

type HttpRoute struct {
	Port   *int   `json:"port"`
	Url    string `json:"url"`
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
}

func (h HttpRoute) GetEffectivePort() int {
	if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}
