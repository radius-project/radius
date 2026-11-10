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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type coordinatorTestClient struct {
	sdkclients.ResourceDeploymentsClient
	putCalls      atomic.Int64
	deleteCalls   atomic.Int64
	deleteRequest func(context.Context, string, string) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error)
}

func (c *coordinatorTestClient) CreateOrUpdate(ctx context.Context, d sdkclients.Deployment, id, version string) (sdkclients.Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	c.putCalls.Add(1)
	return c.ResourceDeploymentsClient.CreateOrUpdate(ctx, d, id, version)
}

func (c *coordinatorTestClient) Delete(ctx context.Context, id, version string) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error) {
	c.deleteCalls.Add(1)
	if c.deleteRequest != nil {
		return c.deleteRequest(ctx, id, version)
	}
	return c.ResourceDeploymentsClient.Delete(ctx, id, version)
}

func coordinatorFixture(t *testing.T) (*DeploymentTemplateReconciler, *radappiov1alpha3.DeploymentTemplate, *coordinatorTestClient, *sdkclients.MockResourceDeploymentsClient) {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, radappiov1alpha3.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	config, err := sdkclients.NewDefaultProviderConfig("test").String()
	require.NoError(t, err)
	owner := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "test", UID: "owner", Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}", ProviderConfig: config},
		Status:     radappiov1alpha3.DeploymentTemplateStatus{Coordinator: &radappiov1alpha3.DeploymentCoordinatorStatus{Phase: coordinatorIdle}},
	}
	owner.Status.StatusHash, err = computeHash(owner)
	require.NoError(t, err)
	k8s := fake.NewClientBuilder().WithScheme(s).WithObjects(owner, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}}).WithStatusSubresource(owner, &radappiov1alpha3.DeploymentResource{}).Build()
	mock := sdkclients.NewMockResourceDeploymentsClient()
	sdk := &coordinatorTestClient{ResourceDeploymentsClient: mock}
	r := &DeploymentTemplateReconciler{Client: k8s, APIReader: k8s, Scheme: s, Radius: NewMockRadiusClient(), ResourceDeploymentsClient: sdk, EventRecorder: record.NewFakeRecorder(100)}
	return r, owner, sdk, mock
}

func coordinatorStep(t *testing.T, r *DeploymentTemplateReconciler, owner *radappiov1alpha3.DeploymentTemplate) {
	t.Helper()
	_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(owner)})
	require.NoError(t, err)
	require.NoError(t, r.Client.Get(t.Context(), client.ObjectKeyFromObject(owner), owner))
}

func Test_CoordinatorHeldDeletePreventsReuse(t *testing.T) {
	r, owner, sdk, mock := coordinatorFixture(t)
	id := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/environment"
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: id, State: outputPendingDelete}}
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	sdk.deleteRequest = func(ctx context.Context, id, version string) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error) {
		persisted := &radappiov1alpha3.DeploymentTemplate{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(owner), persisted); err != nil {
			return nil, err
		}
		if persisted.Status.Coordinator.Phase != coordinatorDeleteDispatching {
			return nil, errors.New("DELETE dispatched without durable intent")
		}
		close(entered)
		<-release
		return mock.Delete(ctx, id, version)
	}
	result := make(chan error, 1)
	go func() {
		_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(owner)})
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second * 5):
		t.Fatal("DELETE was not dispatched")
	}
	require.NoError(t, r.Client.Get(t.Context(), client.ObjectKeyFromObject(owner), owner))
	owner.Spec.Template = `{"resources":[]}`
	require.NoError(t, r.Client.Update(t.Context(), owner))
	// A restarted coordinator cannot interpret the missing resume token as no DELETE.
	restarted := *r
	coordinatorStep(t, &restarted, owner)
	require.Equal(t, coordinatorBlocked, owner.Status.Coordinator.Phase)
	require.Equal(t, id, owner.Status.Coordinator.ResourceID)
	require.Zero(t, sdk.putCalls.Load())
	// Release exactly once; wait before accessing the original reconciler's result.
	release <- struct{}{}
	require.Error(t, <-result, "the old request must not overwrite the restarted coordinator's decision")
	coordinatorStep(t, &restarted, owner)
	require.Zero(t, sdk.putCalls.Load())
	require.EqualValues(t, 1, sdk.deleteCalls.Load())
}

func Test_CoordinatorDispatchRequiresPersistedSchema(t *testing.T) {
	for _, kind := range []string{"PUT", "DELETE"} {
		t.Run(kind, func(t *testing.T) {
			r, owner, sdk, _ := coordinatorFixture(t)
			if kind == "PUT" {
				owner.Status.StatusHash = ""
			} else {
				owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output", State: outputPendingDelete}}
			}
			require.NoError(t, r.Client.Status().Update(t.Context(), owner))
			underlying := r.Client.(client.WithWatch)
			pruning := interceptor.NewClient(underlying, interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					if template, ok := obj.(*radappiov1alpha3.DeploymentTemplate); ok {
						template.Status.Coordinator = nil
					}
					return c.SubResource(sub).Update(ctx, obj, opts...)
				},
			})
			r.Client = pruning
			_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(owner)})
			require.ErrorContains(t, err, "not persisted")
			require.Zero(t, sdk.putCalls.Load())
			require.Zero(t, sdk.deleteCalls.Load())
		})
	}
}

func Test_CoordinatorAdoptsLegacyDelete(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(fmt.Sprint(held), func(t *testing.T) { testCoordinatorAdoptsLegacyDelete(t, held) })
	}
}

func testCoordinatorAdoptsLegacyDelete(t *testing.T, held bool) {
	t.Helper()
	r, owner, sdk, mock := coordinatorFixture(t)
	id := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output"
	poller, err := mock.Delete(t.Context(), id, "version")
	require.NoError(t, err)
	token, err := poller.ResumeToken()
	require.NoError(t, err)
	owner.Status.Coordinator = nil
	owner.Status.StatusHash = ""
	owner.Status.OutputResources = []string{id}
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	child := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: owner.Namespace, Finalizers: []string{DeploymentResourceFinalizer}, OwnerReferences: []metav1.OwnerReference{{APIVersion: radappiov1alpha3.GroupVersion.String(), Kind: deploymentTemplateKind, Name: owner.Name, UID: owner.UID, Controller: new(true)}}},
		Spec:       radappiov1alpha3.DeploymentResourceSpec{Id: id},
	}
	if held {
		child.Finalizers = append(child.Finalizers, "example.com/hold")
	}
	require.NoError(t, r.Client.Create(t.Context(), child))
	child.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
	require.NoError(t, r.Client.Status().Update(t.Context(), child))
	require.NoError(t, r.Client.Delete(t.Context(), child))
	coordinatorStep(t, r, owner)
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorDeleteRunning, owner.Status.Coordinator.Phase)
	require.Equal(t, token, owner.Status.Operation.ResumeToken)
	coordinatorStep(t, r, owner)
	require.Zero(t, sdk.putCalls.Load())
	require.Zero(t, sdk.deleteCalls.Load(), "adoption polls the original operation, not a second DELETE")
	mock.CompleteOperation(token, nil)
	coordinatorStep(t, r, owner)
	require.Empty(t, owner.Status.Coordinator.Outputs)
	if held {
		require.NoError(t, r.Client.Get(t.Context(), client.ObjectKeyFromObject(child), child))
		require.Nil(t, child.Status.Operation)
		require.NotContains(t, child.Finalizers, DeploymentResourceFinalizer)
	} else {
		require.True(t, apierrors.IsNotFound(r.Client.Get(t.Context(), client.ObjectKeyFromObject(child), child)))
	}
	coordinatorStep(t, r, owner)
	require.EqualValues(t, 1, sdk.putCalls.Load())
	require.Equal(t, coordinatorPutRunning, owner.Status.Coordinator.Phase)
}

type coordinatorHTTPTransport func(*http.Request) (*http.Response, error)

func (transport coordinatorHTTPTransport) Do(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func Test_CoordinatorSDKDoesNotRetryUncertainMutations(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			r, owner, sdk, _ := coordinatorFixture(t)
			var attempts atomic.Int64
			options := &arm.ClientOptions{}
			options.DisableRPRegistration = true
			options.Transport = coordinatorHTTPTransport(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, method, req.Method)
				if attempts.Add(1) == 1 {
					return nil, errors.New("response lost after server accepted mutation")
				}
				return &http.Response{
					StatusCode: http.StatusConflict, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{"error":{"code":"Conflict","message":"original operation is running"}}`)), Request: req,
				}, nil
			})
			realClient, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{BaseURI: "https://radius.invalid", Cred: &aztoken.AnonymousCredential{}, ARMClientOptions: options})
			require.NoError(t, err)
			sdk.ResourceDeploymentsClient = realClient
			if method == http.MethodPut {
				owner.Status.StatusHash = ""
			} else {
				owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output", State: outputPendingDelete}}
			}
			require.NoError(t, r.Client.Status().Update(t.Context(), owner))
			coordinatorStep(t, r, owner)
			require.EqualValues(t, 1, attempts.Load(), "the SDK must not mask a lost response with a later 409")
			require.Equal(t, coordinatorBlocked, owner.Status.Coordinator.Phase)
			owner.Spec.Template = `{"resources":[]}`
			require.NoError(t, r.Client.Update(t.Context(), owner))
			coordinatorStep(t, r, owner)
			require.EqualValues(t, 1, attempts.Load(), "an uncertain mutation cannot permit replacement work")
		})
	}
}

func Test_CoordinatorInventorySurvivesTrackerDisappearance(t *testing.T) {
	for _, window := range []string{"before-completion", "after-completion", "before-provenance"} {
		t.Run(window, func(t *testing.T) {
			r, owner, sdk, mock := coordinatorFixture(t)
			id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/web"
			owner.Status.StatusHash = ""
			require.NoError(t, r.Client.Status().Update(t.Context(), owner))
			coordinatorStep(t, r, owner)
			token := owner.Status.Operation.ResumeToken
			mock.CompleteOperation(token, func(state *sdkclients.OperationState) {
				state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{OutputResources: []*armdeployments.ResourceReference{{ID: new(id)}}}}
			})
			if window == "before-completion" {
				// No tracker exists yet; the completed result itself becomes durable inventory.
				coordinatorStep(t, r, owner)
			} else {
				coordinatorStep(t, r, owner)
				if window == "after-completion" {
					coordinatorStep(t, r, owner)
				}

			}
			name, err := generateDeploymentResourceName(id)
			require.NoError(t, err)
			if window == "after-completion" {
				child := &radappiov1alpha3.DeploymentResource{}
				require.NoError(t, r.Client.Get(t.Context(), client.ObjectKey{Namespace: owner.Namespace, Name: name}, child))
				child.Finalizers = nil
				require.NoError(t, r.Client.Update(t.Context(), child))
				require.NoError(t, r.Client.Delete(t.Context(), child))
			}
			if window == "before-provenance" {
				underlying := r.Client.(client.WithWatch)
				once := false
				r.Client = interceptor.NewClient(underlying, interceptor.Funcs{
					SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						if child, ok := obj.(*radappiov1alpha3.DeploymentResource); ok && !once {
							once = true
							if err := c.Delete(ctx, child); err != nil {
								return err
							}
							if err := c.Get(ctx, client.ObjectKeyFromObject(child), child); err != nil {
								return err
							}
							child.Finalizers = nil
							if err := c.Update(ctx, child); err != nil {
								return err
							}
							return apierrors.NewNotFound(radappiov1alpha3.GroupVersion.WithResource("deploymentresources").GroupResource(), child.Name)
						}
						return c.SubResource(sub).Update(ctx, obj, opts...)
					},
				})
				_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(owner)})
				require.Error(t, err)
				r.Client = underlying
			}
			restarted := *r
			coordinatorStep(t, &restarted, owner)
			require.Equal(t, outputCurrent, findOutput(owner, id).State)
			require.Zero(t, sdk.deleteCalls.Load(), "tracker disappearance cannot delete an output")
			require.EqualValues(t, 1, sdk.putCalls.Load())
		})
	}
}

func Test_CoordinatorSpecEditDuringDeletePreservesToken(t *testing.T) {
	r, owner, sdk, mock := coordinatorFixture(t)
	id := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output"
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: id, State: outputPendingDelete}}
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	sdk.deleteRequest = func(ctx context.Context, id, version string) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error) {
		current := &radappiov1alpha3.DeploymentTemplate{}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(owner), current); err != nil {
			return nil, err
		}
		current.Spec.Template = `{"resources":[]}`
		if err := r.Client.Update(ctx, current); err != nil {
			return nil, err
		}
		return mock.Delete(ctx, id, version)
	}
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorDeleteRunning, owner.Status.Coordinator.Phase)
	require.NotNil(t, owner.Status.Operation)
	require.Equal(t, `{"resources":[]}`, owner.Spec.Template)
	coordinatorStep(t, r, owner)
	require.Zero(t, sdk.putCalls.Load())
	mock.CompleteOperation(owner.Status.Operation.ResumeToken, nil)
	coordinatorStep(t, r, owner)
	coordinatorStep(t, r, owner)
	require.EqualValues(t, 1, sdk.putCalls.Load())
	require.EqualValues(t, 1, sdk.deleteCalls.Load())
}

func Test_CoordinatorRetainsPartialFailedPutInventory(t *testing.T) {
	r, owner, _, mock := coordinatorFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/partial"
	owner.Status.StatusHash = ""
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	coordinatorStep(t, r, owner)
	mock.CompleteOperation(owner.Status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{OutputResources: []*armdeployments.ResourceReference{{ID: new(id)}}}}
		state.Err = errors.New("another output failed")
	})
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorPutInventoryPending, owner.Status.Coordinator.Phase)
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorRetryWaiting, owner.Status.Coordinator.Phase)
	require.NotNil(t, findOutput(owner, id))
	require.Empty(t, owner.Status.StatusHash, "failed PUTs must not inherit the previous successful hash")
	before := owner.ResourceVersion
	coordinatorStep(t, r, owner)
	require.Equal(t, before, owner.ResourceVersion, "retry waiting must not churn status")
}

func Test_CoordinatorRejectsEditedTrackerIdentity(t *testing.T) {
	r, owner, sdk, _ := coordinatorFixture(t)
	a := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/a"
	b := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/b"
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: a, State: outputCurrent}, {ID: b, State: outputCurrent}}
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	coordinatorStep(t, r, owner)
	name, err := generateDeploymentResourceName(a)
	require.NoError(t, err)
	child := &radappiov1alpha3.DeploymentResource{}
	require.NoError(t, r.Client.Get(t.Context(), client.ObjectKey{Namespace: owner.Namespace, Name: name}, child))
	require.Equal(t, a, child.Status.ProvisionedResourceID)
	child.Spec.Id = b
	require.NoError(t, r.Client.Update(t.Context(), child))
	require.NoError(t, r.Client.Delete(t.Context(), child))
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorBlocked, owner.Status.Coordinator.Phase)
	require.Equal(t, outputCurrent, findOutput(owner, b).State)
	require.Zero(t, sdk.deleteCalls.Load())
}

func Test_CoordinatorCaseOnlyRadiusRedeployment(t *testing.T) {
	r, owner, sdk, mock := coordinatorFixture(t)
	id := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/Web"
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: id, State: outputCurrent}}
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	coordinatorStep(t, r, owner)
	owner.Spec.Template = `{"resources":[]}`
	require.NoError(t, r.Client.Update(t.Context(), owner))
	coordinatorStep(t, r, owner)
	mock.CompleteOperation(owner.Status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		state.Value = sdkclients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{OutputResources: []*armdeployments.ResourceReference{{ID: new(strings.ToLower(id))}}}}
	})
	for range 3 {
		coordinatorStep(t, r, owner)
	}
	require.Len(t, owner.Status.Coordinator.Outputs, 1)
	require.Equal(t, outputCurrent, findOutput(owner, id).State)
	require.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, owner.Status.Phrase)
	require.Zero(t, sdk.deleteCalls.Load())
	children, err := r.ownedTrackers(t.Context(), owner)
	require.NoError(t, err)
	require.Len(t, children, 1)
	for _, pair := range [][2]string{
		{id, strings.ToLower(id)},
		{"/planes/kubernetes/local/namespaces/test/providers/example.io/Widget/Web", "/planes/kubernetes/local/namespaces/test/providers/example.io/Widget/web"},
	} {
		left, err := generateDeploymentResourceName(pair[0])
		require.NoError(t, err)
		right, err := generateDeploymentResourceName(pair[1])
		require.NoError(t, err)
		require.Equal(t, sameOutput(pair[0], pair[1]), left == right)
	}
}

func Test_CoordinatorRealSDKFailedPutInventory(t *testing.T) {
	r, owner, sdk, _ := coordinatorFixture(t)
	id := "/planes/kubernetes/local/namespaces/test/providers/core/ConfigMap/partial"
	var gets, puts atomic.Int64
	var available atomic.Bool
	var endpoint string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPut:
			puts.Add(1)
			w.Header().Set("Azure-AsyncOperation", endpoint+"/operations/1")
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"properties":{"provisioningState":"Running"}}`)
		case req.URL.Path == "/operations/1":
			fmt.Fprint(w, `{"status":"Failed","error":{"code":"DeploymentFailed","message":"another output failed"}}`)
		case req.Method == http.MethodGet:
			gets.Add(1)
			if !available.Load() {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":{"code":"Forbidden","message":"inventory unavailable"}}`)
				return
			}
			fmt.Fprintf(w, `{"properties":{"provisioningState":"Failed","outputResources":[{"id":%q}]}}`, id)
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()
	endpoint = ts.URL
	options := &arm.ClientOptions{}
	options.Transport = ts.Client()
	options.DisableRPRegistration = true
	realClient, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{BaseURI: ts.URL, Cred: &aztoken.AnonymousCredential{}, ARMClientOptions: options})
	require.NoError(t, err)
	sdk.ResourceDeploymentsClient = realClient
	owner.Status.StatusHash = ""
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorPutRunning, owner.Status.Coordinator.Phase)
	token, deploymentID := owner.Status.Operation.ResumeToken, owner.Status.Coordinator.ResourceID
	coordinatorStep(t, r, owner)
	require.Equal(t, coordinatorPutInventoryPending, owner.Status.Coordinator.Phase)
	coordinatorStep(t, r, owner)
	require.Equal(t, token, owner.Status.Operation.ResumeToken)
	require.Equal(t, deploymentID, owner.Status.Coordinator.ResourceID)
	require.Equal(t, coordinatorPutInventoryPending, owner.Status.Coordinator.Phase)
	require.NoError(t, r.Client.Delete(t.Context(), owner))
	available.Store(true)
	restarted := *r
	coordinatorStep(t, &restarted, owner)
	require.Equal(t, coordinatorRetryWaiting, owner.Status.Coordinator.Phase)
	require.NotNil(t, findOutput(owner, id))
	require.Nil(t, owner.Status.Operation)
	require.EqualValues(t, 1, puts.Load())
	require.EqualValues(t, 2, gets.Load())
	require.Zero(t, sdk.deleteCalls.Load())
}

func Test_CoordinatorRetryRateWithManager(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "accepted"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) { testCoordinatorRetryRateWithManager(t, name, rejected) })
	}
}

func testCoordinatorRetryRateWithManager(t *testing.T, name string, rejected bool) {
	t.Helper()
	SkipWithoutEnvironment(t)
	ctx, cancel := context.WithCancel(t.Context())
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme, Metrics: server.Options{BindAddress: "0"},
		Controller: crconfig.Controller{SkipNameValidation: new(true)},
	})
	require.NoError(t, err)
	direct, err := client.NewWithWatch(config, client.Options{Scheme: scheme})
	require.NoError(t, err)
	namespace := "coordinator-retry-rate-" + name
	require.NoError(t, direct.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}))
	owner := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: namespace, Finalizers: []string{DeploymentTemplateFinalizer}},
		Spec:       radappiov1alpha3.DeploymentTemplateSpec{Template: "{}"},
	}
	require.NoError(t, direct.Create(ctx, owner))
	owner.Status.Coordinator = &radappiov1alpha3.DeploymentCoordinatorStatus{
		Phase: coordinatorIdle, Outputs: []radappiov1alpha3.DeploymentOutputStatus{{
			ID: "/planes/kubernetes/local/namespaces/" + namespace + "/providers/core/ConfigMap/output", State: outputPendingDelete,
		}},
	}
	require.NoError(t, direct.Status().Update(ctx, owner))
	var attempts atomic.Int64
	writes := interceptor.NewClient(direct, interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetObjectKind().GroupVersionKind().Kind == "ConfigMap" {
				attempts.Add(1)
				if !rejected {
					return nil // Accepted DELETE; simulate an output held by an external finalizer.
				}
				return apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, obj.GetName(), errors.New("delete denied"))
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	r := &DeploymentTemplateReconciler{
		Client: writes, APIReader: direct, Scheme: scheme, EventRecorder: record.NewFakeRecorder(100),
		DelayInterval: time.Second, DeleteRetryInterval: time.Second,
	}
	require.NoError(t, r.SetupWithManager(mgr))
	startManager(t, mgr, ctx, cancel)
	phase := coordinatorDeleteRunning
	if rejected {
		phase = coordinatorRetryWaiting
	}
	require.Eventually(t, func() bool {
		return direct.Get(ctx, client.ObjectKeyFromObject(owner), owner) == nil &&
			owner.Status.Coordinator.Phase == phase && attempts.Load() == 1
	}, time.Second*5, time.Millisecond*20)
	version := owner.ResourceVersion
	require.Never(t, func() bool { return attempts.Load() > 1 }, time.Millisecond*350, time.Millisecond*20)
	require.NoError(t, direct.Get(ctx, client.ObjectKeyFromObject(owner), owner))
	require.Equal(t, version, owner.ResourceVersion)
	require.Eventually(t, func() bool { return attempts.Load() >= 2 }, time.Second*3, time.Millisecond*20,
		"the persisted deadline must still schedule a timed retry")
}

func Test_CoordinatorPollFailureDoesNotLoseDeleteIntent(t *testing.T) {
	r, owner, sdk, _ := coordinatorFixture(t)
	id := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/containers/output"
	owner.Status.Coordinator.Phase = coordinatorDeleteRunning
	owner.Status.Coordinator.ResourceID = id
	owner.Status.Coordinator.Outputs = []radappiov1alpha3.DeploymentOutputStatus{{ID: id, State: outputPendingDelete}}
	owner.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: "unavailable-token"}
	require.NoError(t, r.Client.Status().Update(t.Context(), owner))
	version := owner.ResourceVersion
	for range 3 {
		coordinatorStep(t, r, owner)
		require.Equal(t, coordinatorDeleteRunning, owner.Status.Coordinator.Phase)
		require.Equal(t, "unavailable-token", owner.Status.Operation.ResumeToken)
		require.Equal(t, version, owner.ResourceVersion)
	}
	require.Zero(t, sdk.putCalls.Load())
	require.Zero(t, sdk.deleteCalls.Load())
}
