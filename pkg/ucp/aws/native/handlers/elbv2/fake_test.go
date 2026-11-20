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

package elbv2

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// outsideType is the pseudo type under which the store keeps the certificates added to a listener outside Radius,
// keyed by the listener ARN.
const outsideType = "outside::" + listenerCertificateType

// missingCertificate is the ARN of a certificate that doesn't exist.
const missingCertificate = "arn:aws:acm:us-west-2:123456789012:certificate/00000000-0000-0000-0000-000000000000"

// certificatesPerLoadBalancer is the default quota of certificates per load balancer, without default certificates.
const certificatesPerLoadBalancer = 25

// defaultCertificate returns the ARN of the listener's default certificate.
func defaultCertificate(listener string) string {
	return listener + "/default-certificate"
}

// fakeAPI is an in-memory Elastic Load Balancing v2 API. The store has one entry per identifier, in the type's
// property format, with the certificates that alias added, and one entry of outsideType per listener with the
// certificates added outside Radius. A listener's certificate list is the union of its entries. Every listener has a
// default certificate, which DescribeListenerCertificates returns first with IsDefault set.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu sync.Mutex

	// current is the identifier of the entry an add puts its certificate in.
	current string

	// missing are listeners that don't exist.
	missing map[string]bool

	// pageSize is how many certificates DescribeListenerCertificates returns per page.
	pageSize int

	calls   []string
	regions []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, missing: map[string]bool{}, pageSize: 2, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) call(name string, optFns []func(*elasticloadbalancingv2.Options)) error {
	options := elasticloadbalancingv2.Options{}
	for _, fn := range optFns {
		fn(&options)
	}
	f.calls = append(f.calls, name)
	f.regions = append(f.regions, options.Region)
	return f.errs[name]
}

// use makes adds put their certificates in the entry of identifier.
func (f *fakeAPI) use(identifier string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = identifier
}

// entry is a store entry of a listener: its type and identifier.
type entry struct {
	typeName   string
	identifier string
}

// entries returns the store entries of the listener: the aliases' entries, then the one added outside Radius.
func (f *fakeAPI) entries(listener string) []entry {
	out := []entry{}
	for _, id := range f.store.List(listenerCertificateType) {
		if awsapi.String(f.store.Resource(listenerCertificateType, id), "ListenerArn") == listener {
			out = append(out, entry{listenerCertificateType, id})
		}
	}
	if f.store.Resource(outsideType, listener) != nil {
		out = append(out, entry{outsideType, listener})
	}
	return out
}

// list returns the certificate list of the listener, without its default certificate.
func (f *fakeAPI) list(listener string) []string {
	out := []string{}
	for _, e := range f.entries(listener) {
		for _, arn := range certificateArns(f.store.Resource(e.typeName, e.identifier)) {
			if !slices.Contains(out, arn) {
				out = append(out, arn)
			}
		}
	}
	return out
}

// loadBalancerOf returns the part of a listener ARN that names its load balancer: the ARN without the listener's ID.
func loadBalancerOf(listener string) string {
	if i := strings.LastIndex(listener, "/"); i >= 0 {
		return listener[:i]
	}
	return listener
}

// loadBalancerCertificates returns the certificates in the certificate lists of every listener of the load
// balancer, without default certificates.
func (f *fakeAPI) loadBalancerCertificates(loadBalancer string) []string {
	listeners := []string{}
	for _, id := range f.store.List(listenerCertificateType) {
		listeners = append(listeners, awsapi.String(f.store.Resource(listenerCertificateType, id), "ListenerArn"))
	}
	listeners = append(listeners, f.store.List(outsideType)...)
	out := []string{}
	for _, l := range listeners {
		if loadBalancerOf(l) != loadBalancer {
			continue
		}
		for _, arn := range f.list(l) {
			if arn != defaultCertificate(l) && !slices.Contains(out, arn) {
				out = append(out, arn)
			}
		}
	}
	return out
}

// one returns the ARN of the single certificate of a call, or ValidationError, because AWS takes one per call, with
// CertificateArn set and IsDefault not set.
func one(certificates []types.Certificate) (string, error) {
	if len(certificates) != 1 || aws.ToString(certificates[0].CertificateArn) == "" || certificates[0].IsDefault != nil {
		return "", fault("ValidationError")
	}
	return aws.ToString(certificates[0].CertificateArn), nil
}

func (f *fakeAPI) DescribeListenerCertificates(ctx context.Context, in *elasticloadbalancingv2.DescribeListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeListenerCertificatesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeListenerCertificates", optFns); err != nil {
		return nil, err
	}
	if in.PageSize != nil && (*in.PageSize < 1 || *in.PageSize > 400) {
		return nil, fault("ValidationError")
	}
	listener := aws.ToString(in.ListenerArn)
	if f.missing[listener] {
		return nil, fault("ListenerNotFound")
	}
	all := []types.Certificate{{CertificateArn: aws.String(defaultCertificate(listener)), IsDefault: aws.Bool(true)}}
	for _, arn := range f.list(listener) {
		all = append(all, types.Certificate{CertificateArn: aws.String(arn), IsDefault: aws.Bool(false)})
	}
	start := 0
	if in.Marker != nil {
		var err error
		if start, err = strconv.Atoi(aws.ToString(in.Marker)); err != nil {
			return nil, fault("ValidationError")
		}
	}
	size := f.pageSize
	if in.PageSize != nil {
		size = min(size, int(*in.PageSize))
	}
	end := min(start+size, len(all))
	out := &elasticloadbalancingv2.DescribeListenerCertificatesOutput{Certificates: all[start:end]}
	if end < len(all) {
		out.NextMarker = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) AddListenerCertificates(ctx context.Context, in *elasticloadbalancingv2.AddListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.AddListenerCertificatesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("AddListenerCertificates", optFns); err != nil {
		return nil, err
	}
	arn, err := one(in.Certificates)
	if err != nil {
		return nil, err
	}
	listener := aws.ToString(in.ListenerArn)
	if f.missing[listener] {
		return nil, fault("ListenerNotFound")
	}
	if arn == missingCertificate {
		return nil, fault("CertificateNotFound")
	}
	if f.current == "" {
		return nil, errors.New("the fake has no current entry: set it with use")
	}
	loadBalancer := f.loadBalancerCertificates(loadBalancerOf(listener))
	if arn != defaultCertificate(listener) && !slices.Contains(loadBalancer, arn) && len(loadBalancer) >= certificatesPerLoadBalancer {
		return nil, fault("TooManyCertificates")
	}
	if err := f.failer.Before(); err != nil {
		return nil, err
	}
	props := f.store.Resource(listenerCertificateType, f.current)
	if props == nil {
		props = map[string]any{"ListenerArn": listener, "Certificates": []any{}}
	}
	if !slices.Contains(certificateArns(props), arn) {
		props["Certificates"] = append(props["Certificates"].([]any), map[string]any{"CertificateArn": arn})
	}
	f.store.Put(listenerCertificateType, f.current, props)
	return &elasticloadbalancingv2.AddListenerCertificatesOutput{}, f.failer.After()
}

func (f *fakeAPI) RemoveListenerCertificates(ctx context.Context, in *elasticloadbalancingv2.RemoveListenerCertificatesInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.RemoveListenerCertificatesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RemoveListenerCertificates", optFns); err != nil {
		return nil, err
	}
	arn, err := one(in.Certificates)
	if err != nil {
		return nil, err
	}
	listener := aws.ToString(in.ListenerArn)
	if f.missing[listener] {
		return nil, fault("ListenerNotFound")
	}
	if arn == defaultCertificate(listener) && !slices.Contains(f.list(listener), arn) {
		return nil, fault("OperationNotPermitted")
	}
	if err := f.failer.Before(); err != nil {
		return nil, err
	}
	// AWS has one certificate list per listener, so a removal takes the certificate out of every entry.
	for _, e := range f.entries(listener) {
		props := f.store.Resource(e.typeName, e.identifier)
		list := []any{}
		for _, item := range props["Certificates"].([]any) {
			if awsapi.String(item.(map[string]any), "CertificateArn") != arn {
				list = append(list, item)
			}
		}
		if len(list) == 0 {
			f.store.Remove(e.typeName, e.identifier)
			continue
		}
		props["Certificates"] = list
		f.store.Put(e.typeName, e.identifier, props)
	}
	return &elasticloadbalancingv2.RemoveListenerCertificatesOutput{}, f.failer.After()
}

// tracked is the handler under test with its fake. It sets the fake's current entry to the identifier of each call,
// computed for a create as the handler does, so adds go to the entry of the alias that makes them.
type tracked struct {
	*ListenerCertificate
	api *fakeAPI
}

func newTracked(store *awsfake.CloudControl) *tracked {
	api := newFakeAPI(store)
	return &tracked{ListenerCertificate: NewListenerCertificate(api), api: api}
}

func (t *tracked) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	t.api.use(awsapi.String(request.Properties, "ListenerArn") + "|" + native.GenerateName(request.ClientToken))
	return t.ListenerCertificate.Create(ctx, request)
}

func (t *tracked) Update(ctx context.Context, request native.UpdateRequest) error {
	t.api.use(request.Identifier)
	return t.ListenerCertificate.Update(ctx, request)
}

func (t *tracked) ReadScoped(ctx context.Context, region string, identifier string, managed []map[string]any) (map[string]any, error) {
	t.api.use(identifier)
	return t.ListenerCertificate.ReadScoped(ctx, region, identifier, managed)
}

func (t *tracked) DeleteScoped(ctx context.Context, region string, identifier string, managed []map[string]any) error {
	t.api.use(identifier)
	return t.ListenerCertificate.DeleteScoped(ctx, region, identifier, managed)
}
