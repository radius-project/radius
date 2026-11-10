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
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestRotationEveryWriterChecksOperation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"begin", "recovery begin", "APIService", "phase advance", "trust generation",
		"issuer", "component Certificate", "aggregation Certificate", "deployment", "aggregation deployment", "route", "recovery route", "runner"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner, state := rotationRunnerFixture(t)
			if name == "aggregation deployment" {
				runner, state = aggregationExecutionFixture(t, true)
			}
			s, ctx := runner.store, t.Context()
			kube := s.kube.(*kubefake.Clientset)
			client := s.dynamic.(*dynamicfake.FakeDynamicClient)
			journal, _, err := s.read(ctx)
			require.NoError(t, err)
			switch name {
			case "issuer", "component Certificate", "aggregation Certificate", "route":
				state.Phase = rotationIssue
			case "deployment":
				state.Phase = rotationAdopt
				_, _, err := s.publishComponentTrust(ctx, rotationComponentBundle(state))
				require.NoError(t, err)
			case "aggregation deployment":
				state.Phase = rotationRetire
				_, _, err := s.publishComponentTrust(ctx, state.NewCA)
				require.NoError(t, err)
			case "recovery route":
				state.Phase, state.Recovery = rotationRetire, true
			}
			data, err := json.Marshal(state)
			require.NoError(t, err)
			journal.Data[rotationJournalKey] = string(data)
			require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("configmaps"), journal, s.namespace))
			if name == "phase advance" {
				require.NoError(t, s.reconcileTrust(ctx, state.ID))
			}
			if name == "begin" {
				require.NoError(t, kube.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("configmaps"), s.namespace, journal.Name))
			}
			blocked := errors.New("operation identity changed")
			called := 0
			s.guardWrite = func(context.Context) error { called++; return blocked }
			kube.ClearActions()
			client.ClearActions()
			switch name {
			case "begin":
				_, err = s.begin(ctx, uuid.NewString(), state.NewCA)
			case "recovery begin":
				id := uuid.NewString()
				plan := *state.Plan
				plan.Issuer, plan.ServingSecret = "radius-ca-"+strings.ReplaceAll(id, "-", "")[:16], "radius-aggregation-"+strings.ReplaceAll(id, "-", "")[:16]
				_, err = s.beginRecovery(ctx, id, state.NewCA, &plan, journal)
			case "APIService":
				err = s.reconcileTrust(ctx, state.ID)
			case "phase advance":
				err = s.advance(ctx, journal, state, rotationAdopt)
			case "trust generation":
				_, _, err = s.publishComponentTrust(ctx, state.NewCA)
			case "issuer":
				err = s.ensureRotationIssuer(ctx, state.ID)
			case "component Certificate":
				err = s.switchRotationCertificate(ctx, state.ID, state.Plan.Consumers[0])
			case "aggregation Certificate":
				err = s.ensureAggregationCertificate(ctx, state.ID)
			case "deployment":
				err = s.reconcileRotationDeployment(ctx, state.ID, state.Plan.Consumers[0])
			case "aggregation deployment":
				err = s.reconcileAggregationDeployment(ctx, state.ID)
			case "route":
				_, err = s.setAggregationRoute(ctx, state.ID, "old", "old-hash", nil)
			case "recovery route":
				err = s.reconcileRecoveryRoute(ctx, state.ID)
			case "runner":
				err = runner.run(ctx, state.ID, rotationPublish)
			}
			require.ErrorIs(t, err, blocked)
			require.Equal(t, 1, called)
			for _, action := range append(kube.Actions(), client.Actions()...) {
				require.Contains(t, []string{"get", "list"}, action.GetVerb(), "no API writes after losing exclusion")
			}
		})
	}
}

func TestRotationWriteGuardRequired(t *testing.T) {
	t.Parallel()
	s := &rotationStore{}
	require.ErrorContains(t, s.checkWrite(t.Context()), "shared identity operation")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.guardWrite = func(context.Context) error {
		t.Fatal("cancelled write must stop before checking external state")
		return nil
	}
	require.ErrorIs(t, s.checkWrite(ctx), context.Canceled)
}

func TestRotationImportChecksOperation(t *testing.T) {
	t.Parallel()
	for _, existingNamespace := range []bool{false, true} {
		t.Run(map[bool]string{false: "namespace creation", true: "Secret creation"}[existingNamespace], func(t *testing.T) {
			t.Parallel()
			kube := kubefake.NewClientset()
			if existingNamespace {
				require.NoError(t, kube.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace}}))
			}
			blocked := errors.New("operation lock changed")
			plan := &certificatePlan{client: kube, namespace: RadiusSystemNamespace,
				secret:      &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "new-ca", Namespace: RadiusSystemNamespace}},
				beforeWrite: func(context.Context) error { return blocked }}
			require.ErrorIs(t, (&Impl{}).applyCertificatePlan(t.Context(), plan), blocked)
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}
