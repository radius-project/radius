// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// ArmCertManager defines the arm client manager for fetching the client cert from arm metadata endpoint
type ArmCertManager struct {
	armMetaEndpoint string
	certStore       *armCertStore
	period          time.Duration
}

// NewArmCertManager creates a new ArmCertManager
func NewArmCertManager(armMetaEndpoint string) *ArmCertManager {
	certMgr := ArmCertManager{
		armMetaEndpoint: armMetaEndpoint,
		certStore:       newArmCertStore(),
		period:          1 * time.Hour,
	}
	return &certMgr
}

// getARMClientCert fetches the client certificates from arm metadata endpoint
func (armCertMgr *ArmCertManager) getARMClientCert() ([]certificate, error) {
	client := http.Client{}
	resp, err := client.Get(armCertMgr.armMetaEndpoint)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch client certificate from arm metadata endpoint")
	}
	var certificates clientCertificates
	err = json.NewDecoder(resp.Body).Decode(&certificates)
	if err != nil {
		return nil, errors.New("failed to fetch client certificate from arm metadata endpoint")
	}
	return certificates.ClientCertificates, nil
}

// IsValidThumbprint verifies the thumbprint received in the request header against the list of thumbprints
// fetched from arm metadata endpoint

func (armCertMgr *ArmCertManager) IsValidThumbprint(thumbprint string) (bool, error) {
	armPublicCerts, err := armCertMgr.certStore.getValidCertificates()
	if err != nil {
		return false, err
	}
	for _, cert := range armPublicCerts {
		if strings.EqualFold(cert.Thumbprint, thumbprint) {
			return true, nil
		}
	}
	return false, nil
}

// Start fetching the client certificates from the arm metadata endpoint during service start up
//  and runs in the background the periodic certificate refresher.
func (armCertMgr *ArmCertManager) Start(ctx context.Context) ([]certificate, error) {
	certs, err := armCertMgr.refreshCert()
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, errors.New("failed to retrieve any certificates on ArmCertManager startup")
	}
	armCertMgr.certStore.storeCertificates(certs)
	go armCertMgr.periodicCertRefresh(ctx)
	storedCerts, err := armCertMgr.certStore.getValidCertificates()
	if err != nil {
		return nil, err
	}
	return storedCerts, nil
}

// refreshCert refreshes the arm client certs and updates it in the cert store
func (armCertMgr *ArmCertManager) refreshCert() ([]certificate, error) {
	newCertificates, err := armCertMgr.getARMClientCert()
	if err != nil {
		return nil, err
	}
	armCertMgr.certStore.storeCertificates(newCertificates)
	certs, err := armCertMgr.certStore.getValidCertificates()
	if err != nil {
		return nil, err
	}
	return certs, nil
}

// periodicCertRefresh refreshes the arm client certs periodically defined in the ArmCertManager
func (armCertMgr *ArmCertManager) periodicCertRefresh(ctx context.Context) {
	for {
		select {
		case <-time.After(armCertMgr.period):
			break
		case <-ctx.Done():
			return
		}
		certs, err := armCertMgr.refreshCert()
		if err != nil {
			return
		}
		armCertMgr.certStore.storeCertificates(certs)
	}
}
