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

package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	radapp "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func referenceDeployment(t *testing.T) *radapp.DeploymentTemplate {
	t.Helper()
	provider, err := sdkclients.GenerateProviderConfig("default", "", "").String()
	require.NoError(t, err)
	return &radapp.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "app", Generation: 1},
		Spec: radapp.DeploymentTemplateSpec{
			Template:   `{"parameters":{"password":{"type":"secureString"},"application":{"type":"string"}},"resources":[]}`,
			Parameters: map[string]string{"application": "orders"},
			ParameterReferences: map[string]radapp.ParameterReference{
				"password": {SecretKeyRef: radapp.SecretKeyReference{Name: "credentials", Key: "password"}},
			},
			ProviderConfig: provider,
		},
	}
}

func secretTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, radapp.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&radapp.DeploymentTemplate{}).WithObjects(objects...).Build()
}

type countingReader struct {
	client.Reader
	secrets int
	fail    bool
}

func (r *countingReader) Get(ctx context.Context, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
	if _, ok := object.(*corev1.Secret); ok {
		r.secrets++
		if r.fail {
			return errors.New("protected-api-error-resolved-secret-marker")
		}
	}
	return r.Reader.Get(ctx, key, object, options...)
}

func resolverForTest(reader client.Reader) *SecretInputResolver {
	return &SecretInputResolver{accountReader: reader, newReader: func(string) (client.Reader, error) { return reader, nil }}
}

func TestSecretInputsFreshReadsAndRevision(t *testing.T) {
	deployment := referenceDeployment(t)
	deployment.Spec.Template = `{"parameters":{"password":{"type":"secureString"},"other":{"type":"secureString"},"application":{"type":"string"}}}`
	deployment.Spec.ParameterReferences["other"] = radapp.ParameterReference{
		SecretKeyRef: radapp.SecretKeyReference{Name: "credentials", Key: "empty"},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "app", UID: "original"},
		Data: map[string][]byte{"password": []byte(" \nresolved-secret-marker "), "empty": {}}}
	c := secretTestClient(t, secret, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: secretReaderAccount, Namespace: "app"}})
	reader := &countingReader{Reader: c}
	resolver := resolverForTest(reader)
	original := deployment.DeepCopy()
	values, revision, err := resolver.Resolve(t.Context(), deployment)
	require.NoError(t, err)
	require.Equal(t, " \nresolved-secret-marker ", values["password"]["value"])
	require.Equal(t, "", values["other"]["value"])
	require.Equal(t, 1, reader.secrets)
	require.Equal(t, original, deployment)

	values, sameRevision, err := resolver.Resolve(t.Context(), deployment)
	require.NoError(t, err)
	require.Equal(t, revision, sameRevision)
	require.Equal(t, 2, reader.secrets)
	require.Equal(t, "orders", values["application"]["value"])
	current := &corev1.Secret{}
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(secret), current))
	current.Data["password"] = []byte("rotated")
	require.NoError(t, c.Update(t.Context(), current))
	_, updatedRevision, err := resolver.Resolve(t.Context(), deployment)
	require.NoError(t, err)
	require.NotEqual(t, revision, updatedRevision)
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(secret), current))
	current.Labels = map[string]string{"metadata": "only"}
	require.NoError(t, c.Update(t.Context(), current))
	_, metadataRevision, err := resolver.Resolve(t.Context(), deployment)
	require.NoError(t, err)
	require.NotEqual(t, updatedRevision, metadataRevision)
	reader.fail = true
	_, _, err = resolver.Resolve(t.Context(), deployment)
	require.ErrorContains(t, err, "SecretInputUnavailable")
	require.NotContains(t, err.Error(), "resolved-secret-marker")
}

func TestSecretInputsUnavailableAndInvalid(t *testing.T) {
	for _, test := range []struct {
		name    string
		account bool
		secret  bool
		data    map[string][]byte
	}{
		{"missing account", false, true, map[string][]byte{"password": []byte("resolved-secret-marker")}},
		{"missing Secret", true, false, nil},
		{"missing key", true, true, map[string][]byte{"other": []byte("resolved-secret-marker")}},
		{"invalid UTF8", true, true, map[string][]byte{"password": {0xff}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			objects := []client.Object{}
			if test.account {
				objects = append(objects, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: secretReaderAccount, Namespace: "app"}})
			}
			if test.secret {
				objects = append(objects, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "app"}, Data: test.data})
			}
			reader := &countingReader{Reader: secretTestClient(t, objects...)}
			_, _, err := resolverForTest(reader).Resolve(t.Context(), referenceDeployment(t))
			require.Error(t, err)
			require.NotContains(t, err.Error(), "resolved-secret-marker")
			if !test.account {
				require.Zero(t, reader.secrets)
			}
		})
	}
}

type inputDeploymentClient struct {
	sdkclients.ResourceDeploymentsClient
	put func(context.Context, sdkclients.Deployment, string) error
	get func(string) (sdkclients.ClientCreateOrUpdateResponse, error)
}

func (c *inputDeploymentClient) CreateOrUpdate(ctx context.Context, deployment sdkclients.Deployment, id, version string) (sdkclients.Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	return nil, c.put(ctx, deployment, id)
}

func (c *inputDeploymentClient) GetDeployment(_ context.Context, id, version string) (sdkclients.ClientCreateOrUpdateResponse, error) {
	return c.get(id)
}

type failingStatusClient struct {
	client.Client
	updates int
	failAt  int
}

func (c *failingStatusClient) Status() client.SubResourceWriter {
	return &failingStatusWriter{SubResourceWriter: c.Client.Status(), owner: c}
}

type failingStatusWriter struct {
	client.SubResourceWriter
	owner *failingStatusClient
}

func (w *failingStatusWriter) Update(ctx context.Context, object client.Object, options ...client.SubResourceUpdateOption) error {
	w.owner.updates++
	if w.owner.updates == w.owner.failAt {
		return errors.New("injected status write failure")
	}
	return w.SubResourceWriter.Update(ctx, object, options...)
}

func TestSecretSubmissionRecoveryRotationAndRetry(t *testing.T) {
	deployment := referenceDeployment(t)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "app", UID: "first"},
		Data: map[string][]byte{"password": []byte("resolved-secret-marker")}}
	base := secretTestClient(t, deployment, secret, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: secretReaderAccount, Namespace: "app"}})
	// The first status write saves intent; the second (after PUT) loses the acknowledgement.
	c := &failingStatusClient{Client: base, failAt: 2}
	puts := 0
	state := armdeployments.ProvisioningStateRunning
	var submittedID string
	engine := &inputDeploymentClient{
		put: func(ctx context.Context, request sdkclients.Deployment, id string) error {
			puts++
			submittedID = id
			stored := &radapp.DeploymentTemplate{}
			require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(deployment), stored))
			require.Equal(t, id, stored.Status.PendingSubmission)
			require.NotEmpty(t, stored.Status.AttemptedInputRevision)
			if puts == 1 {
				require.Empty(t, stored.Status.AppliedInputRevision)
			}
			require.Equal(t, "resolved-secret-marker", request.Properties.Parameters.(map[string]map[string]string)["password"]["value"])
			return nil
		},
		get: func(id string) (sdkclients.ClientCreateOrUpdateResponse, error) {
			require.Equal(t, submittedID, id)
			return sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armdeployments.DeploymentExtended{
				Properties: &armdeployments.DeploymentPropertiesExtended{ProvisioningState: &state},
			}}, nil
		},
	}
	reconciler := &DeploymentTemplateReconciler{
		Client: c, Scheme: base.Scheme(), EventRecorder: record.NewFakeRecorder(50), Radius: NewMockRadiusClient(),
		ResourceDeploymentsClient: engine, SecretInputs: resolverForTest(base),
	}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}}
	_, err := reconciler.Reconcile(t.Context(), request)
	require.ErrorContains(t, err, "status write failure")
	require.Equal(t, 1, puts)
	stored := &radapp.DeploymentTemplate{}
	require.NoError(t, base.Get(t.Context(), request.NamespacedName, stored))
	attempted := stored.Status.AttemptedInputRevision
	require.NotEmpty(t, stored.Status.PendingSubmission)
	// A restarted reconciler recovers the deployment through GET without a new PUT.
	result, err := reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.NotZero(t, result.RequeueAfter)
	require.Equal(t, 1, puts)
	require.NoError(t, base.Get(t.Context(), request.NamespacedName, stored))
	require.Empty(t, stored.Status.PendingSubmission)
	require.Empty(t, stored.Status.Operation.ResumeToken)
	require.Equal(t, submittedID, stored.Status.Operation.DeploymentID)
	require.Empty(t, stored.Status.AppliedInputRevision)

	state = armdeployments.ProvisioningStateSucceeded
	result, err = reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.NotZero(t, result.RequeueAfter)
	require.Equal(t, 1, puts)
	require.NoError(t, base.Get(t.Context(), request.NamespacedName, stored))
	require.Equal(t, attempted, stored.Status.AppliedInputRevision)
	require.Equal(t, radapp.DeploymentTemplatePhraseReady, stored.Status.Phrase)
	serialized, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "resolved-secret-marker")
	// A rotation with unchanged spec starts a new attempt; the applied revision stays old.
	require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(secret), secret))
	secret.Labels = map[string]string{"rotation": "metadata-only"}
	require.NoError(t, base.Update(t.Context(), secret))
	_, err = reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, 2, puts)
	require.NoError(t, base.Get(t.Context(), request.NamespacedName, stored))
	require.NotEqual(t, attempted, stored.Status.AttemptedInputRevision)
	require.Equal(t, attempted, stored.Status.AppliedInputRevision)
	// A failed deployment must retry even when the previous successful spec hash matches.
	state = armdeployments.ProvisioningStateFailed
	_, err = reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, 3, puts)
	require.NoError(t, base.Get(t.Context(), request.NamespacedName, stored))
	require.Equal(t, attempted, stored.Status.AppliedInputRevision)
}

func TestSecretSubmissionUncertainFailsClosed(t *testing.T) {
	deployment := referenceDeployment(t)
	c := secretTestClient(t, deployment,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: secretReaderAccount, Namespace: "app"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "app"},
			Data: map[string][]byte{"password": []byte("resolved-secret-marker")}})
	events := record.NewFakeRecorder(20)
	puts := 0
	engine := &inputDeploymentClient{
		put: func(context.Context, sdkclients.Deployment, string) error {
			puts++
			return errors.New("HTTP response echoes resolved-secret-marker")
		},
		get: func(string) (sdkclients.ClientCreateOrUpdateResponse, error) {
			return sdkclients.ClientCreateOrUpdateResponse{}, errors.New("lookup echoes resolved-secret-marker")
		},
	}
	r := &DeploymentTemplateReconciler{
		Client: c, Scheme: c.Scheme(), Radius: NewMockRadiusClient(), EventRecorder: events,
		SecretInputs: resolverForTest(c), ResourceDeploymentsClient: engine,
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(deployment)}
	for range 3 {
		_, err := r.Reconcile(t.Context(), request)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "resolved-secret-marker")
	}
	require.Equal(t, 1, puts)
	stored := &radapp.DeploymentTemplate{}
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, stored))
	require.Equal(t, radapp.DeploymentTemplatePhraseFailed, stored.Status.Phrase)
	require.NotEmpty(t, stored.Status.PendingSubmission)
	require.Empty(t, stored.Status.AppliedInputRevision)
	data, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(data), "resolved-secret-marker")
	for len(events.Events) > 0 {
		require.NotContains(t, <-events.Events, "resolved-secret-marker")
	}
}

func TestSecretInputFailureDoesNotReusePreviousSuccess(t *testing.T) {
	deployment := referenceDeployment(t)
	hash, err := computeHash(deployment)
	require.NoError(t, err)
	deployment.Status.StatusHash = hash
	deployment.Status.Phrase = radapp.DeploymentTemplatePhraseReady
	deployment.Status.AppliedInputRevision = "previous-success"
	c := secretTestClient(t, deployment)
	r := &DeploymentTemplateReconciler{
		Client: c, Scheme: c.Scheme(), EventRecorder: record.NewFakeRecorder(20),
		SecretInputs: resolverForTest(c),
		// No engine client is configured: any submission would panic.
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(deployment)}
	_, err = r.Reconcile(t.Context(), request)
	require.ErrorContains(t, err, "SecretInputUnavailable")
	stored := &radapp.DeploymentTemplate{}
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, stored))
	require.Equal(t, radapp.DeploymentTemplatePhraseFailed, stored.Status.Phrase)
	require.Equal(t, "previous-success", stored.Status.AppliedInputRevision)
	require.Nil(t, stored.Status.Operation)
}

func TestSecretOperationOutputTrackingRetry(t *testing.T) {
	deployment := referenceDeployment(t)
	deployment.UID = "deployment-uid"
	deployment.Status.Operation = &radapp.ResourceOperation{OperationKind: radapp.OperationKindPut, DeploymentID: "/deployment"}
	deployment.Status.AttemptedInputRevision = "attempt"
	deployment.Status.ObservedGeneration = deployment.Generation
	base := secretTestClient(t, deployment)
	c := &failingStatusClient{Client: base, failAt: 1}
	state := armdeployments.ProvisioningStateSucceeded
	id := "/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/test"
	r := &DeploymentTemplateReconciler{
		Client: c, Scheme: base.Scheme(), EventRecorder: record.NewFakeRecorder(10),
		ResourceDeploymentsClient: &inputDeploymentClient{
			get: func(string) (sdkclients.ClientCreateOrUpdateResponse, error) {
				return sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armdeployments.DeploymentExtended{
					Properties: &armdeployments.DeploymentPropertiesExtended{
						ProvisioningState: &state, OutputResources: []*armdeployments.ResourceReference{{ID: &id}},
					},
				}}, nil
			},
		},
	}
	// Output tracking succeeded, but saving the applied revision failed.
	_, err := r.reconcileOperation(t.Context(), deployment)
	require.ErrorContains(t, err, "status write failure")
	require.NoError(t, base.Get(t.Context(), client.ObjectKeyFromObject(deployment), deployment))
	require.Empty(t, deployment.Status.AppliedInputRevision)
	_, err = r.reconcileOperation(t.Context(), deployment)
	require.NoError(t, err)
	require.Equal(t, "attempt", deployment.Status.AppliedInputRevision)
	resources := &radapp.DeploymentResourceList{}
	require.NoError(t, base.List(t.Context(), resources))
	require.Len(t, resources.Items, 1)
}

func TestSecretRotationDuringOperationAndDeletion(t *testing.T) {
	deployment := referenceDeployment(t)
	deployment.Status.Operation = &radapp.ResourceOperation{OperationKind: radapp.OperationKindPut, DeploymentID: "/deployment"}
	deployment.Status.AttemptedInputRevision = "old-attempt"
	deployment.Status.ObservedGeneration = deployment.Generation
	deployment.Finalizers = []string{DeploymentTemplateFinalizer}
	c := secretTestClient(t, deployment)
	state := armdeployments.ProvisioningStateRunning
	r := &DeploymentTemplateReconciler{
		Client: c, Scheme: c.Scheme(), EventRecorder: record.NewFakeRecorder(20),
		ResourceDeploymentsClient: &inputDeploymentClient{
			get: func(string) (sdkclients.ClientCreateOrUpdateResponse, error) {
				return sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armdeployments.DeploymentExtended{
					Properties: &armdeployments.DeploymentPropertiesExtended{ProvisioningState: &state},
				}}, nil
			},
		},
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(deployment)}
	// No input resolver exists, so an input read while tracking the operation would fail.
	_, err := r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	state = armdeployments.ProvisioningStateSucceeded
	_, err = r.Reconcile(t.Context(), request)
	require.ErrorContains(t, err, "SecretInputUnavailable")
	require.NoError(t, c.Get(t.Context(), request.NamespacedName, deployment))
	require.Equal(t, "old-attempt", deployment.Status.AppliedInputRevision)
	require.Equal(t, radapp.DeploymentTemplatePhraseFailed, deployment.Status.Phrase)
	// Deletion must finish without resolving now-unavailable inputs.
	require.NoError(t, c.Delete(t.Context(), deployment))
	_, err = r.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.Error(t, c.Get(t.Context(), request.NamespacedName, &radapp.DeploymentTemplate{}))
}
