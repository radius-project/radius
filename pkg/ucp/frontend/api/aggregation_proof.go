/*
Copyright 2026 The Radius Authors.

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

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
)

// PrepareAggregationProof initializes the same Source later installed on the
// aggregation listener. It does not initialize component mTLS or enable metrics.
// Call before the host starts services; the returned handler belongs only on
// the management listener, never on a public or internal application route.
func (s *Service) PrepareAggregationProof(managementAvailable bool) (*mtls.AggregationProof, error) {
	switch os.Getenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED") {
	case "", "false":
		if os.Getenv("UCP_AGGREGATION_EXPECTED_TRUST_SHA256") != "" || os.Getenv("UCP_AGGREGATION_POD_UID") != "" {
			return nil, errors.New("aggregation serving proof settings require proof mode")
		}
		return nil, nil
	case "true":
	default:
		return nil, errors.New("aggregation serving proof enabled setting must be true or false")
	}
	if !managementAvailable {
		return nil, errors.New("aggregation serving proof requires an enabled management listener on a fixed port")
	}
	directory := s.options.Config.Server.TLSCertificateDirectory
	if directory == "" {
		return nil, errors.New("aggregation serving proof requires a TLS certificate directory")
	}
	podUID := os.Getenv("UCP_AGGREGATION_POD_UID")
	if os.Getenv("UCP_MTLS_STARTUP_PROOF_ENABLED") == "true" && os.Getenv("UCP_MTLS_POD_UID") != podUID {
		return nil, errors.New("component and aggregation proof must identify the same pod")
	}
	source := s.aggregationSource
	if source == nil {
		var err error
		source, err = mtls.NewSource(directory)
		if err != nil {
			return nil, errors.New("cannot initialize aggregation serving proof material")
		}
	}
	proof, err := mtls.NewAggregationProof(source, s.options.Config.Authorization.Namespace,
		podUID, os.Getenv("UCP_AGGREGATION_EXPECTED_TRUST_SHA256"))
	if err != nil {
		return nil, err
	}
	s.aggregationSource, s.aggregationProof, s.aggregationPodUID = source, proof, podUID
	return proof, nil
}

func (s *Service) configureAggregationServing(server *http.Server) (*mtls.Source, error) {
	directory := s.options.Config.Server.TLSCertificateDirectory
	if directory == "" {
		if s.aggregationProof != nil || os.Getenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED") == "true" {
			return nil, errors.New("aggregation serving proof requires the actual TLS listener")
		}
		return nil, nil
	}
	source := s.aggregationSource
	if os.Getenv("UCP_AGGREGATION_STARTUP_PROOF_ENABLED") == "true" && (source == nil || s.aggregationProof == nil) {
		return nil, errors.New("aggregation serving proof must be prepared before services start")
	}
	if source == nil {
		var err error
		source, err = mtls.NewSource(directory)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize aggregation serving certificate: %w", err)
		}
	}
	server.TLSConfig = source.ServingConfig(server.TLSConfig)
	podUID := s.aggregationPodUID
	if podUID == "" && os.Getenv("UCP_MTLS_STARTUP_PROOF_ENABLED") == "true" {
		// Preserve the original managed rotation header until chart integration
		// moves aggregation evidence to its independent settings.
		podUID = os.Getenv("UCP_MTLS_POD_UID")
		id, err := uuid.Parse(podUID)
		if err != nil || id == uuid.Nil || id.String() != podUID {
			return nil, errors.New("aggregation rotation evidence requires a canonical pod UID")
		}
	}
	if podUID != "" {
		server.Handler = withAggregationPodUID(server.Handler, podUID)
	}
	return source, nil
}
