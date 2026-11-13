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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/elasticloadbalancing/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	quotas       = "https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-limits.html"
)

// addCertificate adds the certificates to the listener with a fake Elastic Load Balancing v2 API backed by store.
func addCertificate(ctx context.Context, store *awsfake.CloudControl, listenerArn string, certificates ...types.Certificate) error {
	f := newFakeAPI(store)
	f.use(listenerArn + "|rules")
	_, err := f.AddListenerCertificates(ctx, &elasticloadbalancingv2.AddListenerCertificatesInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: certificates,
	})
	return err
}

// removeCertificate removes the certificates from the listener with a fake Elastic Load Balancing v2 API backed by
// store.
func removeCertificate(ctx context.Context, store *awsfake.CloudControl, listenerArn string, certificates ...types.Certificate) error {
	_, err := newFakeAPI(store).RemoveListenerCertificates(ctx, &elasticloadbalancingv2.RemoveListenerCertificatesInput{
		ListenerArn:  aws.String(listenerArn),
		Certificates: certificates,
	})
	return err
}

// describeCertificates returns every certificate of the listener with a fake Elastic Load Balancing v2 API backed by
// store.
func describeCertificates(ctx context.Context, store *awsfake.CloudControl, listenerArn string) ([]types.Certificate, error) {
	out := []types.Certificate{}
	pages := elasticloadbalancingv2.NewDescribeListenerCertificatesPaginator(newFakeAPI(store), &elasticloadbalancingv2.DescribeListenerCertificatesInput{ListenerArn: aws.String(listenerArn)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Certificates...)
	}
	return out, nil
}

// certificate returns a certificate with ARN arn, for a request.
func certificate(arn string) types.Certificate {
	return types.Certificate{CertificateArn: aws.String(arn)}
}

// missingListener returns a fake Elastic Load Balancing v2 API backed by store on which listener doesn't exist.
func missingListener(store *awsfake.CloudControl) *fakeAPI {
	f := newFakeAPI(store)
	f.missing[listener] = true
	return f
}

// certificateRules returns the rules AWS's API reference gives for the certificates of a call that takes them.
func certificateRules(call func(ctx context.Context, store *awsfake.CloudControl, listenerArn string, certificates ...types.Certificate) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "a call takes one certificate",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				if err := call(ctx, store, listener); err == nil {
					return fmt.Errorf("the fake accepts a call with no certificate")
				}
				return call(ctx, store, listener, certificate(certA), certificate(certB))
			},
			Code: "ValidationError",
			Doc:  commonErrors,
		},
		{
			Rule: "the certificate doesn't set IsDefault",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, listener, types.Certificate{CertificateArn: aws.String(certA), IsDefault: aws.Bool(false)})
			},
			Code: "ValidationError",
			Doc:  commonErrors,
		},
	}
}

// listenerCertificateAPI records the Elastic Load Balancing v2 calls ListenerCertificate makes.
var listenerCertificateAPI = map[string]nativetest.Call{
	"AddListenerCertificates": {
		Doc: apiReference + "API_AddListenerCertificates.html",
		Rules: append(certificateRules(addCertificate),
			nativetest.Rule{
				Rule: "adding a certificate already in the certificate list succeeds and doesn't add it again",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					for range 2 {
						if err := addCertificate(ctx, store, listener, certificate(certA)); err != nil {
							return err
						}
					}
					all, err := describeCertificates(ctx, store, listener)
					if err != nil {
						return err
					}
					if len(all) != 2 {
						return fmt.Errorf("the listener has certificates %v", all)
					}
					return nil
				},
			},
			nativetest.Rule{
				Rule: "the certificate exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return addCertificate(ctx, store, listener, certificate(missingCertificate))
				},
				Code: "CertificateNotFound",
			},
			nativetest.Rule{
				Rule: "the listener exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := missingListener(store)
					f.use(listener + "|rules")
					_, err := f.AddListenerCertificates(ctx, &elasticloadbalancingv2.AddListenerCertificatesInput{
						ListenerArn:  aws.String(listener),
						Certificates: []types.Certificate{certificate(certA)},
					})
					return err
				},
				Code: "ListenerNotFound",
			},
			nativetest.Rule{
				Rule: "a load balancer takes at most 25 certificates across its listeners by default, not counting default certificates",
				Doc:  quotas,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					for i := range certificatesPerLoadBalancer {
						l := listener
						if i%2 == 1 {
							l = listener2
						}
						if err := addCertificate(ctx, store, l, certificate(fmt.Sprintf("%s-%d", certA, i))); err != nil {
							return err
						}
					}
					return addCertificate(ctx, store, listener, certificate(certB))
				},
				Code: "TooManyCertificates",
			},
		),
		Unclear: "The page says the listener is HTTPS or TLS but names no error for another protocol; the fake has no " +
			"listener protocols. The quota of certificates per load balancer is adjustable, and the quota pages don't " +
			"say whether a certificate on two listeners of a load balancer counts twice; the fake applies the default " +
			"quota of 25 and counts it once.",
	},
	"DescribeListenerCertificates": {
		Doc: apiReference + "API_DescribeListenerCertificates.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the default certificate is returned with IsDefault true, and again with IsDefault false when it is also in the certificate list",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := addCertificate(ctx, store, listener, certificate(defaultCertificate(listener))); err != nil {
						return err
					}
					all, err := describeCertificates(ctx, store, listener)
					if err != nil {
						return err
					}
					if len(all) != 2 || !aws.ToBool(all[0].IsDefault) || aws.ToBool(all[1].IsDefault) ||
						aws.ToString(all[0].CertificateArn) != aws.ToString(all[1].CertificateArn) {
						return fmt.Errorf("the listener has certificates %v", all)
					}
					return nil
				},
			},
			{
				Rule: "PageSize is 1 to 400",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeListenerCertificates(ctx, &elasticloadbalancingv2.DescribeListenerCertificatesInput{
						ListenerArn: aws.String(listener),
						PageSize:    aws.Int32(401),
					})
					return err
				},
				Code: "ValidationError",
				Doc:  commonErrors,
			},
			{
				Rule: "the listener exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := missingListener(store).DescribeListenerCertificates(ctx, &elasticloadbalancingv2.DescribeListenerCertificatesInput{ListenerArn: aws.String(listener)})
					return err
				},
				Code: "ListenerNotFound",
			},
		},
		Unclear: "The page doesn't say what AWS returns for a marker it didn't issue; the fake returns ValidationError.",
	},
	"RemoveListenerCertificates": {
		Doc: apiReference + "API_RemoveListenerCertificates.html",
		Rules: append(certificateRules(removeCertificate),
			nativetest.Rule{
				Rule: "the default certificate can't be removed",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return removeCertificate(ctx, store, listener, certificate(defaultCertificate(listener)))
				},
				Code: "OperationNotPermitted",
			},
			nativetest.Rule{
				Rule: "the listener exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := missingListener(store).RemoveListenerCertificates(ctx, &elasticloadbalancingv2.RemoveListenerCertificatesInput{
						ListenerArn:  aws.String(listener),
						Certificates: []types.Certificate{certificate(certA)},
					})
					return err
				},
				Code: "ListenerNotFound",
			},
		),
		Unclear: "The page doesn't say what AWS does for a certificate that isn't in the certificate list; the fake " +
			"accepts it and changes nothing. It doesn't say whether the default certificate can be taken out of the " +
			"certificate list when it was added there too; the fake removes it from the list and keeps it as default.",
	},
}
