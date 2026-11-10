//go:build !windows

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
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

func TestProviderPublicEntryAPIServer(t *testing.T) {
	s, options, impl := bootstrapNativeInstallFixture(t, func(selected *chart.Chart) {
		if selected.Metadata.Annotations == nil {
			selected.Metadata.Annotations = map[string]string{}
		}
		selected.Metadata.Annotations["radius.io/provider-transition-protocol"] = "1"
	})
	// Only the private test API server uses a larger client request budget.
	// Production retains the selected context's REST client defaults.
	flags := genericclioptions.NewConfigFlags(false)
	config, err := flags.ToRESTConfig()
	require.NoError(t, err)
	config.QPS, config.Burst = 100, 100
	s.kube, err = kubernetes.NewForConfig(config)
	require.NoError(t, err)
	s.dynamic, err = dynamic.NewForConfig(config)
	require.NoError(t, err)
	ctx := t.Context()
	// This fixture represents a prior completed installation. Issuance and
	// process evidence are not simulated as acceptance of the new provider.
	// The production driver must still stop at its real evidence barrier.
	var ca rotationState
	require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		s.guardWrite = operation.check
		defer func() { s.guardWrite = nil }()
		_, bootstrap, err := s.readBootstrap(ctx)
		require.NoError(t, err)
		s.activeBootstrap = bootstrap.ID
		defer func() { s.activeBootstrap = "" }()
		ca, err = s.bindBootstrapLifecycle(ctx, bootstrap)
		require.NoError(t, err)
		require.NoError(t, s.recordBootstrapLifecycle(ctx, ca))
		require.NoError(t, s.reconcileTrust(ctx, ca.ID))
		journal, bootstrap, err := s.readBootstrap(ctx)
		require.NoError(t, err)
		next := bootstrap
		next.Phase = bootstrapComplete
		_, err = s.updateBootstrap(ctx, journal, bootstrap, next)
		return err
	}))
	snapshot, err := s.aggregationTransitionSnapshot(ctx)
	require.NoError(t, err)
	require.True(t, snapshot.Complete)
	options.Radius.ChartPath, options.Radius.ChartVersion = "", ""
	options.Certificates = CertificateOptions{}
	options.MTLS.Provider = "radius"
	options.Radius.SetArgs = []string{"de.ucpMTLS.enabled=true"}
	impl.logger = nil
	deployments, err := s.kube.AppsV1().Deployments(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	configs, err := s.kube.CoreV1().ConfigMaps(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	handled, err := impl.CheckProviderTransition(ctx, options, "test")
	require.NoError(t, err)
	require.True(t, handled)
	require.ErrorContains(t, impl.UpgradeRadius(ctx, options, "test"), "outage confirmation")
	current, err := s.kube.AppsV1().Deployments(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, deployments.Items, current.Items, "preflight and missing consent cannot mutate workloads")
	currentConfigs, err := s.kube.CoreV1().ConfigMaps(s.namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, configs.Items, currentConfigs.Items, "preflight cannot prepare target configuration")
	_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, providerTransitionJournal, metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))

	// Supply valid component credentials as a cert-manager simulator. The
	// actual public driver, native dry runs, lease and journal remain in use.
	require.NoError(t, withIdentityOperation(ctx, s.kube, s.namespace, func(operation *identityOperation) error {
		return s.prepareProviderCertificates(ctx, ca, operation.check, false)
	}))
	root, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, ca.Plan.SigningSecret, metav1.GetOptions{})
	require.NoError(t, err)
	block, _ := pem.Decode(root.Data["tls.crt"])
	require.NotNil(t, block)
	certificate, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	block, _ = pem.Decode(root.Data["tls.key"])
	require.NotNil(t, block)
	private, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	require.NoError(t, err)
	signer, ok := private.(crypto.Signer)
	require.True(t, ok)
	for _, component := range rotationComponents {
		cert, key, _ := testSigningCA(t, nil, func(leaf *x509.Certificate) {
			leaf.IsCA, leaf.KeyUsage = false, x509.KeyUsageDigitalSignature
			leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
			leaf.DNSNames = []string{component + "." + s.namespace + ".svc"}
			leaf.NotAfter = time.Now().Add(24 * time.Hour)
		}, certificate, signer)
		_, err := s.kube.CoreV1().Secrets(s.namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: component + "-mtls", Namespace: s.namespace},
			Type:       corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": cert, "tls.key": key},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	// Envtest has no Deployment controller or component processes. Execute
	// through native staging, then require a resumable readiness failure.
	options.MTLS.OutageConfirmed = true
	options.Radius.Timeout = 10 * time.Second
	require.ErrorContains(t, impl.UpgradeRadius(ctx, options, "test"), "unfinished")
	store := &providerTransitionStore{kube: s.kube}
	_, pending, err := store.read(ctx, s.namespace)
	require.NoError(t, err)
	require.NotEqual(t, providerComplete, pending.Phase)
	require.NotNil(t, pending.Plan.Helm)
	require.Equal(t, snapshot, pending.Plan.Aggregation)
	require.NoError(t, s.checkAggregationTransitionSnapshot(ctx, snapshot))
	for _, component := range rotationComponents {
		object, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, component+"-cert", metav1.GetOptions{})
		require.NoError(t, err)
		ref, _, err := unstructured.NestedMap(object.Object, "spec", "issuerRef")
		require.NoError(t, err)
		require.Equal(t, ca.Plan.issuerReference(), ref)
	}
	_, retry, err := store.read(ctx, s.namespace)
	require.NoError(t, err)
	options.Radius.Timeout = 5 * time.Second
	require.Error(t, impl.UpgradeRadius(ctx, options, "test"), "no fabricated process evidence on retry")
	_, after, err := store.read(ctx, s.namespace)
	require.NoError(t, err)
	require.Equal(t, retry.Plan, after.Plan, "retry retains the pinned source, CA, values and workload identities")

	// Complete both directions using only simulated controller status and
	// process observations. Helm, dry-run admission, CA pins, configuration,
	// journals, credential retirement and all write fences remain native.
	controllerContext, stopController := context.WithCancel(ctx)
	controllerDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-controllerContext.Done():
				controllerDone <- nil
				return
			case <-ticker.C:
			}
			deployments, err := s.kube.AppsV1().Deployments(s.namespace).List(controllerContext, metav1.ListOptions{})
			if err != nil {
				if controllerContext.Err() != nil {
					controllerDone <- nil
				} else {
					controllerDone <- err
				}
				return
			}
			for index := range deployments.Items {
				deployment := &deployments.Items[index]
				count := ptr.Deref(deployment.Spec.Replicas, 1)
				if deployment.Status.ObservedGeneration == deployment.Generation && deployment.Status.Replicas == count {
					continue
				}
				deployment.Status = appsv1.DeploymentStatus{
					ObservedGeneration: deployment.Generation, Replicas: count, ReadyReplicas: count,
					AvailableReplicas: count, UpdatedReplicas: count,
					Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"}},
				}
				if _, err := s.kube.AppsV1().Deployments(s.namespace).UpdateStatus(controllerContext, deployment, metav1.UpdateOptions{}); err != nil &&
					!apierrors.IsConflict(err) && controllerContext.Err() == nil {
					controllerDone <- err
					return
				}
			}
		}
	}()
	t.Cleanup(func() {
		stopController()
		require.NoError(t, <-controllerDone)
	})
	var observed atomic.Int32
	impl.observeProviderTransition = func(context.Context, providerTransitionPlan) error {
		observed.Add(1)
		return nil
	}
	options.Radius.Timeout = 3 * time.Minute
	err = impl.UpgradeRadius(ctx, options, "test")
	if err != nil {
		_, failed, readErr := store.read(ctx, s.namespace)
		t.Logf("native provider failure: phase=%s readError=%v", failed.Phase, readErr)
	}
	require.NoError(t, err)
	_, completed, err := store.read(ctx, s.namespace)
	require.NoError(t, err)
	require.Equal(t, providerComplete, completed.Phase)
	require.Equal(t, snapshot, completed.Plan.Aggregation)
	options.MTLS.Provider = "external"
	options.Radius.SetArgs = []string{"de.externalIdentity.enabled=true"}
	require.NoError(t, impl.UpgradeRadius(ctx, options, "test"))
	_, reversed, err := store.read(ctx, s.namespace)
	require.NoError(t, err)
	require.Equal(t, providerComplete, reversed.Phase)
	require.NotEqual(t, completed.Plan.ID, reversed.Plan.ID)
	require.Equal(t, snapshot, reversed.Plan.Aggregation)
	require.GreaterOrEqual(t, observed.Load(), int32(6))
	for _, name := range rotationComponents {
		_, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, name+"-mtls", metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err), "retire only the former managed component private keys")
		_, err = s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, name+"-cert", metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err), "external provider must not retain component issuance")
	}
	require.NoError(t, s.checkAggregationTransitionSnapshot(ctx, snapshot))
}
