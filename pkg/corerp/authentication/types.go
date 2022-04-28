// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"sync"
	"time"
)

const (
	ArmTimeFormat = time.RFC3339
)

// Certificate represents the client certificate fetched from arm metadata endpoint
type certificate struct {
	Certificate string `json:"certificate"`
	NotAfter    string `json:"notAfter"`
	NotBefore   string `json:"notBefore"`
	Thumbprint  string `json:"thumbprint"`
}

// ClientCertificates stores the array of certificate returned from arm metadata endpoint
type clientCertificates struct {
	ClientCertificates []certificate `json:"clientCertificates"`
}

// armCertStore stores active client certificates fetched from arm metadata endpoint
type armCertStore struct {
	thumbprintMap sync.Map // maps from thumbprint -> Certificate
}
