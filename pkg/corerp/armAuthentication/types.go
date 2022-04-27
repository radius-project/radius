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

//struct for the certificates fetched from arm metadata
type Certificate struct {
	Certificate string `json:"certificate"`
	NotAfter    string `json:"notAfter"`
	NotBefore   string `json:"notBefore"`
	Thumbprint  string `json:"thumbprint"`
}

//arm metadata endpoint returns an array of certificate
type ClientCertificates struct {
	ClientCertificates []Certificate `json:"clientCertificates"`
}

/*
The armCertStore is responsible for storing certificates and returning certificates that are valid based on
the current time
*/
type armCertStore struct {
	thumbprintMap map[string]Certificate //maps from thumbprint -> Certificate
	mutex         sync.Mutex
}
