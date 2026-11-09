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
	"maps"
	"strings"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// planAggregationOnlyRotation is read-only and intentionally does not inspect
// component Secrets, Certificates, proof settings or external token volumes.
// Public activation remains gated until issuer/provider integration is complete.
func (s *rotationStore) planAggregationOnlyRotation(ctx context.Context, id, signingSecret, mode string, target []byte) (*rotationPlan, error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil || parsed.String() != id {
		return nil, errors.New("invalid aggregation rotation transaction")
	}
	digest, roots, err := mtls.TrustBundleDigest(target, s.now())
	if err != nil || len(roots) != 1 {
		return nil, errors.New("aggregation target must be one currently valid dedicated CA")
	}
	deployment, err := s.kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	service, err := s.kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if !rotationOwned(deployment, s.namespace, s.release) || deployment.Spec.Paused ||
		!rotationOwned(service, s.namespace, s.release) || deployment.Spec.ProgressDeadlineSeconds == nil {
		return nil, errors.New("aggregation planning requires owned, active UCP deployment and Service")
	}
	port, err := aggregationProofPort(deployment.Spec.Template.Spec)
	if err != nil {
		return nil, err
	}
	signing, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, signingSecret, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	suffix := strings.ReplaceAll(id, "-", "")[:16]
	plan := &rotationPlan{Scope: rotationAggregationOnly,
		Aggregation: &rotationAggregationWorkload{DeploymentUID: deployment.UID, ProofPort: port},
		TargetMode:  mode, TargetTrustMap: "radius-mtls-ca-" + digest[:32], TargetTrustSHA256: digest,
		SigningSecret: signingSecret, SigningUID: signing.UID,
		Issuer: "radius-ca-" + suffix, ServingSecret: "ucp-cert-aggregation-" + suffix,
		ServiceUID: service.UID, ServiceSelector: maps.Clone(service.Spec.Selector),
		UCPMinReadySeconds: deployment.Spec.MinReadySeconds, UCPProgressDeadlineSeconds: *deployment.Spec.ProgressDeadlineSeconds}
	if err := plan.validate(); err != nil {
		return nil, err
	}
	if err := s.checkPlannedSigner(ctx, rotationState{Plan: plan, NewCA: target}); err != nil {
		return nil, err
	}
	return plan, nil
}
