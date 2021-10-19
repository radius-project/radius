// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

const (
	ResourceType = "Gateway"
)

type Gateway struct {
	Listeners map[string]Listener `json:"listeners,omitempty"`
}

type Listener struct {
	Port     *int   `json:"port,omitempty"`
	Protocol string `json:"protocol"`
	Tls      TLS    `json:"tls"`
}

type TLS struct {
	Source string `json:"certificate,omitempty"`
}
