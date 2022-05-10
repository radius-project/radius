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

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/radlogger"
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
func (armCertMgr *ArmCertManager) fetchARMClientCert() ([]Certificate, error) {
	client := http.Client{}
	resp, err := client.Get(armCertMgr.armMetaEndpoint)
	if err != nil || resp.StatusCode != http.StatusOK {
		armCertMgr.log.V(radlogger.Error).Info(ErrClientCertFetch.Error(), err.Error())
		return nil, ErrClientCertFetch
	} else if resp.StatusCode != http.StatusOK {
		armCertMgr.log.V(radlogger.Error).Info(ErrClientCertFetch.Error(), "Response code - ", resp.StatusCode)
		return nil, ErrClientCertFetch
	}
	defer resp.Body.Close()
	var certificates clientCertificates
	err = json.NewDecoder(resp.Body).Decode(&certificates)
	if err != nil {
		armCertMgr.log.V(radlogger.Error).Info(ErrClientCertFetch.Error(), err.Error())
		return nil, ErrClientCertFetch
	}
	return certificates.ClientCertificates, nil
}

// IsValidThumbprint verifies the thumbprint received in the request header against the list of thumbprints
// fetched from arm metadata endpoint
func (armCertMgr *ArmCertManager) IsValidThumbprint(thumbprint string) (bool, error) {
	armPublicCerts := getValidCertificates()
	for _, cert := range armPublicCerts {
		if strings.EqualFold(cert.Thumbprint, thumbprint) {
			return true, nil
		}
	}
	return false, nil
}

// Start fetching the client certificates from the arm metadata endpoint during service start up
//  and runs in the background the periodic certificate refresher.
func (armCertMgr *ArmCertManager) Start(ctx context.Context) ([]Certificate, error) {
	certs, err := armCertMgr.refreshCert()
	if err != nil || len(certs) == 0 {
		armCertMgr.log.V(radlogger.Error).Info(ErrClientCertFetch.Error(), err, " number of certs fetched ", len(certs))
		return nil, ErrClientCertFetch
	}
	storeCertificates(certs)
	go armCertMgr.periodicCertRefresh(ctx)
	storedCerts := getValidCertificates()
	return storedCerts, nil
}

// refreshCert refreshes the arm client certs and updates it in the cert store
func (armCertMgr *ArmCertManager) refreshCert() ([]Certificate, error) {
	newCertificates, err := armCertMgr.fetchARMClientCert()
	if err != nil {
		armCertMgr.log.V(radlogger.Error).Info(ErrClientCertFetch.Error(), err)
		return nil, ErrClientCertFetch
	}
	storeCertificates(newCertificates)
	certs := getValidCertificates()
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
		storeCertificates(certs)
	}
}
