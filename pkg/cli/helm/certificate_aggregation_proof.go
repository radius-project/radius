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

package helm

import (
	"context"
	"errors"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"k8s.io/client-go/rest"
)

// PodAggregationProof binds a serving Source report to the running UCP container.
type PodAggregationProof struct {
	Evidence     mtls.AggregationEvidence
	ContainerID  string
	RestartCount int32
}

// ObserveAggregationProof observes serving trust without requiring component PKI.
func ObserveAggregationProof(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodAggregationProof, error) {
	if target.ContainerName != "ucp" {
		return PodAggregationProof{}, errors.New("aggregation serving evidence requires the UCP container")
	}
	proof, err := observePodTLSProof(ctx, config, target, mtls.AggregationProofPath, func(evidence mtls.StartupEvidence) error {
		return mtls.AggregationEvidence(evidence).Validate(string(target.PodUID), target.TrustBundleSHA256, target.LeafSHA256, time.Now())
	})
	if err != nil {
		return PodAggregationProof{}, err
	}
	return PodAggregationProof{Evidence: mtls.AggregationEvidence(proof.Evidence),
		ContainerID: proof.ContainerID, RestartCount: proof.RestartCount}, nil
}
