/*
Copyright 2023 The Radius Authors.

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

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/sdk"
	ucp_credentials "github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileResourceOutcome mirrors the wire shape defined in Radius.Core's TypeSpec so the
// corerp app-scoped orchestrator can aggregate reports across resource-provider namespaces
// without re-marshaling. Kept lowercase in JSON to match the client's expectations.
type ReconcileResourceOutcome struct {
	ResourceID string `json:"resourceId"`
	From       string `json:"from"`
	To         string `json:"to"`
	Reason     string `json:"reason,omitempty"`
}

// ReconcileResponse is the per-resource reconcile response emitted by dynamic-rp. The corerp
// orchestrator (which fans out per-application) collects one ReconcileResourceOutcome per
// resource; each dynamic-rp reconcile call returns a single-element resources array (or an empty
// array when the resource is already terminal and no work was needed).
type ReconcileResponse struct {
	Resources []ReconcileResourceOutcome `json:"resources"`
}

// Reconcile is the dynamic-rp handler for the reconcile custom action registered on every dynamic
// resource type. See specs/006-state-restoration: when 'rad startup' invokes the app-scoped
// reconcile on Radius.Core/applications, the corerp orchestrator POSTs to this handler once per
// non-terminal child resource. The handler walks the resource's outputResources, checks each one
// against its underlying provider, and updates provisioningState to reflect reality.
//
// For each output resource we query its underlying provider and categorize the result as gone
// (404), settled (present), or skipped (unknown provider / transient error). We then aggregate:
// all outputs gone → Failed, all settled → Succeeded, otherwise the current state is retained.
type Reconcile struct {
	ctrl.Operation[*datamodel.DynamicResource, datamodel.DynamicResource]
	resourceOptions ctrl.ResourceOptions[datamodel.DynamicResource]
	ucpConnection   sdk.Connection
	discovery       discovery.DiscoveryInterface
}

// NewReconcile constructs the reconcile controller for a dynamic resource type. The runtime
// client is read from opts.KubeClient at request time so the same handler serves every dynamic
// type without per-type wiring.
func NewReconcile(opts ctrl.Options, resourceOptions ctrl.ResourceOptions[datamodel.DynamicResource], ucpConnection sdk.Connection, discovery discovery.DiscoveryInterface) (ctrl.Controller, error) {
	return &Reconcile{
		Operation:       ctrl.NewOperation(opts, resourceOptions),
		resourceOptions: resourceOptions,
		ucpConnection:   ucpConnection,
		discovery:       discovery,
	}, nil
}

// outputStatus is the reality category assigned to a single outputResource.
type outputStatus string

const (
	outputGone    outputStatus = "gone"
	outputSettled outputStatus = "settled"
	outputSkipped outputStatus = "skipped"
)

type outputCheck struct {
	id     string
	status outputStatus
	reason string
}

// Run reconciles the target resource's provisioningState against the reality of its
// outputResources and returns a single-element report for the resource. If the resource is
// already in a terminal state, no work is done and an empty resources array is returned so the
// caller can distinguish "no-op" from "reconciled".
func (c *Reconcile) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Route: /planes/radius/{plane}/resourceGroups/{rg}/providers/{ns}/{type}/{name}/reconcile
	resourceID := sCtx.ResourceID.Truncate()
	resource, etag, err := c.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	fromState := resource.ProvisioningState()
	if fromState.IsTerminal() {
		return rest.NewOKResponse(&ReconcileResponse{Resources: []ReconcileResourceOutcome{}}), nil
	}

	checks := make([]outputCheck, 0)
	for _, out := range resource.OutputResources() {
		checks = append(checks, c.checkOutput(ctx, out))
	}

	toState := aggregateReconcileState(fromState, checks)
	outcome := ReconcileResourceOutcome{
		ResourceID: resourceID.String(),
		From:       string(fromState),
		To:         string(toState),
		Reason:     summarizeChecks(checks),
	}

	if toState != fromState {
		resource.SetProvisioningState(toState)
		if _, err := c.SaveResource(ctx, resourceID.String(), resource, etag); err != nil {
			return nil, err
		}
	}

	return rest.NewOKResponse(&ReconcileResponse{Resources: []ReconcileResourceOutcome{outcome}}), nil
}

// checkOutput probes a single output resource against its underlying provider.
func (c *Reconcile) checkOutput(ctx context.Context, out rpv1.OutputResource) outputCheck {
	idStr := out.ID.String()

	if resources_azure.IsAzureResource(out.ID) {
		return c.checkAzureOutput(ctx, out.ID)
	}

	scopes := out.ID.ScopeSegments()
	if len(scopes) == 0 || !strings.EqualFold(scopes[0].Type, resources_kubernetes.PlaneTypeKubernetes) {
		return outputCheck{id: idStr, status: outputSkipped, reason: "unsupported output resource provider"}
	}

	kubeClient := c.Options().KubeClient
	if kubeClient == nil {
		return outputCheck{id: idStr, status: outputSkipped, reason: "kubernetes runtime client not configured"}
	}

	group, kind, namespace, name := resources_kubernetes.ToParts(out.ID)

	version, err := c.lookupKubernetesAPIVersion(group, kind, namespace != "")
	if err != nil {
		return outputCheck{id: idStr, status: outputSkipped, reason: fmt.Sprintf("could not resolve Kubernetes API version for %s/%s: %v", group, kind, err)}
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})

	err = kubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: namespace, Name: name}, obj)
	switch {
	case apierrors.IsNotFound(err):
		return outputCheck{id: idStr, status: outputGone, reason: "kubernetes object not found"}
	case err != nil:
		return outputCheck{id: idStr, status: outputSkipped, reason: fmt.Sprintf("kubernetes GET failed: %v", err)}
	default:
		return outputCheck{id: idStr, status: outputSettled}
	}
}

func (c *Reconcile) checkAzureOutput(ctx context.Context, id resources.ID) outputCheck {
	idStr := id.String()
	if c.ucpConnection == nil {
		return outputCheck{id: idStr, status: outputSkipped, reason: "UCP connection not configured"}
	}

	armID := id
	azurePlaneScope := "/planes/azure/" + ucp_credentials.AzureCloud
	if id.IsUCPQualified() {
		var err error
		armID, err = resources.ParseResource(resources.MakeRelativeID(id.ScopeSegments()[1:], id.TypeSegments(), id.ExtensionSegments()))
		if err != nil {
			return outputCheck{id: idStr, status: outputSkipped, reason: fmt.Sprintf("could not normalize Azure resource ID: %v", err)}
		}
		azurePlaneScope = id.PlaneScope()
	}

	clientOptions := sdk.NewClientOptions(&endpointConnection{
		Connection: c.ucpConnection,
		endpoint:   strings.TrimRight(c.ucpConnection.Endpoint(), "/") + azurePlaneScope,
	})
	apiVersion, err := lookupAzureAPIVersion(ctx, armID, clientOptions)
	if err != nil {
		return outputCheck{id: idStr, status: outputSkipped, reason: fmt.Sprintf("could not resolve Azure API version: %v", err)}
	}

	client, err := clientv2.NewGenericResourceClient(
		armID.FindScope(resources_azure.ScopeSubscriptions),
		&clientv2.Options{Cred: &aztoken.AnonymousCredential{}},
		clientOptions,
	)
	if err != nil {
		return outputCheck{id: idStr, status: outputSkipped, reason: fmt.Sprintf("could not create Azure resource client: %v", err)}
	}

	_, err = client.GetByID(ctx, armID.String(), apiVersion, &armresources.ClientGetByIDOptions{})
	switch {
	case clients.Is404Error(err):
		return outputCheck{id: idStr, status: outputGone, reason: "Azure resource not found"}
	case err != nil:
		return outputCheck{id: idStr, status: outputSkipped, reason: fmt.Sprintf("Azure GET failed: %v", err)}
	default:
		return outputCheck{id: idStr, status: outputSettled}
	}
}

func lookupAzureAPIVersion(ctx context.Context, id resources.ID, clientOptions *arm.ClientOptions) (string, error) {
	client, err := clientv2.NewProvidersClient(
		id.FindScope(resources_azure.ScopeSubscriptions),
		&clientv2.Options{Cred: &aztoken.AnonymousCredential{}},
		clientOptions,
	)
	if err != nil {
		return "", err
	}

	provider, err := client.Get(ctx, id.ProviderNamespace(), nil)
	if err != nil {
		return "", err
	}

	segments := id.TypeSegments()
	if len(id.ExtensionSegments()) > 0 {
		segments = id.ExtensionSegments()
	}
	shortType := strings.TrimPrefix(segments[0].Type, id.ProviderNamespace()+"/")
	for _, resourceType := range provider.ResourceTypes {
		if resourceType.ResourceType == nil || !strings.EqualFold(shortType, *resourceType.ResourceType) {
			continue
		}
		if resourceType.DefaultAPIVersion != nil && *resourceType.DefaultAPIVersion != "" {
			return *resourceType.DefaultAPIVersion, nil
		}
		if len(resourceType.APIVersions) > 0 && resourceType.APIVersions[0] != nil {
			return *resourceType.APIVersions[0], nil
		}
		return "", fmt.Errorf("no supported API versions for type %q", id.Type())
	}

	return "", fmt.Errorf("resource type %q was not found", id.Type())
}

type endpointConnection struct {
	sdk.Connection
	endpoint string
}

func (c *endpointConnection) Endpoint() string {
	return c.endpoint
}

// lookupKubernetesAPIVersion resolves the preferred API version for a group+kind via the
// discovery client. This mirrors the walk in the corerp kubernetes handler; keeping a copy here
// decouples the reconcile path from the deployment codepath.
func (c *Reconcile) lookupKubernetesAPIVersion(group, kind string, namespaced bool) (string, error) {
	if c.discovery == nil {
		return "", fmt.Errorf("discovery client is not configured")
	}

	// resources_kubernetes.ToParts maps the "core" ProviderNamespace back to "" for the built-in
	// group. Discovery reports the same group as "" — normalize before comparing.
	normalizedGroup := group
	if normalizedGroup == "core" {
		normalizedGroup = ""
	}

	var lists []*metav1.APIResourceList
	var err error
	if namespaced {
		lists, err = c.discovery.ServerPreferredNamespacedResources()
	} else {
		lists, err = c.discovery.ServerPreferredResources()
	}
	if err != nil {
		return "", err
	}

	for _, list := range lists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		if !strings.EqualFold(gv.Group, normalizedGroup) {
			continue
		}
		for _, r := range list.APIResources {
			if strings.EqualFold(r.Kind, kind) {
				return gv.Version, nil
			}
		}
	}

	return "", fmt.Errorf("no preferred API version for %s/%s", group, kind)
}

// aggregateReconcileState folds the per-output outcomes into a single provisioningState decision.
//
// Rules:
//   - No outputs on record → leave state unchanged. We do not assume "gone" without evidence.
//   - All outputs gone → move to Failed.
//   - Any output skipped (cloud output, unresolved version, transient GET failure) → leave state
//     unchanged. We refuse to lie about state we could not verify.
//   - All outputs settled → move to Succeeded.
//   - Otherwise (mix of settled and gone with no skipped) → leave state unchanged.
func aggregateReconcileState(from v1.ProvisioningState, checks []outputCheck) v1.ProvisioningState {
	if len(checks) == 0 {
		return from
	}
	hasSkipped := false
	hasSettled := false
	hasGone := false
	for _, c := range checks {
		switch c.status {
		case outputSkipped:
			hasSkipped = true
		case outputSettled:
			hasSettled = true
		case outputGone:
			hasGone = true
		}
	}
	if hasSkipped {
		return from
	}
	if hasGone && !hasSettled {
		return v1.ProvisioningStateFailed
	}
	if hasSettled && !hasGone {
		return v1.ProvisioningStateSucceeded
	}
	return from
}

// summarizeChecks flattens the per-output outcomes into one human-readable reason string.
func summarizeChecks(checks []outputCheck) string {
	if len(checks) == 0 {
		return "no output resources on record"
	}
	parts := make([]string, 0, len(checks))
	for _, c := range checks {
		s := fmt.Sprintf("%s: %s", c.id, c.status)
		if c.reason != "" {
			s += " (" + c.reason + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}
