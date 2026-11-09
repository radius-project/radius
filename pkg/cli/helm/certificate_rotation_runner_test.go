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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

// This models controller reconciliation synchronously. Issuance uses real
// cryptography, but this is not a cert-manager, kubelet, or image acceptance test.
func rotationRunnerFixture(t *testing.T) (*rotationRunner, rotationState) {
	t.Helper()
	oldKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	oldCA, _, oldCert := testSigningCA(t, oldKey, nil, nil, nil)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	next, private, _ := testSigningCA(t, key, nil, nil, nil)
	legacy, _, _ := testSigningCA(t, nil, nil, nil, nil)
	observer, consumers := rotationObserverFixtureForCA(t, oldCA)
	kube := observer.kube.(*kubefake.Clientset)
	ctx, ns, id := t.Context(), observer.namespace, uuid.NewString()
	digest, _, err := mtls.TrustBundleDigest(next, time.Now())
	require.NoError(t, err)
	owned := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uuid.NewString()), ResourceVersion: "1",
			Annotations: map[string]string{"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": ns}}
	}
	signer := &corev1.Secret{ObjectMeta: owned("operator-ca"), Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{"tls.crt": next, "tls.key": private}}
	require.NoError(t, kube.Tracker().Add(signer))
	service := &corev1.Service{ObjectMeta: owned("ucp"), Spec: corev1.ServiceSpec{Selector: map[string]string{"app.kubernetes.io/name": "ucp"}}}
	require.NoError(t, kube.Tracker().Add(service))
	plan := &rotationPlan{TargetMode: "caSecret", TargetTrustMap: "radius-mtls-ca-" + digest[:32], TargetTrustSHA256: digest,
		OldComponentCA: oldCA, SigningSecret: signer.Name, SigningUID: signer.UID,
		Issuer: "radius-ca-" + strings.ReplaceAll(id, "-", "")[:16], ServingSecret: "radius-aggregation-" + strings.ReplaceAll(id, "-", "")[:16],
		ServiceUID: service.UID, ServiceSelector: service.Spec.Selector, UCPProgressDeadlineSeconds: 600}
	api := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": radiusAPIService, "uid": uuid.NewString(), "resourceVersion": "1",
			"annotations": map[string]any{"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": ns}},
		"spec":   map[string]any{"service": map[string]any{"name": "ucp", "namespace": ns}, "caBundle": base64.StdEncoding.EncodeToString(legacy)},
		"status": map[string]any{"conditions": []any{map[string]any{"type": "Available", "status": "True"}}},
	}}
	objects := []runtime.Object{api}
	issue := func(name, component string, parent *x509.Certificate, signer *ecdsa.PrivateKey) {
		crt, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
			c.IsCA, c.KeyUsage = false, x509.KeyUsageDigitalSignature
			c.DNSNames = []string{component + "." + ns + ".svc"}
			c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
			c.NotAfter = time.Now().Add(24 * time.Hour)
		}, parent, signer)
		secret := &corev1.Secret{ObjectMeta: owned(name), Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{"tls.crt": crt, "tls.key": key}}
		existing, err := kube.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			secret.UID = existing.UID
			require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, ns))
		} else {
			require.NoError(t, kube.Tracker().Add(secret))
		}
	}
	for _, consumer := range consumers {
		certUID := types.UID(uuid.NewString())
		plan.Consumers = append(plan.Consumers, rotationWorkload{Name: consumer.Name, UID: consumer.DeploymentUID,
			CertificateUID: certUID, OldIssuer: "radius-ca-issuer", ProofPort: 9090})
		objects = append(objects, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
			"metadata": map[string]any{"name": consumer.Name + "-cert", "namespace": ns, "uid": string(certUID), "resourceVersion": "1",
				"annotations": map[string]any{"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": ns}},
			"spec": map[string]any{"secretName": consumer.Name + "-mtls", "issuerRef": map[string]any{"name": "radius-ca-issuer"}},
		}})
		issue(consumer.Name+"-mtls", consumer.Name, oldCert, oldKey)
		pod, err := kube.CoreV1().Pods(ns).Get(ctx, consumer.Name+"-pod", metav1.GetOptions{})
		require.NoError(t, err)
		pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey] = "initial"
		if consumer.Name == "ucp" {
			deployment, err := kube.AppsV1().Deployments(ns).Get(ctx, consumer.Name, metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes,
				corev1.Volume{Name: "cert", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "ucp-cert"}}})
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{Name: "cert", MountPath: "/var/tls/cert", ReadOnly: true})
			require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, ns))
			pod.Spec = *deployment.Spec.Template.Spec.DeepCopy()
		}
		require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), pod, ns))
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	version := 1
	metadata := func(action k8stesting.Action) (bool, runtime.Object, error) {
		var object runtime.Object
		switch action := action.(type) {
		case k8stesting.CreateAction:
			object = action.GetObject()
		case k8stesting.UpdateAction:
			object = action.GetObject()
		default:
			return false, nil, nil
		}
		accessor, err := meta.Accessor(object)
		require.NoError(t, err)
		if accessor.GetUID() == "" {
			accessor.SetUID(types.UID(uuid.NewString()))
		}
		version++
		accessor.SetResourceVersion(fmt.Sprint(version))
		return false, nil, nil
	}
	kube.PrependReactor("*", "*", metadata)
	client.PrependReactor("*", "*", metadata)
	client.PrependReactor("*", "certificates", func(action k8stesting.Action) (bool, runtime.Object, error) {
		var object *unstructured.Unstructured
		switch action := action.(type) {
		case k8stesting.CreateAction:
			object = action.GetObject().(*unstructured.Unstructured)
		case k8stesting.UpdateAction:
			object = action.GetObject().(*unstructured.Unstructured)
		default:
			return false, nil, nil
		}
		name, _, err := unstructured.NestedString(object.Object, "spec", "secretName")
		require.NoError(t, err)
		component := strings.TrimSuffix(name, "-mtls")
		if strings.HasPrefix(name, "radius-aggregation-") {
			component = "ucp"
		}
		issuerName, _, err := unstructured.NestedString(object.Object, "spec", "issuerRef", "name")
		require.NoError(t, err)
		issuerObject, err := client.Tracker().Get(rotationIssuerResource, ns, issuerName)
		require.NoError(t, err)
		signingName, _, err := unstructured.NestedString(issuerObject.(*unstructured.Unstructured).Object, "spec", "ca", "secretName")
		require.NoError(t, err)
		signing, err := kube.CoreV1().Secrets(ns).Get(ctx, signingName, metav1.GetOptions{})
		require.NoError(t, err)
		pair, err := tls.X509KeyPair(signing.Data["tls.crt"], signing.Data["tls.key"])
		require.NoError(t, err)
		ca, err := x509.ParseCertificate(pair.Certificate[0])
		require.NoError(t, err)
		issue(name, component, ca, pair.PrivateKey.(*ecdsa.PrivateKey))
		return false, nil, nil
	})
	kube.PrependReactor("update", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deployment := action.(k8stesting.UpdateAction).GetObject().(*appsv1.Deployment)
		deployment.Generation++
		template, err := json.Marshal(deployment.Spec.Template)
		require.NoError(t, err)
		sum := sha256.Sum256(template)
		hash := hex.EncodeToString(sum[:4])
		name := deployment.Name + "-" + hash
		held := deployment.Name == "ucp" && deployment.Spec.MinReadySeconds == math.MaxInt32-1
		podObjects, err := kube.Tracker().List(corev1.SchemeGroupVersion.WithResource("pods"), corev1.SchemeGroupVersion.WithKind("Pod"), ns)
		require.NoError(t, err)
		pods := podObjects.(*corev1.PodList)
		if !held {
			for _, pod := range pods.Items {
				if pod.Labels["app.kubernetes.io/name"] == deployment.Name && pod.Name != name+"-pod" {
					require.NoError(t, kube.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), ns, pod.Name))
				}
			}
			setObjects, err := kube.Tracker().List(appsv1.SchemeGroupVersion.WithResource("replicasets"), appsv1.SchemeGroupVersion.WithKind("ReplicaSet"), ns)
			require.NoError(t, err)
			sets := setObjects.(*appsv1.ReplicaSetList)
			for _, set := range sets.Items {
				if owner := metav1.GetControllerOf(&set); owner != nil && owner.UID == deployment.UID && set.Name != name {
					require.NoError(t, kube.Tracker().Delete(appsv1.SchemeGroupVersion.WithResource("replicasets"), ns, set.Name))
				}
			}
		}
		_, err = kube.Tracker().Get(corev1.SchemeGroupVersion.WithResource("pods"), ns, name+"-pod")
		if err != nil {
			set := &appsv1.ReplicaSet{ObjectMeta: owned(name), Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To(int32(1)), Template: *deployment.Spec.Template.DeepCopy()},
				Status: appsv1.ReplicaSetStatus{Replicas: 1}}
			set.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: deployment.Name, UID: deployment.UID, Controller: ptr.To(true)}}
			require.NoError(t, kube.Tracker().Add(set))
			pod := &corev1.Pod{ObjectMeta: owned(name + "-pod"), Spec: *deployment.Spec.Template.Spec.DeepCopy(),
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
					Name: deployment.Spec.Template.Spec.Containers[0].Name, Ready: true, ContainerID: "containerd://simulated-" + hash,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}},
				}}}}
			pod.Labels = map[string]string{"app.kubernetes.io/name": deployment.Name, appsv1.DefaultDeploymentUniqueLabelKey: hash}
			pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: name, UID: set.UID, Controller: ptr.To(true)}}
			require.NoError(t, kube.Tracker().Add(pod))
		}
		deployment.Status = appsv1.DeploymentStatus{ObservedGeneration: deployment.Generation, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
		if held {
			deployment.Status.Replicas, deployment.Status.ReadyReplicas = 2, 2
		}
		return false, nil, nil
	})
	store := &rotationStore{kube: kube, dynamic: client, namespace: ns, release: radiusReleaseName,
		now: time.Now, guardWrite: func(context.Context) error { return nil }}
	state, err := store.beginPlanned(ctx, id, next, plan)
	require.NoError(t, err)
	runner := &rotationRunner{store: store}
	runner.observe = func(ctx context.Context, _ *rest.Config, target TLSProofTarget) (PodTLSProof, error) {
		pod, err := kube.CoreV1().Pods(ns).Get(ctx, target.PodName, metav1.GetOptions{})
		if err != nil {
			return PodTLSProof{}, err
		}
		var trustName, secretName string
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == "mtls" && volume.Projected != nil {
				secretName, trustName = volume.Projected.Sources[0].Secret.Name, volume.Projected.Sources[1].ConfigMap.Name
			}
		}
		trust, err := kube.CoreV1().ConfigMaps(ns).Get(ctx, trustName, metav1.GetOptions{})
		if err != nil {
			return PodTLSProof{}, err
		}
		digest, roots, err := mtls.TrustBundleDigest([]byte(trust.Data["ca.crt"]), time.Now())
		if err != nil {
			return PodTLSProof{}, err
		}
		secret, err := kube.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return PodTLSProof{}, err
		}
		block, _ := pem.Decode(secret.Data["tls.crt"])
		leaf, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return PodTLSProof{}, err
		}
		leafHash := sha256.Sum256(leaf.Raw)
		return PodTLSProof{Evidence: mtls.StartupEvidence{SchemaVersion: 1, Ready: true, ProofKind: "startup-tls-initialized",
			PodUID: string(pod.UID), BootID: uuid.NewString(), InitializedAt: time.Now().Add(-time.Second),
			TrustBundleSHA256: digest, CASHA256: roots, LeafSHA256: hex.EncodeToString(leafHash[:]), NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter}}, nil
	}
	runner.probe = func(ctx context.Context, _ *rest.Config, expected map[types.UID]bool) error {
		service, err := kube.CoreV1().Services(ns).Get(ctx, "ucp", metav1.GetOptions{})
		if err != nil {
			return err
		}
		pods, err := kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=ucp"})
		if err != nil {
			return err
		}
		hash := service.Spec.Selector[appsv1.DefaultDeploymentUniqueLabelKey]
		for _, pod := range pods.Items {
			if (hash == "" || pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey] == hash) && expected[pod.UID] {
				return nil
			}
		}
		return errors.New("simulated aggregator reached an old backend")
	}
	return runner, state
}

func TestRotationPhasesWithSimulatedControllers(t *testing.T) {
	runner, state := rotationRunnerFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue} {
		require.NoError(t, runner.run(ctx, state.ID, phase), phase)
	}
	_, ready, err := runner.store.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationReady, ready.Phase, "issuance cannot authorize retirement")
	api, err := runner.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(api)
	require.NoError(t, err)
	_, roots, err := mtls.TrustBundleDigest(bundle, time.Now())
	require.NoError(t, err)
	require.Len(t, roots, 2)
	require.NoError(t, runner.run(ctx, state.ID, rotationIssue), "completed phase retry does not retire trust")
	require.NoError(t, runner.run(ctx, state.ID, rotationRetire))
	_, complete, err := runner.store.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationComplete, complete.Phase)
	require.NoError(t, runner.verifyConsumers(ctx, complete))
	require.NoError(t, runner.verifyAggregation(ctx, complete))
	require.NoError(t, runner.run(ctx, state.ID, rotationRetire))
	issuers, err := runner.store.dynamic.Resource(rotationIssuerResource).Namespace(runner.store.namespace).Get(ctx, state.Plan.Issuer, metav1.GetOptions{})
	require.NoError(t, err)
	require.Empty(t, issuers.GetOwnerReferences())
	require.Empty(t, issuers.GetAnnotations()["meta.helm.sh/release-name"])
}

func TestRotationInterruptedAdoption(t *testing.T) {
	runner, state := rotationRunnerFixture(t)
	require.NoError(t, runner.run(t.Context(), state.ID, rotationPublish))
	observe := runner.observe
	ctx, cancel := context.WithCancel(t.Context())
	runner.observe = func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error) {
		cancel()
		return PodTLSProof{}, errors.New("simulated consumer has not loaded trust")
	}
	require.ErrorContains(t, runner.run(ctx, state.ID, rotationAdopt), "interrupted")
	_, current, err := runner.store.read(t.Context())
	require.NoError(t, err)
	require.Equal(t, rotationAdopt, current.Phase)
	runner.observe = observe
	require.NoError(t, runner.run(t.Context(), state.ID, rotationAdopt))
	_, current, err = runner.store.read(t.Context())
	require.NoError(t, err)
	require.Equal(t, rotationIssue, current.Phase)
}

func TestRotationInterruptedAggregation(t *testing.T) {
	runner, state := rotationRunnerFixture(t)
	for _, phase := range []rotationPhase{rotationPublish, rotationAdopt} {
		require.NoError(t, runner.run(t.Context(), state.ID, phase))
	}
	probe := runner.probe
	ctx, cancel := context.WithCancel(t.Context())
	runner.probe = func(context.Context, *rest.Config, map[types.UID]bool) error {
		cancel()
		return errors.New("simulated aggregator still uses its prior trust")
	}
	require.ErrorContains(t, runner.run(ctx, state.ID, rotationIssue), "interrupted")
	service, err := runner.store.kube.CoreV1().Services(runner.store.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "old", service.Annotations[rotationRoute])
	deployment, err := runner.store.kube.AppsV1().Deployments(runner.store.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(math.MaxInt32-1), deployment.Spec.MinReadySeconds)
	_, current, err := runner.store.read(t.Context())
	require.NoError(t, err)
	require.Equal(t, rotationIssue, current.Phase)
	runner.probe = probe
	ctx, cancel = context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	require.NoError(t, runner.run(ctx, state.ID, rotationIssue))
	_, current, err = runner.store.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationReady, current.Phase)
}

func TestRotationIncidentRecoveryWithSimulatedControllers(t *testing.T) {
	runner, original := rotationRunnerFixture(t)
	for _, phase := range []rotationPhase{rotationPublish, rotationAdopt} {
		require.NoError(t, runner.run(t.Context(), original.ID, phase))
	}
	journal, _, err := runner.store.read(t.Context())
	require.NoError(t, err)
	id := uuid.NewString()
	plan := original.Plan
	plan.Issuer = "radius-ca-" + strings.ReplaceAll(id, "-", "")[:16]
	plan.ServingSecret = "radius-aggregation-" + strings.ReplaceAll(id, "-", "")[:16]
	recovery, err := runner.store.beginRecovery(t.Context(), id, original.NewCA, plan, journal)
	require.NoError(t, err)
	require.True(t, recovery.Recovery)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	require.NoError(t, runner.run(ctx, recovery.ID, rotationRecover))
	_, completed, err := runner.store.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationComplete, completed.Phase)
	require.True(t, completed.Recovery)
	require.NoError(t, runner.verifyConsumers(ctx, completed))
	api, err := runner.store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(api)
	require.NoError(t, err)
	require.Equal(t, original.NewCA, bundle)
	require.Error(t, runner.run(ctx, original.ID, rotationIssue), "superseded transactions cannot resume")
}

func TestRotationRecoveryPlanning(t *testing.T) {
	for _, invalidSelector := range []bool{false, true} {
		t.Run(fmt.Sprint(invalidSelector), func(t *testing.T) {
			runner, original := rotationRunnerFixture(t)
			store, ctx := runner.store, t.Context()
			kube := store.kube.(*kubefake.Clientset)
			for _, name := range rotationComponents {
				deployment, err := kube.AppsV1().Deployments(store.namespace).Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				port := "metrics"
				if name == "bicep-de" {
					port = "bicep-de-api"
				}

				deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: port, ContainerPort: 9090}}
				deployment.Spec.ProgressDeadlineSeconds = ptr.To(int32(600))
				if name == "ucp" {
					deployment.Spec.MinReadySeconds = math.MaxInt32 - 1
					deployment.Spec.ProgressDeadlineSeconds = ptr.To(int32(math.MaxInt32))
				}
				require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, store.namespace))
			}
			service, err := kube.CoreV1().Services(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
			require.NoError(t, err)
			if invalidSelector {
				service.Spec.Selector["unexpected"] = "operator-change"
			} else {
				service.Spec.Selector[appsv1.DefaultDeploymentUniqueLabelKey] = "initial"
				service.Annotations[rotationTransaction] = original.ID
				service.Annotations[rotationRevision] = original.revision()
				service.Annotations[rotationRoute] = "old"
				service.Annotations[rotationOldHash] = "initial"
			}
			require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("services"), service, store.namespace))
			kube.ClearActions()
			dynamic := store.dynamic.(*dynamicfake.FakeDynamicClient)
			dynamic.ClearActions()
			id := uuid.NewString()
			plan, err := planRotationResources(ctx, store, nil, id, original.Plan.SigningSecret,
				"caSecret", original.Plan.OldComponentCA, original.NewCA, nil, &original)
			if invalidSelector {
				require.ErrorContains(t, err, "selector changed outside")
			} else {
				require.NoError(t, err)
				require.Len(t, plan.Consumers, 5)
				require.Equal(t, original.Plan.ServiceSelector, plan.ServiceSelector)
				require.Equal(t, original.Plan.UCPMinReadySeconds, plan.UCPMinReadySeconds)
				require.Equal(t, original.Plan.UCPProgressDeadlineSeconds, plan.UCPProgressDeadlineSeconds)
				require.NoError(t, plan.validate())
			}
			for _, action := range append(kube.Actions(), dynamic.Actions()...) {
				require.Contains(t, []string{"get", "list"}, action.GetVerb(), "planning must not write")
			}
			require.ErrorContains(t, checkRecoveryTarget(original.OldCA, original), "cannot restore")
			require.NoError(t, checkRecoveryTarget(original.NewCA, original))
		})
	}
}

func TestRotationConsecutiveDrivers(t *testing.T) {
	runner, state := rotationRunnerFixture(t)
	ctx, store := t.Context(), runner.store
	kube := store.kube.(*kubefake.Clientset)
	dynamic := store.dynamic.(*dynamicfake.FakeDynamicClient)
	for cycle := range 2 {
		for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire} {
			require.NoError(t, runner.run(ctx, state.ID, phase), "cycle %d phase %s", cycle, phase)
		}
		_, complete, err := store.read(ctx)
		require.NoError(t, err)
		require.Equal(t, rotationComplete, complete.Phase)
		if cycle == 1 {
			break
		}
		// Model the completion metadata written by the separately tested shipping
		// Helm templates before the next command can start a transaction.
		for _, workload := range complete.Plan.Consumers {
			deployment, err := kube.AppsV1().Deployments(store.namespace).Get(ctx, workload.Name, metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Annotations[rotationRevision] = complete.revision()
			require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, store.namespace))
			certificate, err := dynamic.Resource(rotationCertificateResource).Namespace(store.namespace).Get(ctx, workload.Name+"-cert", metav1.GetOptions{})
			require.NoError(t, err)
			annotations := certificate.GetAnnotations()
			annotations[rotationRevision] = complete.revision()
			certificate.SetAnnotations(annotations)
			require.NoError(t, dynamic.Tracker().Update(rotationCertificateResource, certificate, store.namespace))
		}
		service, err := kube.CoreV1().Services(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
		require.NoError(t, err)
		service.Annotations[rotationRevision] = complete.revision()
		require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("services"), service, store.namespace))
		next, private, _ := testSigningCA(t, nil, nil, nil, nil)
		signer := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "next-operator-ca", Namespace: store.namespace},
			Type: corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": next, "tls.key": private}}
		signer, err = kube.CoreV1().Secrets(store.namespace).Create(ctx, signer, metav1.CreateOptions{})
		require.NoError(t, err)
		id := uuid.NewString()
		plan := *complete.Plan
		plan.Consumers = append([]rotationWorkload(nil), complete.Plan.Consumers...)
		for index := range plan.Consumers {
			plan.Consumers[index].OldIssuer = complete.Plan.Issuer
		}
		digest, _, err := mtls.TrustBundleDigest(next, time.Now())
		require.NoError(t, err)
		plan.OldComponentCA = complete.NewCA
		plan.SigningSecret, plan.SigningUID = signer.Name, signer.UID
		plan.TargetTrustMap, plan.TargetTrustSHA256 = "radius-mtls-ca-"+digest[:32], digest
		plan.Issuer, plan.ServingSecret = "radius-ca-"+strings.ReplaceAll(id, "-", "")[:16], "radius-aggregation-"+strings.ReplaceAll(id, "-", "")[:16]
		state, err = store.beginPlanned(ctx, id, next, &plan)
		require.NoError(t, err)
		require.Equal(t, complete.revision(), state.PreviousRevision)
	}
}
