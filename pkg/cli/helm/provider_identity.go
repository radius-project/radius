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
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
)

type externalIdentityTarget struct {
	TLSProofTarget
	Component string
}

type externalIdentityEvidence struct {
	mtls.InternalIdentityReceipt
	Upstream *mtls.InternalIdentityReceipt `json:"upstream,omitempty"`
}

type externalIdentityObserver struct {
	kube   kubernetes.Interface
	config *rest.Config
	// Tests replace only the tunnel, retaining actual HTTP, token requests,
	// receipt validation and Kubernetes pod/container checks.
	open func(context.Context, externalIdentityTarget) (string, func(), error)
}

func identityUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func identityPod(pod *corev1.Pod, target externalIdentityTarget) (corev1.ContainerStatus, error) {
	if !identityUUID(string(target.PodUID)) || pod.Namespace != target.Namespace || pod.Name != target.PodName ||
		pod.Spec.ServiceAccountName != target.Component || len(validation.IsDNS1123Label(target.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(target.PodName)) != 0 || len(validation.IsDNS1123Label(target.ContainerName)) != 0 ||
		!slices.Contains(rotationComponents, target.Component) || target.Port == 0 {
		return corev1.ContainerStatus{}, errors.New("external identity pod target or ServiceAccount changed")
	}
	container, err := proofContainer(pod, target.TLSProofTarget)
	if err != nil {
		return corev1.ContainerStatus{}, err
	}
	if !container.Ready {
		return corev1.ContainerStatus{}, errors.New("external identity container is not ready")
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return container, nil
		}
	}
	return corev1.ContainerStatus{}, errors.New("external identity pod is not ready")
}

// observe uses a short-lived, pod-bound observer token, never a workload's
// projected credential. The nested receipt must come from the workload's actual
// UCP client; a second direct probe binds that receipt to the current UCP process.
// Neither receipt supplies cryptographic server authentication over external HTTP.
func (o *externalIdentityObserver) observe(ctx context.Context, mode authz.Mode, target, caller externalIdentityTarget,
	ucps map[types.UID]externalIdentityTarget) (externalIdentityEvidence, error) {
	fail := externalIdentityEvidence{}
	if mode != authz.ModeDryRun && mode != authz.ModeEnforce ||
		target.Namespace != caller.Namespace || target.Component == "ucp" && caller.Component != "bicep-de" ||
		target.Component != "ucp" && (caller.Component != "ucp" || len(ucps) == 0) ||
		!slices.Contains(rotationComponents, target.Component) {
		return fail, errors.New("invalid external identity observation")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return fail, err
	}
	targets := []externalIdentityTarget{target, caller}
	for uid, upstream := range ucps {
		if uid != upstream.PodUID || upstream.Component != "ucp" || upstream.Namespace != target.Namespace {
			return fail, errors.New("invalid external identity UCP inventory")
		}
		targets = append(targets, upstream)
	}
	before := map[types.UID]corev1.ContainerStatus{}
	for _, expected := range targets {
		pod, err := o.kube.CoreV1().Pods(expected.Namespace).Get(ctx, expected.PodName, metav1.GetOptions{})
		if err != nil {
			return fail, fmt.Errorf("cannot inspect external identity pod: %w", err)
		}
		container, err := identityPod(pod, expected)
		if err != nil {
			return fail, err
		}
		before[expected.PodUID] = container
	}
	evidence, err := o.receipt(ctx, mode, target, caller)
	if err != nil {
		return fail, err
	}
	if target.Component != "ucp" {
		upstream, found := ucps[types.UID(evidence.Upstream.PodUID)]
		if !found {
			return fail, errors.New("external identity receipt came from an unexpected UCP pod")
		}
		current, err := o.receipt(ctx, mode, upstream, target)
		if err != nil {
			return fail, err
		}
		if current.BootID != evidence.Upstream.BootID {
			return fail, errors.New("external identity UCP process changed during observation")
		}
	}
	for _, expected := range targets {
		pod, err := o.kube.CoreV1().Pods(expected.Namespace).Get(ctx, expected.PodName, metav1.GetOptions{})
		if err != nil {
			return fail, fmt.Errorf("cannot recheck external identity pod: %w", err)
		}
		current, err := identityPod(pod, expected)
		if err != nil {
			return fail, err
		}
		previous := before[expected.PodUID]
		if current.ContainerID != previous.ContainerID || current.RestartCount != previous.RestartCount ||
			!current.State.Running.StartedAt.Equal(&previous.State.Running.StartedAt) {
			return fail, errors.New("external identity container restarted during observation")
		}
	}
	return evidence, nil
}

func (o *externalIdentityObserver) receipt(ctx context.Context, mode authz.Mode, target, caller externalIdentityTarget) (externalIdentityEvidence, error) {
	fail := externalIdentityEvidence{}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return fail, err
	}
	token, err := o.kube.CoreV1().ServiceAccounts(target.Namespace).CreateToken(ctx, caller.Component,
		&authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{
			Audiences: []string{target.Component + "." + target.Namespace + ".svc"}, ExpirationSeconds: ptr.To(int64(600)),
			BoundObjectRef: &authenticationv1.BoundObjectReference{Kind: "Pod", APIVersion: "v1", Name: caller.PodName, UID: caller.PodUID},
		}}, metav1.CreateOptions{})
	if err != nil {
		return fail, errors.New("cannot issue pod-bound identity observer token; check ServiceAccount token permissions")
	}
	if token.Status.Token == "" || len(token.Status.Token) > 16384 || !token.Status.ExpirationTimestamp.After(time.Now()) {
		return fail, errors.New("identity observer token is empty, oversized or expired")
	}
	open := o.open
	if open == nil {
		open = o.portForward
	}
	origin, stop, err := open(ctx, target)
	if err != nil {
		return fail, err
	}
	defer stop()
	challenge := uuid.NewString()
	start := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+mtls.InternalIdentityPath+"?challenge="+challenge, nil)
	if err != nil {
		return fail, errors.New("cannot construct external identity request")
	}
	request.Header.Set("Authorization", "Bearer "+token.Status.Token)
	defer request.Header.Del("Authorization")
	token.Status.Token = ""
	request.Header.Set("Cache-Control", "no-store")
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 7 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return fail, errors.New("external identity request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fail, fmt.Errorf("external identity endpoint unavailable (HTTP %d)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(data) > 8192 {
		return fail, errors.New("external identity response is unreadable or oversized")
	}
	evidence, err := decodeExternalIdentityEvidence(data, target.Component != "ucp")
	if err != nil {
		return fail, err
	}
	if err := validateIdentityReceipt(evidence.InternalIdentityReceipt, mode, target.Component, target.Namespace,
		caller.Component, challenge, start); err != nil {
		return fail, err
	}
	if evidence.PodUID != string(target.PodUID) {
		return fail, errors.New("external identity endpoint belongs to a different pod")
	}
	if evidence.Upstream != nil {
		if err := validateIdentityReceipt(*evidence.Upstream, mode, "ucp", target.Namespace,
			target.Component, challenge, start); err != nil {
			return fail, err
		}
	}
	return evidence, nil
}

func validateIdentityReceipt(receipt mtls.InternalIdentityReceipt, mode authz.Mode, component, namespace, caller, challenge string, start time.Time) error {
	if receipt.SchemaVersion != 1 || receipt.Provider != "external" || receipt.Mode != mode ||
		receipt.Component != component || receipt.Namespace != namespace || receipt.Challenge != challenge ||
		receipt.Caller != "system:serviceaccount:"+namespace+":"+caller ||
		!identityUUID(receipt.PodUID) || !identityUUID(receipt.BootID) ||
		receipt.ObservedAt.Before(start.Add(-30*time.Second)) || receipt.ObservedAt.After(time.Now().Add(30*time.Second)) {
		return errors.New("external identity receipt does not match the current observation")
	}
	return nil
}

func decodeExternalIdentityEvidence(data []byte, upstream bool) (externalIdentityEvidence, error) {
	fail := externalIdentityEvidence{}
	if len(data) > 8192 || checkProviderTransitionJSON(string(data)) != nil {
		return fail, errors.New("invalid external identity evidence")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return fail, errors.New("invalid external identity evidence")
	}
	nested, present := fields["upstream"]
	if present != upstream {
		return fail, errors.New("external identity evidence has the wrong upstream shape")
	}
	delete(fields, "upstream")
	base, err := json.Marshal(fields)
	if err != nil {
		return fail, err
	}
	receipt, err := mtls.DecodeInternalIdentityReceipt(base)
	if err != nil {
		return fail, err
	}
	evidence := externalIdentityEvidence{InternalIdentityReceipt: receipt}
	if upstream {
		receipt, err := mtls.DecodeInternalIdentityReceipt(nested)
		if err != nil {
			return fail, err
		}
		evidence.Upstream = &receipt
	}
	return evidence, nil
}

func (o *externalIdentityObserver) portForward(ctx context.Context, target externalIdentityTarget) (string, func(), error) {
	return openPodProofPortForward(ctx, o.config, target.TLSProofTarget)
}
