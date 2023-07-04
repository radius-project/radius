/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

var (
	ErrClientCertFetch = errors.New("failed to fetch client certificate from arm metadata endpoint - ")
)

// ArmCertManager defines the arm client manager for fetching the client cert from arm metadata endpoint
type ArmCertManager struct {
	armMetaEndpoint string
	period          time.Duration
	log             logr.Logger
}

// NewArmCertManager creates a new ArmCertManager
func NewArmCertManager(armMetaEndpoint string, log logr.Logger) *ArmCertManager {
	certMgr := ArmCertManager{
		armMetaEndpoint: armMetaEndpoint,
		period:          1 * time.Hour,
		log:             log,
	}
	return &certMgr
}

// fetchARMClientCert fetches the client certificates from arm metadata endpoint
func (acm *ArmCertManager) fetchARMClientCert() ([]Certificate, error) {
	client := http.Client{}
	resp, err := client.Get(acm.armMetaEndpoint)
	if err != nil || resp.StatusCode != http.StatusOK {
		acm.log.Error(ErrClientCertFetch, err.Error())
		return nil, ErrClientCertFetch
	} else if resp.StatusCode != http.StatusOK {
		acm.log.Error(ErrClientCertFetch, "Response code - ", resp.StatusCode)
		return nil, ErrClientCertFetch
	}
	defer resp.Body.Close()
	var certificates clientCertificates
	err = json.NewDecoder(resp.Body).Decode(&certificates)
	if err != nil {
		acm.log.Error(ErrClientCertFetch, err.Error())
		return nil, ErrClientCertFetch
	}
	return certificates.ClientCertificates, nil
}

// IsValidThumbprint verifies the thumbprint received in the request header against the list of thumbprints
// fetched from arm metadata endpoint
func IsValidThumbprint(thumbprint string) bool {
	armPublicCerts := getValidCertificates()
	for _, cert := range armPublicCerts {
		if strings.EqualFold(cert.Thumbprint, thumbprint) {
			return true
		}
	}
	return false
}

// Start fetches the client certificates from the arm metadata endpoint during service start up
// and runs in the background the periodic certificate refresher.
func (acm *ArmCertManager) Start(ctx context.Context) error {
	certs, err := acm.refreshCert()
	if err != nil {
		acm.log.Error(ErrClientCertFetch, err.Error())
		return ErrClientCertFetch
	} else if len(certs) == 0 {
		acm.log.Error(ErrClientCertFetch, " No client certificates fetched from ARM Meta endpoint")
		return ErrClientCertFetch
	}
	storeCertificates(certs)
	go acm.periodicCertRefresh(ctx)
	return nil
}

// refreshCert refreshes the arm client certs and updates it in the cert store
func (acm *ArmCertManager) refreshCert() ([]Certificate, error) {
	newCertificates, err := acm.fetchARMClientCert()
	if err != nil {
		acm.log.Error(ErrClientCertFetch, err.Error())
		return nil, ErrClientCertFetch
	}
	storeCertificates(newCertificates)
	certs := getValidCertificates()
	return certs, nil
}

// periodicCertRefresh refreshes the arm client certs periodically defined in the ArmCertManager
func (acm *ArmCertManager) periodicCertRefresh(ctx context.Context) {
	for {
		select {
		case <-time.After(acm.period):
			break
		case <-ctx.Done():
			return
		}
		certs, err := acm.refreshCert()
		if err != nil {
			return
		}
		storeCertificates(certs)
	}
}
