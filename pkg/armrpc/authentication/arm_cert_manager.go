// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/util"
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

// Start fetching the client certificates from the arm metadata endpoint during service start up
//
//	and runs in the background the periodic certificate refresher.
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
	acm.log.Info(fmt.Sprintf("@@@@@@ Before calling acm.periodicRefresh in %s, goroutineCount: %v", util.GetCaller(), runtime.NumGoroutine()))
	go acm.periodicCertRefresh(ctx)
	acm.log.Info(fmt.Sprintf("@@@@@@ After calling acm.periodicRefresh in %s, goroutineCount: %v", util.GetCaller(), runtime.NumGoroutine()))
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
