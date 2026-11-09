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
	"errors"

	"k8s.io/apimachinery/pkg/types"
)

type rotationScope string

const (
	rotationComponentsAndAggregation rotationScope = "componentsAndAggregation"
	rotationAggregationOnly          rotationScope = "aggregationOnly"
)

// Aggregation is a distinct serving identity, not the UCP component Certificate.
type rotationAggregationWorkload struct {
	DeploymentUID types.UID `json:"deploymentUID"`
	ProofPort     uint16    `json:"proofPort"`
}

func (p rotationPlan) effectiveScope() rotationScope {
	if p.Scope == "" {
		return rotationComponentsAndAggregation
	}
	return p.Scope
}

func (p rotationPlan) validateScope() error {
	if p.Aggregation != nil && (p.Aggregation.DeploymentUID == "" || p.Aggregation.ProofPort == 0) {
		return errors.New("invalid aggregation serving workload")
	}
	switch p.effectiveScope() {
	case rotationComponentsAndAggregation:
		_, roots, err := rotationHistoricalDigest(p.OldComponentCA)
		if err != nil || len(roots) != 1 || len(p.Consumers) != len(rotationComponents) {
			return errors.New("managed rotation requires all five component identities and dedicated trust")
		}
		if p.Aggregation != nil {
			for _, consumer := range p.Consumers {
				if consumer.Name == "ucp" && consumer.UID != p.Aggregation.DeploymentUID {
					return errors.New("aggregation and component UCP deployment identities differ")
				}
			}
		}
	case rotationAggregationOnly:
		if p.Aggregation == nil || len(p.Consumers) != 0 || len(p.OldComponentCA) != 0 {
			return errors.New("aggregation-only rotation requires independent serving identity and no component PKI")
		}
	default:
		return errors.New("unknown CA rotation scope")
	}
	return nil
}

func (p rotationPlan) aggregationWorkload() (rotationAggregationWorkload, error) {
	if p.Aggregation != nil {
		return *p.Aggregation, nil
	}
	if p.effectiveScope() == rotationComponentsAndAggregation {
		for _, consumer := range p.Consumers {
			if consumer.Name == "ucp" {
				return rotationAggregationWorkload{DeploymentUID: consumer.UID, ProofPort: consumer.ProofPort}, nil
			}
		}
	}
	return rotationAggregationWorkload{}, errors.New("rotation has no aggregation workload identity")
}
