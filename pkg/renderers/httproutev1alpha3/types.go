// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

const (
	ResourceType = "HttpRoute"
)

type HttpRoute struct {
	Port    *int     `json:"port"`
	Gateway *Gateway `json:"gateway,omitempty"`
	Url     string   `json:"url"`
	Host    string   `json:"host"`
	Scheme  string   `json:"scheme"`
}

func (h HttpRoute) GetEffectivePort() int {
	if h.Port != nil {
		return *h.Port
	} else {
		return 80
	}
}

type Gateway struct {
	Source   string `json:"source"`
	Hostname string `json:"hostname"`
	Rules    []Rule `json:"rules"`
}

type Rule struct {
	Method  string   `json:"method"`
	Path    Path     `json:"path"`
	Headers []Header `json:"headers"`
}

type Path struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

type Header struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}
