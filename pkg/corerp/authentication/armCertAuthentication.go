// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package authentication

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type ArmCertManager struct {
	armMetaEndpoint string
	certStore       *armCertStore
	period          time.Duration
}

func NewArmCertManager(armMetaEndpoint string) *ArmCertManager {
	certMgr := ArmCertManager{
		armMetaEndpoint: armMetaEndpoint,
		certStore:       newArmCertStore(),
		period:          1 * time.Hour,
	}
	return &certMgr
}

//an endpoint URL can be passed from config, now it is hardcoded to the dogfood env
func (armCertMgr *ArmCertManager) getARMClientCert() ([]Certificate, error) {
	client := http.Client{}
	resp, err := client.Get(armCertMgr.armMetaEndpoint)
	var certificates ClientCertificates
	if err != nil {
		return nil, err
	}

	body, err := getResponseBody(resp)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, &certificates)
	if err != nil {
		return nil, err
	}
	return certificates.ClientCertificates, nil
}

func getResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil HTTP Response received")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

//The function verifies the thumbprint received in the header against the list of thumbprints
//fetched from arm metadata endpoint
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

//It is the starter function for fetching the client certificates from the arm metadata endpoint.
// it runs in the background the periodic certificate refresher.
func (armCertMgr *ArmCertManager) Start(ctx context.Context) ([]Certificate, error) {
	certs, err := armCertMgr.refreshCertificates()
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("failed to retrieve any certificates on ArmCertManager startup")
	}
	armCertMgr.certStore.storeCertificates(certs)
	go armCertMgr.periodicCertificateRefresh(ctx)
	storedCerts, err := armCertMgr.certStore.getValidCertificates()
	if err != nil {
		return nil, err
	}
	return storedCerts, nil
}

//the function refreshes the arm client certs and updates the store
func (armCertMgr *ArmCertManager) refreshCertificates() ([]Certificate, error) {
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

//the function refreshes the arm client certs periodically
func (armCertMgr *ArmCertManager) periodicCertificateRefresh(ctx context.Context) {
	for {
		select {
		case <-time.After(armCertMgr.period):
			break
		case <-ctx.Done():
			return
		}
		certs, err := armCertMgr.refreshCertificates()
		if err != nil {
			return
		}
		armCertMgr.certStore.storeCertificates(certs)
	}
}
