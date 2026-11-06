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

// Package elbv2 has the native handler for Elastic Load Balancing v2 listener certificates, which Cloud Control can't
// create in some regions.
package elbv2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const listenerCertificateType = "AWS::ElasticLoadBalancingV2::ListenerCertificate"

// API is the part of the Elastic Load Balancing v2 client the handler uses.
type API interface {
	AddListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error)
	RemoveListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error)
	DescribeListenerCertificates(ctx context.Context, params *elasticloadbalancingv2.DescribeListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeListenerCertificatesOutput, error)
}

func inRegion(region string) func(*elasticloadbalancingv2.Options) {
	return func(o *elasticloadbalancingv2.Options) { o.Region = region }
}

var (
	_ native.Handler       = (*ListenerCertificate)(nil)
	_ native.ScopedReader  = (*ListenerCertificate)(nil)
	_ native.ScopedDeleter = (*ListenerCertificate)(nil)
)

// ListenerCertificate serves AWS::ElasticLoadBalancingV2::ListenerCertificate: certificates a template adds to the
// certificate list of an HTTPS or TLS listener. Others can add certificates to the same listener, and every such
// listener has a default certificate, so the handler reads, changes, and deletes only the certificates the alias
// manages, and never the default one. Its identifier, Id, is ListenerArn and a name made from the record's client
// token, joined with "|".
type ListenerCertificate struct {
	api API
}

// NewListenerCertificate returns the handler for listener certificates.
func NewListenerCertificate(api API) *ListenerCertificate {
	return &ListenerCertificate{api: api}
}

// TypeName returns AWS::ElasticLoadBalancingV2::ListenerCertificate.
func (h *ListenerCertificate) TypeName() string {
	return listenerCertificateType
}

// Options returns how UCP uses the handler. Adding a certificate that is already on the listener changes nothing, so
// UCP finds a lost create by sending it again.
func (h *ListenerCertificate) Options() native.Options {
	return native.Options{
		List:     native.ListNone,
		Recovery: native.RecoverByResend,
	}
}

// listenerOf returns the listener ARN in identifier. An identifier without one, such as one another handler
// assigned, takes ListenerArn from the first of properties that sets it.
func listenerOf(identifier string, properties ...map[string]any) (string, error) {
	if listener, _, ok := strings.Cut(identifier, "|"); ok && listener != "" {
		return listener, nil
	}
	for _, p := range properties {
		if listener := awsapi.String(p, "ListenerArn"); listener != "" {
			return listener, nil
		}
	}
	return "", &native.Failure{
		Message:     fmt.Sprintf("the identifier %q of the AWS type %s names no listener", identifier, listenerCertificateType),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// certificateArns returns the CertificateArn of each item of the Certificates property.
func certificateArns(properties map[string]any) []string {
	list, _ := properties["Certificates"].([]any)
	out := []string{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		if arn := awsapi.String(m, "CertificateArn"); arn != "" && !slices.Contains(out, arn) {
			out = append(out, arn)
		}
	}
	return out
}

// certificates returns the ARNs in the listener's certificate list, without the default certificate. A default
// certificate that is also in the list comes back a second time without IsDefault, so it is kept.
func (h *ListenerCertificate) certificates(ctx context.Context, region string, listener string) ([]string, error) {
	out := []string{}
	pages := elasticloadbalancingv2.NewDescribeListenerCertificatesPaginator(h.api, &elasticloadbalancingv2.DescribeListenerCertificatesInput{ListenerArn: aws.String(listener)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "ListenerNotFound")
		}
		for _, c := range page.Certificates {
			arn := aws.ToString(c.CertificateArn)
			if aws.ToBool(c.IsDefault) || arn == "" || slices.Contains(out, arn) {
				continue
			}
			out = append(out, arn)
		}
	}
	return out, nil
}

// Read returns every certificate in the listener's certificate list, except the default certificate.
func (h *ListenerCertificate) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.ReadScoped(ctx, region, identifier, nil)
}

// ReadScoped returns the certificates in the listener's certificate list that one of managed lists, or every one
// when managed is nil, but never the default certificate. It returns ErrNotFound when none are there, or the listener
// is gone.
func (h *ListenerCertificate) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	listener, err := listenerOf(identifier, managed...)
	if err != nil {
		return nil, err
	}
	arns, err := h.certificates(ctx, region, listener)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, properties := range managed {
		for _, arn := range certificateArns(properties) {
			keep[arn] = true
		}
	}
	certificates := []any{}
	for _, arn := range arns {
		if managed == nil || keep[arn] {
			certificates = append(certificates, map[string]any{"CertificateArn": arn})
		}
	}
	if len(certificates) == 0 {
		return nil, native.ErrNotFound
	}
	return map[string]any{"Id": identifier, "ListenerArn": listener, "Certificates": certificates}, nil
}

// add returns the call that adds one certificate to the listener. AWS takes one certificate per call.
func (h *ListenerCertificate) add(ctx context.Context, region string, listener string, arn string) func() error {
	return func() error {
		_, err := h.api.AddListenerCertificates(ctx, &elasticloadbalancingv2.AddListenerCertificatesInput{
			ListenerArn:  aws.String(listener),
			Certificates: []types.Certificate{{CertificateArn: aws.String(arn)}},
		}, inRegion(region))
		return err
	}
}

// remove returns the call that removes one certificate from the listener. AWS takes one certificate per call.
func (h *ListenerCertificate) remove(ctx context.Context, region string, listener string, arn string) func() error {
	return func() error {
		_, err := h.api.RemoveListenerCertificates(ctx, &elasticloadbalancingv2.RemoveListenerCertificatesInput{
			ListenerArn:  aws.String(listener),
			Certificates: []types.Certificate{{CertificateArn: aws.String(arn)}},
		}, inRegion(region))
		return err
	}
}

// Create adds the certificates to the listener, one call each, and returns the identifier. A create that fails after
// the first call returns the identifier with the failure, so UCP saves it.
func (h *ListenerCertificate) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	listener := awsapi.String(request.Properties, "ListenerArn")
	arns := certificateArns(request.Properties)
	if listener == "" || len(arns) == 0 {
		return "", &native.Failure{Message: "set ListenerArn and at least one certificate in Certificates", ClientError: true, Outcome: native.OutcomeRejected}
	}
	identifier := listener + "|" + native.GenerateName(request.ClientToken)
	calls := []func() error{}
	for _, arn := range arns {
		calls = append(calls, h.add(ctx, request.Region, listener, arn))
	}
	if err := awsapi.Calls(calls...); err != nil {
		var failure *native.Failure
		if errors.As(err, &failure) {
			return identifier, err
		}
		return "", err
	}
	return identifier, nil
}

// Update removes the certificates Current has and Desired doesn't, then adds the ones Desired has and Current
// doesn't, one call each. A failed removal is reported as rejected, even when its response is lost, so the record
// keeps every certificate it may still have to remove.
func (h *ListenerCertificate) Update(ctx context.Context, request native.UpdateRequest) error {
	listener, err := listenerOf(request.Identifier, request.Desired, request.Current)
	if err != nil {
		return err
	}
	current, desired := certificateArns(request.Current), certificateArns(request.Desired)
	calls := []func() error{}
	for _, arn := range current {
		if slices.Contains(desired, arn) {
			continue
		}
		remove := h.remove(ctx, request.Region, listener, arn)
		calls = append(calls, func() error {
			if err := remove(); err != nil {
				return &native.Failure{Message: fmt.Sprintf("failed to remove a certificate from the listener: %s", err), ClientError: ucp_aws.IsClientFault(err), Outcome: native.OutcomeRejected}
			}
			return nil
		})
	}
	for _, arn := range desired {
		if !slices.Contains(current, arn) {
			calls = append(calls, h.add(ctx, request.Region, listener, arn))
		}
	}
	return awsapi.Calls(calls...)
}

// Delete is not used: UCP deletes listener certificates with DeleteScoped, so certificates Radius doesn't manage
// stay on the listener.
func (h *ListenerCertificate) Delete(ctx context.Context, region string, identifier string) error {
	return fmt.Errorf("the native handler for %s deletes only the certificates Radius manages, with DeleteScoped", listenerCertificateType)
}

// DeleteScoped removes the certificates managed lists that are still on the listener, one call each. It returns
// ErrNotFound when none are there, or the listener is gone.
func (h *ListenerCertificate) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	if managed == nil {
		// With no record of what the alias added, remove nothing rather than every certificate on the listener.
		managed = []map[string]any{}
	}
	current, err := h.ReadScoped(ctx, region, identifier, managed)
	if err != nil {
		return err
	}
	listener := awsapi.String(current, "ListenerArn")
	calls := []func() error{}
	for _, arn := range certificateArns(current) {
		remove := h.remove(ctx, region, listener, arn)
		calls = append(calls, func() error { return awsapi.NotFound(remove(), "ListenerNotFound") })
	}
	return awsapi.Calls(calls...)
}

// List is not supported: UCP calls it only for types it can list (native.ListRegion).
func (h *ListenerCertificate) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", listenerCertificateType)
}

// Progress reports every operation as finished, because adding and removing certificates takes effect at once.
func (h *ListenerCertificate) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
